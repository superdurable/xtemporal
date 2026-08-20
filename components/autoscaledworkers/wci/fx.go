// Package wci contains the implementation for the worker controller instances
package wci

import (
	"github.com/superdurable/temporal-server/common"
	"github.com/superdurable/temporal-server/common/cluster"
	"github.com/superdurable/temporal-server/common/config"
	"github.com/superdurable/temporal-server/common/dynamicconfig"
	"github.com/superdurable/temporal-server/common/log"
	"github.com/superdurable/temporal-server/common/membership"
	"github.com/superdurable/temporal-server/common/metrics"
	"github.com/superdurable/temporal-server/common/namespace"
	"github.com/superdurable/temporal-server/common/namespace/nsregistry"
	"github.com/superdurable/temporal-server/common/persistence"
	persistenceclient "github.com/superdurable/temporal-server/common/persistence/client"
	"github.com/superdurable/temporal-server/common/persistence/serialization"
	"github.com/superdurable/temporal-server/common/pprof"
	"github.com/superdurable/temporal-server/common/resource"
	"github.com/superdurable/temporal-server/common/sdk"
	"github.com/superdurable/temporal-server/components/autoscaledworkers/wci/client"
	"github.com/superdurable/temporal-server/components/autoscaledworkers/wci/workercomponent"
	"github.com/superdurable/temporal-server/service/worker"
	workercommon "github.com/superdurable/temporal-server/service/worker/common"
	"github.com/superdurable/temporal-server/temporal"
	"go.uber.org/fx"
)

var Module = fx.Options(
	serialization.Module,
	temporal.ServiceTracingModule,
	dynamicconfig.Module,
	pprof.Module,
	resource.DefaultOptions,
	fx.Provide(resource.HostNameProvider),
	fx.Provide(ThrottledLoggerRpsFnProvider),
	membership.GRPCResolverModule,

	fx.Provide(persistenceclient.ClusterNameProvider),
	fx.Provide(persistenceclient.HealthSignalAggregatorProvider),
	fx.Provide(persistenceclient.DataStoreFactoryProvider),
	fx.Provide(persistenceclient.EnableDataLossMetricsProvider),
	fx.Invoke(persistenceclient.DataStoreFactoryLifetimeHooks),
	fx.Provide(newMetadataManager, newClusterMetadataManager),

	fx.Provide(resource.ClientFactoryProvider),
	fx.Provide(namespace.NewDefaultReplicationResolverFactory),
	fx.Provide(resource.NamespaceRegistryProvider),
	nsregistry.RegistryLifetimeHooksModule,
	cluster.MetadataLifetimeHooksModule,

	fx.Provide(ConfigProvider),
	fx.Provide(WorkerConfigProvider),
	fx.Provide(ServiceResolverProvider),
	fx.Provide(NewService),
	fx.Provide(PerNamespaceWorkerManagerProvider),
	fx.Invoke(ServiceLifetimeHooks),
)

func ServiceResolverProvider(
	membershipMonitor membership.Monitor,
) (membership.ServiceResolver, error) {
	return membershipMonitor.GetResolver(ServiceName)
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

func WorkerConfigProvider(config *Config) *worker.Config {
	return &config.Config
}

func newMetadataManager(
	dataStoreFactory persistence.DataStoreFactory,
	serializer serialization.Serializer,
	logger log.Logger,
	clusterName persistenceclient.ClusterName,
	metricsHandler metrics.Handler,
	healthSignals persistence.HealthSignalAggregator,
	enableDataLossMetrics persistenceclient.EnableDataLossMetrics,
) (persistence.MetadataManager, error) {
	store, err := dataStoreFactory.NewMetadataStore()
	if err != nil {
		return nil, err
	}

	result := persistence.NewMetadataManagerImpl(store, serializer, logger, string(clusterName))
	// if f.systemRateLimiter != nil && f.namespaceRateLimiter != nil {
	// 	result = persistence.NewMetadataPersistenceRateLimitedClient(result, f.systemRateLimiter, f.namespaceRateLimiter, f.shardRateLimiter, f.logger)
	// }
	if metricsHandler != nil && healthSignals != nil {
		result = persistence.NewMetadataPersistenceMetricsClient(result, metricsHandler, healthSignals, logger, dynamicconfig.BoolPropertyFn(enableDataLossMetrics))
	}
	result = persistence.NewMetadataPersistenceRetryableClient(result, common.CreatePersistenceClientRetryPolicy(), persistenceclient.IsPersistenceTransientError)
	return result, nil
}

func newClusterMetadataManager(
	dataStoreFactory persistence.DataStoreFactory,
	serializer serialization.Serializer,
	logger log.Logger,
	clusterName persistenceclient.ClusterName,
	metricsHandler metrics.Handler,
	healthSignals persistence.HealthSignalAggregator,
	enableDataLossMetrics persistenceclient.EnableDataLossMetrics,
) (persistence.ClusterMetadataManager, error) {
	store, err := dataStoreFactory.NewClusterMetadataStore()
	if err != nil {
		return nil, err
	}

	result := persistence.NewClusterMetadataManagerImpl(store, serializer, string(clusterName), logger)
	// if f.systemRateLimiter != nil && f.namespaceRateLimiter != nil {
	// 	result = persistence.NewClusterMetadataPersistenceRateLimitedClient(result, f.systemRateLimiter, f.namespaceRateLimiter, f.shardRateLimiter, f.logger)
	// }
	if metricsHandler != nil && healthSignals != nil {
		result = persistence.NewClusterMetadataPersistenceMetricsClient(result, metricsHandler, healthSignals, logger, dynamicconfig.BoolPropertyFn(enableDataLossMetrics))
	}
	result = persistence.NewClusterMetadataPersistenceRetryableClient(result, common.CreatePersistenceClientRetryPolicy(), persistenceclient.IsPersistenceTransientError)
	return result, nil
}

func ThrottledLoggerRpsFnProvider(serviceConfig *worker.Config) resource.ThrottledLoggerRpsFn {
	return func() float64 { return float64(serviceConfig.ThrottledLogRPS()) }
}

func PerNamespaceWorkerManagerProvider(
	logger log.Logger,
	sdkClientFactory sdk.ClientFactory,
	namespaceRegistry namespace.Registry,
	hostName resource.HostName,
	config *worker.Config,
	clusterMetadata cluster.Metadata,
	dynamicConfig *dynamicconfig.Collection,
) *worker.PerNamespaceWorkerManager {
	components := []workercommon.PerNSWorkerComponent{
		workercomponent.NewWCIPerNSWorkerComponent(dynamicConfig, sdkClientFactory),
	}

	return worker.NewPerNamespaceWorkerManager(
		logger,
		sdkClientFactory,
		namespaceRegistry,
		hostName,
		config,
		clusterMetadata,
		components,
		client.WorkerControllerPerNSWorkerTaskQueue,
	)
}

func ServiceLifetimeHooks(lc fx.Lifecycle, svc *Service) {
	lc.Append(fx.StartStopHook(svc.Start, svc.Stop))
}
