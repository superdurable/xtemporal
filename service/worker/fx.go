package worker

import (
	"context"
	"os"

	wcicomponent "github.com/superdurable/xtemporal/components/autoscaledworkers/wci/workercomponent"
	"github.com/superdurable/xtemporal/api/adminservice/v1"
	"github.com/superdurable/xtemporal/chasm"
	"github.com/superdurable/xtemporal/chasm/lib/callback"
	chasmscheduler "github.com/superdurable/xtemporal/chasm/lib/scheduler"
	"github.com/superdurable/xtemporal/chasm/lib/scheduler/gen/schedulerpb/v1"
	"github.com/superdurable/xtemporal/client"
	"github.com/superdurable/xtemporal/common"
	"github.com/superdurable/xtemporal/common/cluster"
	"github.com/superdurable/xtemporal/common/config"
	"github.com/superdurable/xtemporal/common/dynamicconfig"
	"github.com/superdurable/xtemporal/common/log"
	"github.com/superdurable/xtemporal/common/log/tag"
	"github.com/superdurable/xtemporal/common/membership"
	"github.com/superdurable/xtemporal/common/metrics"
	"github.com/superdurable/xtemporal/common/namespace"
	"github.com/superdurable/xtemporal/common/namespace/nsreplication"
	"github.com/superdurable/xtemporal/common/persistence"
	"github.com/superdurable/xtemporal/common/persistence/serialization"
	"github.com/superdurable/xtemporal/common/persistence/visibility"
	"github.com/superdurable/xtemporal/common/persistence/visibility/manager"
	"github.com/superdurable/xtemporal/common/primitives"
	"github.com/superdurable/xtemporal/common/resolver"
	"github.com/superdurable/xtemporal/common/resource"
	"github.com/superdurable/xtemporal/common/sdk"
	"github.com/superdurable/xtemporal/common/searchattribute"
	"github.com/superdurable/xtemporal/common/testing/testhooks"
	"github.com/superdurable/xtemporal/service"
	"github.com/superdurable/xtemporal/service/worker/batcher"
	workercommon "github.com/superdurable/xtemporal/service/worker/common"
	"github.com/superdurable/xtemporal/service/worker/deletenamespace"
	"github.com/superdurable/xtemporal/service/worker/dlq"
	"github.com/superdurable/xtemporal/service/worker/dummy"
	"github.com/superdurable/xtemporal/service/worker/migration"
	"github.com/superdurable/xtemporal/service/worker/scheduler"
	"github.com/superdurable/xtemporal/service/worker/workerdeployment"
	"go.uber.org/fx"
	"google.golang.org/grpc"
)

var Module = fx.Options(
	migration.Module,
	resource.Module,
	deletenamespace.Module,
	chasmscheduler.Module,
	callback.Module,
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
		admitter nsreplication.NamespaceReplicationAdmitter,
		logger log.Logger,
		testHooks testhooks.TestHooks,
	) nsreplication.TaskExecutor {
		return nsreplication.NewTaskExecutor(
			clusterMetadata.GetCurrentClusterName(),
			metadataManager,
			dataMerger,
			admitter,
			logger,
			testHooks,
		)
	}),
	fx.Provide(nsreplication.NewNoopDataMerger),
	fx.Provide(nsreplication.NewDefaultAdmitter),
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
