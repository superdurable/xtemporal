package history

import (
	"context"
	"time"

	"github.com/nexus-rpc/sdk-go/nexus"
	"github.com/superdurable/xtemporal/api/historyservice/v1"
	"github.com/superdurable/xtemporal/chasm"
	"github.com/superdurable/xtemporal/chasm/lib/activity"
	"github.com/superdurable/xtemporal/chasm/lib/callback"
	chasmnexus "github.com/superdurable/xtemporal/chasm/lib/nexusoperation"
	"github.com/superdurable/xtemporal/chasm/lib/scheduler"
	chasmtests "github.com/superdurable/xtemporal/chasm/lib/tests"
	chasmworkflow "github.com/superdurable/xtemporal/chasm/lib/workflow"
	"github.com/superdurable/xtemporal/common"
	commoncache "github.com/superdurable/xtemporal/common/cache"
	"github.com/superdurable/xtemporal/common/clock"
	"github.com/superdurable/xtemporal/common/config"
	"github.com/superdurable/xtemporal/common/dynamicconfig"
	"github.com/superdurable/xtemporal/common/log"
	"github.com/superdurable/xtemporal/common/log/tag"
	"github.com/superdurable/xtemporal/common/membership"
	"github.com/superdurable/xtemporal/common/metrics"
	"github.com/superdurable/xtemporal/common/namespace"
	persistenceClient "github.com/superdurable/xtemporal/common/persistence/client"
	"github.com/superdurable/xtemporal/common/persistence/serialization"
	"github.com/superdurable/xtemporal/common/persistence/visibility"
	"github.com/superdurable/xtemporal/common/persistence/visibility/manager"
	"github.com/superdurable/xtemporal/common/persistence/visibility/store/elasticsearch"
	"github.com/superdurable/xtemporal/common/primitives"
	"github.com/superdurable/xtemporal/common/quotas/calculator"
	"github.com/superdurable/xtemporal/common/resolver"
	"github.com/superdurable/xtemporal/common/resource"
	"github.com/superdurable/xtemporal/common/rpc/interceptor"
	"github.com/superdurable/xtemporal/common/searchattribute"
	"github.com/superdurable/xtemporal/common/tasktoken"
	"github.com/superdurable/xtemporal/common/testing/testhooks"
	"github.com/superdurable/xtemporal/common/worker_versioning"
	"github.com/superdurable/xtemporal/components/callbacks"
	hsmnexusoperations "github.com/superdurable/xtemporal/components/nexusoperations"
	hsmnexusworkflow "github.com/superdurable/xtemporal/components/nexusoperations/workflow"
	"github.com/superdurable/xtemporal/service"
	"github.com/superdurable/xtemporal/service/history/api"
	"github.com/superdurable/xtemporal/service/history/archival"
	"github.com/superdurable/xtemporal/service/history/configs"
	"github.com/superdurable/xtemporal/service/history/consts"
	"github.com/superdurable/xtemporal/service/history/events"
	"github.com/superdurable/xtemporal/service/history/hsm"
	"github.com/superdurable/xtemporal/service/history/replication"
	"github.com/superdurable/xtemporal/service/history/shard"
	"github.com/superdurable/xtemporal/service/history/workflow"
	"github.com/superdurable/xtemporal/service/history/workflow/cache"
	"github.com/superdurable/xtemporal/service/worker/workerdeployment"
	"go.uber.org/fx"
	"google.golang.org/grpc"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

var Module = fx.Options(
	resource.Module,
	fx.Provide(resource.SearchAttributeValidatorProvider),
	fx.Provide(hsm.NewRegistry),
	workflow.Module,

	shard.Module,
	events.Module,
	cache.Module,
	archival.Module,
	ChasmEngineModule,
	chasmtests.Module,
	fx.Provide(ConfigProvider), // might be worth just using provider for configs.Config directly
	fx.Provide(workflow.NewCommandHandlerRegistry),
	fx.Provide(ServiceErrorInterceptorProvider),
	fx.Provide(RetryableInterceptorProvider),
	fx.Provide(ErrorHandlerProvider),
	fx.Provide(TelemetryInterceptorProvider),
	fx.Provide(NamespaceRateLimitInterceptorProvider),
	fx.Provide(RateLimitInterceptorProvider),
	fx.Provide(HealthSignalAggregatorProvider),
	fx.Provide(HealthCheckInterceptorProvider),
	fx.Provide(ContextMetadataInterceptorProvider),
	fx.Provide(chasm.ChasmEngineInterceptorProvider),
	fx.Provide(chasm.ChasmVisibilityInterceptorProvider),
	fx.Provide(HistoryAdditionalInterceptorsProvider),
	fx.Provide(service.GrpcServerOptionsProvider),
	fx.Provide(ESProcessorConfigProvider),
	fx.Provide(VisibilityManagerProvider),
	fx.Provide(visibility.ChasmVisibilityManagerProvider),
	fx.Provide(ThrottledLoggerRpsFnProvider),
	fx.Provide(PersistenceRateLimitingParamsProvider),
	service.PersistenceLazyLoadedServiceResolverModule,
	fx.Provide(ServiceResolverProvider),
	fx.Provide(EventNotifierProvider),
	fx.Provide(HistoryEngineFactoryProvider),
	fx.Provide(HandlerProvider),
	fx.Provide(HistoryServiceServerProvider),
	fx.Provide(ServerProvider),
	fx.Provide(NewService),
	fx.Provide(ReplicationProgressCacheProvider),
	fx.Provide(VersionMembershipCacheProvider),
	workerdeployment.ClientModule,
	fx.Provide(RoutingInfoCacheProvider),
	fx.Invoke(ServiceLifetimeHooks),
	fx.Invoke(func(
		chasmEngine chasm.Engine,
		chasmVisibilityManager chasm.VisibilityManager,
		chasmRegistry *chasm.Registry,
		testHooks testhooks.TestHooks,
	) {
		if hook, ok := testhooks.Get(
			testHooks,
			testhooks.HistoryChasmRuntimeProvider,
			testhooks.GlobalScope,
		); ok {
			hook(chasmEngine, chasmVisibilityManager, chasmRegistry)
		}
	}),

	callbacks.Module,
	hsmnexusoperations.Module,
	fx.Invoke(hsmnexusworkflow.RegisterCommandHandlers),
	activity.HistoryModule,
	scheduler.Module,
	callback.Module,
	chasmnexus.Module,
	chasmworkflow.Module,
	chasmworkflow.HistoryHandlerModule,
)

func ServerProvider(grpcServerOptions []grpc.ServerOption) *grpc.Server {
	return grpc.NewServer(grpcServerOptions...)
}

func HistoryServiceServerProvider(handler *Handler) historyservice.HistoryServiceServer {
	return handler
}

func ServiceResolverProvider(
	membershipMonitor membership.Monitor,
) (membership.ServiceResolver, error) {
	return membershipMonitor.GetResolver(primitives.HistoryService)
}

func HandlerProvider(args NewHandlerArgs, lc fx.Lifecycle) (*Handler, error) {
	handler := &Handler{
		status:                 common.DaemonStatusInitialized,
		config:                 args.Config,
		nexusCompletionHandler: args.NexusCompletionHandler,
		tokenSerializer:        tasktoken.NewSerializer(),
		deepHealthCheckHandler: deepHealthCheckHandler{
			healthServer:            args.HealthServer,
			metricsHandler:          args.MetricsHandler,
			config:                  args.Config,
			historyHealthSignal:     args.HistoryHealthSignal,
			persistenceHealthSignal: args.PersistenceHealthSignal,
			startupTime:             time.Now(),
		},
		logger:                       args.Logger,
		throttledLogger:              args.ThrottledLogger,
		persistenceExecutionManager:  args.PersistenceExecutionManager,
		persistenceShardManager:      args.PersistenceShardManager,
		persistenceVisibilityManager: args.PersistenceVisibilityManager,
		historyServiceResolver:       args.HistoryServiceResolver,
		metricsHandler:               args.MetricsHandler,
		payloadSerializer:            args.PayloadSerializer,
		timeSource:                   args.TimeSource,
		namespaceRegistry:            args.NamespaceRegistry,
		saProvider:                   args.SaProvider,
		clusterMetadata:              args.ClusterMetadata,
		archivalMetadata:             args.ArchivalMetadata,
		hostInfoProvider:             args.HostInfoProvider,
		controller:                   args.ShardController,
		eventNotifier:                args.EventNotifier,
		tracer:                       args.TracerProvider.Tracer(consts.LibraryName),
		taskQueueManager:             args.TaskQueueManager,
		taskCategoryRegistry:         args.TaskCategoryRegistry,
		dlqMetricsEmitter:            args.DLQMetricsEmitter,
		chasmEngine:                  args.ChasmEngine,
		chasmRegistry:                args.ChasmRegistry,
		testHooks:                    args.TestHooks,

		replicationTaskFetcherFactory:    args.ReplicationTaskFetcherFactory,
		replicationTaskConverterProvider: args.ReplicationTaskConverterFactory,
		streamReceiverMonitor:            args.StreamReceiverMonitor,
		replicationServerRateLimiter:     args.ReplicationServerRateLimiter,
	}

	// Build the Nexus handler in OnStart rather than here so that it runs after all
	// fx.Invoke functions have completed. If we built it eagerly, the dependency chain
	//
	//   activity.HistoryModule (fx.Invoke)
	//     → *library → *handler → historyservice.HistoryServiceServer
	//       → HistoryServiceServerProvider → HandlerProvider (this function)
	//
	// would force HandlerProvider to run before modules like chasmtests.Module have had
	// a chance to register their nexus services via their own fx.Invoke calls. As a
	// result, buildNexusHandler would snapshot an empty registry and h.nexusHandler
	// would remain nil, causing all StartNexusOperation calls to the system endpoint to
	// return "no nexus services registered". OnStart hooks run after ALL invokes are
	// done, so the registry is fully populated by the time we call buildNexusHandler.
	lc.Append(fx.Hook{
		OnStart: func(_ context.Context) error {
			h, err := buildNexusHandler(args.ChasmRegistry)
			if err != nil {
				return err
			}
			handler.nexusHandler = h
			return nil
		},
	})

	return handler, nil
}

func buildNexusHandler(chasmRegistry *chasm.Registry) (nexus.Handler, error) {
	nexusServices := chasmRegistry.NexusServices()
	if len(nexusServices) == 0 {
		return nil, nil
	}
	serviceRegistry := nexus.NewServiceRegistry()
	for _, svc := range nexusServices {
		// No chance of collision here since the registry would have errored out earlier.
		serviceRegistry.MustRegister(svc)
	}

	return serviceRegistry.NewHandler()
}

func HistoryEngineFactoryProvider(
	params HistoryEngineFactoryParams,
) shard.EngineFactory {
	return &historyEngineFactory{
		HistoryEngineFactoryParams: params,
	}
}

func ConfigProvider(
	dc *dynamicconfig.Collection,
	persistenceConfig config.Persistence,
) *configs.Config {
	return configs.NewConfig(
		dc,
		persistenceConfig.NumHistoryShards,
	)
}

func ServiceErrorInterceptorProvider(
	dc *dynamicconfig.Collection,
	metricsHandler metrics.Handler,
	logger log.Logger,
) *interceptor.ServiceErrorInterceptor {
	return interceptor.NewServiceErrorInterceptor(
		dynamicconfig.MaxServiceErrorMessageLength.Get(dc),
		metricsHandler,
		logger,
	)
}

func ThrottledLoggerRpsFnProvider(serviceConfig *configs.Config) resource.ThrottledLoggerRpsFn {
	return func() float64 { return float64(serviceConfig.ThrottledLogRPS()) }
}

func RetryableInterceptorProvider() *interceptor.RetryableInterceptor {
	return interceptor.NewRetryableInterceptor(
		common.CreateHistoryHandlerRetryPolicy(),
		api.IsRetryableError,
	)
}

func ErrorHandlerProvider(
	logger log.Logger,
	serviceConfig *configs.Config,
) *interceptor.RequestErrorHandler {
	return interceptor.NewRequestErrorHandler(
		logger,
		serviceConfig.LogAllReqErrors,
	)
}

func TelemetryInterceptorProvider(
	logger log.Logger,
	namespaceRegistry namespace.Registry,
	metricsHandler metrics.Handler,
	serviceConfig *configs.Config,
	requestErrorHandler *interceptor.RequestErrorHandler,
) *interceptor.TelemetryInterceptor {
	return interceptor.NewTelemetryInterceptor(
		namespaceRegistry,
		metricsHandler,
		logger,
		serviceConfig.LogAllReqErrors,
		requestErrorHandler,
	)
}

func HealthSignalAggregatorProvider(
	lc fx.Lifecycle,
	dynamicCollection *dynamicconfig.Collection,
	logger log.ThrottledLogger,
) interceptor.HealthSignalAggregator {
	aggregator := interceptor.NewHealthSignalAggregator(
		logger,
		dynamicconfig.HistoryHealthSignalMetricsEnabled.Get(dynamicCollection),
		dynamicconfig.HistoryHealthSignalUsePercentiles.Get(dynamicCollection),
		dynamicconfig.HealthCheckHistoryGRPCSettings.Get(dynamicCollection),
		dynamicconfig.PersistenceHealthSignalWindowSize.Get(dynamicCollection)(),
		dynamicconfig.PersistenceHealthSignalBufferSize.Get(dynamicCollection)(),
	)
	lc.Append(fx.StopHook(aggregator.Stop))

	return aggregator
}

func HealthCheckInterceptorProvider(
	healthSignalAggregator interceptor.HealthSignalAggregator,
) *interceptor.HealthCheckInterceptor {
	return interceptor.NewHealthCheckInterceptor(
		healthSignalAggregator,
	)
}

func ContextMetadataInterceptorProvider(logger log.Logger) *interceptor.ContextMetadataInterceptor {
	return interceptor.NewContextMetadataInterceptor(true, logger)
}

func HistoryAdditionalInterceptorsProvider(
	healthCheckInterceptor *interceptor.HealthCheckInterceptor,
	chasmRequestEngineInterceptor *chasm.ChasmEngineInterceptor,
	chasmRequestVisibilityInterceptor *chasm.ChasmVisibilityInterceptor,
) []grpc.UnaryServerInterceptor {
	return []grpc.UnaryServerInterceptor{
		healthCheckInterceptor.UnaryIntercept,
		chasmRequestEngineInterceptor.Intercept,
		chasmRequestVisibilityInterceptor.Intercept,
	}
}

func NamespaceRateLimitInterceptorProvider(
	serviceConfig *configs.Config,
	namespaceRegistry namespace.Registry,
	metricsHandler metrics.Handler,
) interceptor.NamespaceRateLimitInterceptor {

	namespaceRateFn := func(namespaceName string) float64 {
		if namespaceRPS := serviceConfig.NamespaceRPS(namespaceName); namespaceRPS > 0 {
			return float64(namespaceRPS)
		}
		// This fallback to host level rps limit when NamespaceRPS is not configured (i.e. 0)
		return float64(serviceConfig.RPS())
	}

	return interceptor.NewNamespaceRateLimitInterceptor(
		namespaceRegistry,
		configs.NewNamespaceRateLimiter(
			namespaceRateFn,
			serviceConfig.OperatorRPSRatio,
		),
		map[string]int{},      // no token overrides
		map[string]struct{}{}, // no long polls on history service
		dynamicconfig.GetBoolPropertyFnFilteredByNamespace(false), // no long poll methods
		metricsHandler,
	)
}

func RateLimitInterceptorProvider(
	serviceConfig *configs.Config,
) *interceptor.RateLimitInterceptor {
	return interceptor.NewRateLimitInterceptor(
		configs.NewPriorityRateLimiter(func() float64 { return float64(serviceConfig.RPS()) }, serviceConfig.OperatorRPSRatio),
		map[string]int{
			healthpb.Health_Check_FullMethodName:                         0, // exclude health check requests from rate limiting.
			historyservice.HistoryService_DeepHealthCheck_FullMethodName: 0, // exclude deep health check requests from rate limiting.
		},
	)
}

func ESProcessorConfigProvider(
	serviceConfig *configs.Config,
) *elasticsearch.ProcessorConfig {
	return &elasticsearch.ProcessorConfig{
		IndexerConcurrency:       serviceConfig.IndexerConcurrency,
		ESProcessorNumOfWorkers:  serviceConfig.ESProcessorNumOfWorkers,
		ESProcessorBulkActions:   serviceConfig.ESProcessorBulkActions,
		ESProcessorBulkSize:      serviceConfig.ESProcessorBulkSize,
		ESProcessorFlushInterval: serviceConfig.ESProcessorFlushInterval,
		ESProcessorAckTimeout:    serviceConfig.ESProcessorAckTimeout,
	}
}

func PersistenceRateLimitingParamsProvider(
	serviceConfig *configs.Config,
	persistenceLazyLoadedServiceResolver service.PersistenceLazyLoadedServiceResolver,
	ownershipBasedQuotaScaler shard.LazyLoadedOwnershipBasedQuotaScaler,
	logger log.SnTaggedLogger,
) service.PersistenceRateLimitingParams {
	hostCalculator := calculator.NewLoggedCalculator(
		shard.NewOwnershipAwareQuotaCalculator(
			ownershipBasedQuotaScaler,
			persistenceLazyLoadedServiceResolver,
			serviceConfig.PersistenceMaxQPS,
			serviceConfig.PersistenceGlobalMaxQPS,
		),
		log.With(logger, tag.ComponentPersistence, tag.ScopeHost),
	)
	namespaceCalculator := calculator.NewLoggedNamespaceCalculator(
		shard.NewOwnershipAwareNamespaceQuotaCalculator(
			ownershipBasedQuotaScaler,
			persistenceLazyLoadedServiceResolver,
			serviceConfig.PersistenceNamespaceMaxQPS,
			serviceConfig.PersistenceGlobalNamespaceMaxQPS,
		),
		log.With(logger, tag.ComponentPersistence, tag.ScopeNamespace),
	)
	return service.PersistenceRateLimitingParams{
		PersistenceMaxQps: func() int {
			return int(hostCalculator.GetQuota())
		},
		PersistenceNamespaceMaxQps: func(namespace string) int {
			return int(namespaceCalculator.GetQuota(namespace))
		},
		PersistencePerShardNamespaceMaxQPS: persistenceClient.PersistencePerShardNamespaceMaxQPS(serviceConfig.PersistencePerShardNamespaceMaxQPS),
		OperatorRPSRatio:                   persistenceClient.OperatorRPSRatio(serviceConfig.OperatorRPSRatio),
		PersistenceBurstRatio:              persistenceClient.PersistenceBurstRatio(serviceConfig.PersistenceQPSBurstRatio),
		DynamicRateLimitingParams:          persistenceClient.DynamicRateLimitingParams(serviceConfig.PersistenceDynamicRateLimitingParams),
	}
}

func VisibilityManagerProvider(
	logger log.Logger,
	metricsHandler metrics.Handler,
	persistenceConfig *config.Persistence,
	customVisibilityStoreFactory visibility.VisibilityStoreFactory,
	esProcessorConfig *elasticsearch.ProcessorConfig,
	serviceConfig *configs.Config,
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
		esProcessorConfig,
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
		serviceConfig.SecondaryVisibilityWritingMode,
		serviceConfig.VisibilityDisableOrderByClause,
		serviceConfig.VisibilityEnableManualPagination,
		serviceConfig.VisibilityEnableUnifiedQueryConverter,
		metricsHandler,
		logger,
		serializer,
	)
}

func ChasmVisibilityManagerProvider(
	chasmRegistry *chasm.Registry,
	nsRegistry namespace.Registry,
	visibilityManager manager.VisibilityManager,
) chasm.VisibilityManager {
	return visibility.NewChasmVisibilityManager(
		chasmRegistry,
		nsRegistry,
		visibilityManager,
	)
}

func EventNotifierProvider(
	timeSource clock.TimeSource,
	metricsHandler metrics.Handler,
	config *configs.Config,
) events.Notifier {
	return events.NewNotifier(
		timeSource,
		metricsHandler,
		config.GetShardID,
	)
}

func ServiceLifetimeHooks(lc fx.Lifecycle, svc *Service) {
	lc.Append(fx.StartStopHook(svc.Start, svc.Stop))
}

func ReplicationProgressCacheProvider(
	serviceConfig *configs.Config,
	logger log.Logger,
	handler metrics.Handler,
) replication.ProgressCache {
	return replication.NewProgressCache(serviceConfig, logger, handler)
}

func VersionMembershipCacheProvider(
	lc fx.Lifecycle,
	serviceConfig *configs.Config,
	metricsHandler metrics.Handler,
) worker_versioning.VersionMembershipAndReactivationStatusCache {
	c := commoncache.New(serviceConfig.VersionMembershipCacheMaxSize(), &commoncache.Options{
		TTL: max(1*time.Second, serviceConfig.VersionMembershipCacheTTL()),
	})
	lc.Append(fx.Hook{
		OnStop: func(context.Context) error {
			c.Stop()
			return nil
		},
	})
	return worker_versioning.NewVersionMembershipAndReactivationStatusCache(c, metricsHandler)
}

func RoutingInfoCacheProvider(
	lc fx.Lifecycle,
	serviceConfig *configs.Config,
	metricsHandler metrics.Handler,
) worker_versioning.RoutingInfoCache {
	c := commoncache.New(serviceConfig.RoutingInfoCacheMaxSize(), &commoncache.Options{
		TTL: max(1*time.Second, serviceConfig.RoutingInfoCacheTTL()),
	})
	lc.Append(fx.Hook{
		OnStop: func(context.Context) error {
			c.Stop()
			return nil
		},
	})
	return worker_versioning.NewRoutingInfoCache(c, metricsHandler)
}
