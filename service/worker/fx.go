package worker

import (
	"context"
	"os"

	"github.com/superdurable/temporal-server/api/adminservice/v1"
	"github.com/superdurable/temporal-server/chasm"
	"github.com/superdurable/temporal-server/chasm/lib/scheduler/gen/schedulerpb/v1"
	"github.com/superdurable/temporal-server/client"
	"github.com/superdurable/temporal-server/common"
	"github.com/superdurable/temporal-server/common/cluster"
	"github.com/superdurable/temporal-server/common/config"
	"github.com/superdurable/temporal-server/common/dynamicconfig"
	"github.com/superdurable/temporal-server/common/log"
	"github.com/superdurable/temporal-server/common/log/tag"
	"github.com/superdurable/temporal-server/common/membership"
	"github.com/superdurable/temporal-server/common/metrics"
	"github.com/superdurable/temporal-server/common/namespace"
	"github.com/superdurable/temporal-server/common/namespace/nsreplication"
	"github.com/superdurable/temporal-server/common/persistence"
	"github.com/superdurable/temporal-server/common/persistence/serialization"
	"github.com/superdurable/temporal-server/common/persistence/visibility"
	"github.com/superdurable/temporal-server/common/persistence/visibility/manager"
	"github.com/superdurable/temporal-server/common/primitives"
	"github.com/superdurable/temporal-server/common/resolver"
	"github.com/superdurable/temporal-server/common/resource"
	"github.com/superdurable/temporal-server/common/sdk"
	"github.com/superdurable/temporal-server/common/searchattribute"
	wcicomponent "github.com/superdurable/temporal-server/components/autoscaledworkers/wci/workercomponent"
	"github.com/superdurable/temporal-server/service"
	"github.com/superdurable/temporal-server/service/worker/batcher"
	workercommon "github.com/superdurable/temporal-server/service/worker/common"
	"github.com/superdurable/temporal-server/service/worker/deletenamespace"
	"github.com/superdurable/temporal-server/service/worker/dlq"
	"github.com/superdurable/temporal-server/service/worker/dummy"
	"github.com/superdurable/temporal-server/service/worker/migration"
	"github.com/superdurable/temporal-server/service/worker/scheduler"
	"github.com/superdurable/temporal-server/service/worker/workerdeployment"
	"go.uber.org/fx"
	"google.golang.org/grpc"
)

var Module = fx.Options(
	migration.Module,
	resource.Module,
	deletenamespace.Module,
	scheduler.Module,
	batcher.Module,
	workerdeployment.Module,
	wcicomponent.Module,
	dlq.Module,
	dummy.Module,
	fx.Provide(schedulerpb.NewSchedulerServiceLayeredClient),
	fx.Provide(
		func(c resource.HistoryClient) dlq.HistoryClient {
			return c
		},
		func(m cluster.Metadata) dlq.CurrentClusterName {
			return dlq.CurrentClusterName(m.GetCurrentClusterName())
		},
		func(b client.Bean) dlq.TaskClientDialer {
			return dlq.TaskClientDialerFn(func(_ context.Context, address string) (dlq.TaskClient, error) {
				c, err := b.GetRemoteAdminClient(address)
				if err != nil {
					return nil, err
				}
				return dlq.AddTasksFn(func(
					ctx context.Context,
					req *adminservice.AddTasksRequest,
				) (*adminservice.AddTasksResponse, error) {
					return c.AddTasks(ctx, req)
				}), nil
			})
		},
	),
	fx.Provide(HostInfoProvider),
	fx.Provide(VisibilityManagerProvider),
	fx.Provide(ThrottledLoggerRpsFnProvider),
	fx.Provide(ConfigProvider),
	fx.Provide(PersistenceRateLimitingParamsProvider),
	service.PersistenceLazyLoadedServiceResolverModule,
	fx.Provide(ServiceResolverProvider),
	fx.Provide(func(
		clusterMetadata cluster.Metadata,
		metadataManager persistence.MetadataManager,
		dataMerger nsreplication.NamespaceDataMerger,
		logger log.Logger,
	) nsreplication.TaskExecutor {
		return nsreplication.NewTaskExecutor(
			clusterMetadata.GetCurrentClusterName(),
			metadataManager,
			dataMerger,
			logger,
		)
	}),
	fx.Provide(nsreplication.NewNoopDataMerger),
	fx.Provide(ServerProvider),
	fx.Provide(NewService),
	fx.Provide(fx.Annotate(NewWorkerManager, fx.ParamTags(workercommon.WorkerComponentTag))),
	fx.Provide(PerNamespaceWorkerManagerProvider),
	fx.Invoke(ServiceLifetimeHooks),
)

func ThrottledLoggerRpsFnProvider(serviceConfig *Config) resource.ThrottledLoggerRpsFn {
	return func() float64 { return float64(serviceConfig.ThrottledLogRPS()) }
}

func PersistenceRateLimitingParamsProvider(
	serviceConfig *Config,
	persistenceLazyLoadedServiceResolver service.PersistenceLazyLoadedServiceResolver,
	logger log.SnTaggedLogger,
) service.PersistenceRateLimitingParams {
	return service.NewPersistenceRateLimitingParams(
		serviceConfig.PersistenceMaxQPS,
		serviceConfig.PersistenceGlobalMaxQPS,
		serviceConfig.PersistenceNamespaceMaxQPS,
		serviceConfig.PersistenceGlobalNamespaceMaxQPS,
		serviceConfig.PersistencePerShardNamespaceMaxQPS,
		serviceConfig.OperatorRPSRatio,
		serviceConfig.PersistenceQPSBurstRatio,
		serviceConfig.PersistenceDynamicRateLimitingParams,
		persistenceLazyLoadedServiceResolver,
		logger,
	)
}

func HostInfoProvider() (membership.HostInfo, error) {
	hn, err := os.Hostname()
	return membership.NewHostInfoFromAddress(hn), err
}

func ServiceResolverProvider(
	membershipMonitor membership.Monitor,
) (membership.ServiceResolver, error) {
	return membershipMonitor.GetResolver(primitives.WorkerService)
}

func ConfigProvider(
	dc *dynamicconfig.Collection,
	persistenceConfig *config.Persistence,
) *Config {
	return NewConfig(
		dc,
		persistenceConfig,
	)
}

func VisibilityManagerProvider(
	logger log.Logger,
	metricsHandler metrics.Handler,
	persistenceConfig *config.Persistence,
	customVisibilityStoreFactory visibility.VisibilityStoreFactory,
	serviceConfig *Config,
	persistenceServiceResolver resolver.ServiceResolver,
	searchAttributesMapperProvider searchattribute.MapperProvider,
	saProvider searchattribute.Provider,
	namespaceRegistry namespace.Registry,
	chasmRegistry *chasm.Registry,
	serializer serialization.Serializer,
) (manager.VisibilityManager, error) {
	return visibility.NewManager(
		*persistenceConfig,
		persistenceServiceResolver,
		customVisibilityStoreFactory,
		nil, // worker visibility never write
		saProvider,
		searchAttributesMapperProvider,
		namespaceRegistry,
		chasmRegistry,
		serviceConfig.VisibilityPersistenceMaxReadQPS,
		serviceConfig.VisibilityPersistenceMaxWriteQPS,
		serviceConfig.OperatorRPSRatio,
		serviceConfig.VisibilityPersistenceSlowQueryThreshold,
		serviceConfig.EnableReadFromSecondaryVisibility,
		serviceConfig.VisibilityEnableShadowReadMode,
		dynamicconfig.GetStringPropertyFn(visibility.SecondaryVisibilityWritingModeOff), // worker visibility never write
		serviceConfig.VisibilityDisableOrderByClause,
		serviceConfig.VisibilityEnableManualPagination,
		serviceConfig.VisibilityEnableUnifiedQueryConverter,
		metricsHandler,
		logger,
		serializer,
	)
}

func ServiceLifetimeHooks(lc fx.Lifecycle, svc *Service) {
	lc.Append(fx.StartStopHook(svc.Start, svc.Stop))
}

type perNamespaceWorkerManagerInitParams struct {
	fx.In
	Logger            log.Logger
	SdkClientFactory  sdk.ClientFactory
	NamespaceRegistry namespace.Registry
	HostName          resource.HostName
	Config            *Config
	ClusterMetadata   cluster.Metadata
	Components        []workercommon.PerNSWorkerComponent `group:"perNamespaceWorkerComponent"`
}

func PerNamespaceWorkerManagerProvider(params perNamespaceWorkerManagerInitParams) *PerNamespaceWorkerManager {
	return NewPerNamespaceWorkerManager(
		params.Logger,
		params.SdkClientFactory,
		params.NamespaceRegistry,
		params.HostName,
		params.Config,
		params.ClusterMetadata,
		params.Components,
		primitives.PerNSWorkerTaskQueue,
	)
}

func ServerProvider(rpcFactory common.RPCFactory, logger log.Logger) *grpc.Server {
	opts, err := rpcFactory.GetInternodeGRPCServerOptions()
	if err != nil {
		logger.Fatal("Failed to get gRPC server options", tag.Error(err))
	}
	return grpc.NewServer(opts...)
}
