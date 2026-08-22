package resource

import (
	"crypto/tls"
	"fmt"
	"net"
	"os"
	"time"

	"github.com/superdurable/temporal-server/api/adminservice/v1"
	"github.com/superdurable/temporal-server/api/historyservice/v1"
	"github.com/superdurable/temporal-server/api/matchingservice/v1"
	"github.com/superdurable/temporal-server/client"
	"github.com/superdurable/temporal-server/client/admin"
	"github.com/superdurable/temporal-server/client/frontend"
	"github.com/superdurable/temporal-server/client/history"
	"github.com/superdurable/temporal-server/client/matching"
	"github.com/superdurable/temporal-server/common"
	"github.com/superdurable/temporal-server/common/archiver"
	"github.com/superdurable/temporal-server/common/archiver/provider"
	"github.com/superdurable/temporal-server/common/clock"
	"github.com/superdurable/temporal-server/common/cluster"
	"github.com/superdurable/temporal-server/common/config"
	"github.com/superdurable/temporal-server/common/deadlock"
	"github.com/superdurable/temporal-server/common/dynamicconfig"
	"github.com/superdurable/temporal-server/common/log"
	"github.com/superdurable/temporal-server/common/log/tag"
	"github.com/superdurable/temporal-server/common/membership"
	"github.com/superdurable/temporal-server/common/metrics"
	"github.com/superdurable/temporal-server/common/namespace"
	"github.com/superdurable/temporal-server/common/namespace/nsregistry"
	commonnexus "github.com/superdurable/temporal-server/common/nexus"
	"github.com/superdurable/temporal-server/common/persistence"
	persistenceClient "github.com/superdurable/temporal-server/common/persistence/client"
	"github.com/superdurable/temporal-server/common/persistence/serialization"
	"github.com/superdurable/temporal-server/common/persistence/visibility"
	"github.com/superdurable/temporal-server/common/persistence/visibility/manager"
	"github.com/superdurable/temporal-server/common/pingable"
	"github.com/superdurable/temporal-server/common/primitives"
	"github.com/superdurable/temporal-server/common/quotas"
	"github.com/superdurable/temporal-server/common/rpc"
	"github.com/superdurable/temporal-server/common/rpc/encryption"
	"github.com/superdurable/temporal-server/common/sdk"
	"github.com/superdurable/temporal-server/common/searchattribute"
	"github.com/superdurable/temporal-server/common/telemetry"
	"github.com/superdurable/temporal-server/common/testing/testhooks"
	"go.temporal.io/api/workflowservice/v1"
	"go.uber.org/fx"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
)

type (
	ThrottledLoggerRpsFn quotas.RateFn
	NamespaceLogger      log.Logger
	HostName             string
	InstanceID           string
	ServiceNames         map[primitives.ServiceName]struct{}

	HistoryRawClient historyservice.HistoryServiceClient
	HistoryClient    historyservice.HistoryServiceClient

	MatchingRawClient matchingservice.MatchingServiceClient
	MatchingClient    matchingservice.MatchingServiceClient

	RuntimeMetricsReporterParams struct {
		fx.In

		MetricHandler metrics.Handler
		Logger        log.SnTaggedLogger
		InstanceID    InstanceID `optional:"true"`
	}
)

// Module
// Use fx.Hook and OnStart/OnStop to manage Daemon resource lifecycle
// See LifetimeHooksModule for detail
var Module = fx.Options(
	persistenceClient.Module,
	dynamicconfig.Module,
	serialization.Module,
	fx.Provide(HostNameProvider),
	fx.Provide(TimeSourceProvider),
	cluster.MetadataLifetimeHooksModule,
	fx.Provide(SearchAttributeMapperProviderProvider),
	fx.Provide(SearchAttributeProviderProvider),
	fx.Provide(SearchAttributeManagerProvider),
	fx.Provide(NamespaceRegistryProvider),
	nsregistry.RegistryLifetimeHooksModule,
	fx.Provide(fx.Annotate(
		func(p namespace.Registry) pingable.Pingable { return p },
		fx.ResultTags(`group:"deadlockDetectorRoots"`),
	)),
	fx.Provide(ClientFactoryProvider),
	fx.Provide(ClientBeanProvider),
	fx.Provide(FrontendClientProvider),
	fx.Provide(AdminClientProvider),
	fx.Provide(GrpcListenerProvider),
	fx.Provide(RuntimeMetricsReporterProvider),
	metrics.RuntimeMetricsReporterLifetimeHooksModule,
	fx.Provide(HistoryRawClientProvider),
	fx.Provide(HistoryClientProvider),
	fx.Provide(MatchingRawClientProvider),
	fx.Provide(MatchingClientProvider),
	membership.GRPCResolverModule,
	fx.Provide(FrontendHTTPClientCacheProvider),
	fx.Provide(PersistenceConfigProvider),
	fx.Provide(health.NewServer),
	fx.Provide(namespace.NewDefaultReplicationResolverFactory),
	deadlock.Module,
	config.Module,
	testhooks.Module,
	fx.Provide(commonnexus.NewLoggedHTTPClientTraceProvider),
)

var DefaultOptions = fx.Options(
	fx.Provide(RPCFactoryProvider),
	fx.Provide(PerServiceDialOptionsProvider),
	fx.Provide(ArchivalMetadataProvider),
	fx.Provide(ArchiverProviderProvider),
	fx.Provide(ThrottledLoggerProvider),
	fx.Provide(SdkClientFactoryProvider),
	fx.Provide(DCRedirectionPolicyProvider),
)

func DefaultSnTaggedLoggerProvider(logger log.Logger, sn primitives.ServiceName) log.SnTaggedLogger {
	return log.With(logger, tag.Service(sn))
}

func ThrottledLoggerProvider(
	logger log.SnTaggedLogger,
	fn ThrottledLoggerRpsFn,
) log.ThrottledLogger {
	return log.NewThrottledLogger(
		logger,
		quotas.RateFn(fn),
	)
}

func GrpcListenerProvider(factory common.RPCFactory) net.Listener {
	return factory.GetGRPCListener()
}

func HostNameProvider() (HostName, error) {
	hn, err := os.Hostname()
	return HostName(hn), err
}

func TimeSourceProvider() clock.TimeSource {
	return clock.NewRealTimeSource()
}

func SearchAttributeMapperProviderProvider(
	saMapper searchattribute.Mapper,
	namespaceRegistry namespace.Registry,
	searchAttributeProvider searchattribute.Provider,
	persistenceConfig *config.Persistence,
) searchattribute.MapperProvider {
	primaryVisibilityStoreConfig := persistenceConfig.GetVisibilityStoreConfig()
	return searchattribute.NewMapperProvider(
		saMapper,
		namespaceRegistry,
		searchAttributeProvider,
		primaryVisibilityStoreConfig.GetIndexName(),
	)
}

func SearchAttributeProviderProvider(
	logger log.SnTaggedLogger,
	timeSource clock.TimeSource,
	cmMgr persistence.ClusterMetadataManager,
	dynamicCollection *dynamicconfig.Collection,
) searchattribute.Provider {
	return searchattribute.NewManager(
		timeSource,
		cmMgr,
		logger,
		dynamicconfig.ForceSearchAttributesCacheRefreshOnRead.Get(dynamicCollection))
}

func SearchAttributeManagerProvider(
	logger log.SnTaggedLogger,
	timeSource clock.TimeSource,
	cmMgr persistence.ClusterMetadataManager,
	dynamicCollection *dynamicconfig.Collection,
) searchattribute.Manager {
	return searchattribute.NewManager(
		timeSource,
		cmMgr,
		logger,
		dynamicconfig.ForceSearchAttributesCacheRefreshOnRead.Get(dynamicCollection))
}

// SearchAttributeValidatorProvider creates a new search attribute validator with the given dependencies. It configures
// the validator with dynamic config values for key limits, value size limits, total size limits, visibility allowlist,
// and system search attribute error suppression.
func SearchAttributeValidatorProvider(
	saProvider searchattribute.Provider,
	saMapperProvider searchattribute.MapperProvider,
	visibilityMgr manager.VisibilityManager,
	dynamicCollection *dynamicconfig.Collection,
) *searchattribute.Validator {
	return searchattribute.NewValidator(
		saProvider,
		saMapperProvider,
		dynamicconfig.SearchAttributesNumberOfKeysLimit.Get(dynamicCollection),
		dynamicconfig.SearchAttributesSizeOfValueLimit.Get(dynamicCollection),
		dynamicconfig.SearchAttributesTotalSizeLimit.Get(dynamicCollection),
		visibilityMgr,
		visibility.AllowListForValidation(
			visibilityMgr.GetStoreNames(),
			dynamicconfig.VisibilityAllowList.Get(dynamicCollection),
		),
		dynamicconfig.SuppressErrorSetSystemSearchAttribute.Get(dynamicCollection),
	)
}

func NamespaceRegistryProvider(
	logger log.SnTaggedLogger,
	metricsHandler metrics.Handler,
	clusterMetadata cluster.Metadata,
	metadataManager persistence.MetadataManager,
	dynamicCollection *dynamicconfig.Collection,
	replicationResolverFactory namespace.ReplicationResolverFactory,
) namespace.Registry {
	return nsregistry.NewRegistry(
		metadataManager,
		clusterMetadata.IsGlobalNamespaceEnabled(),
		clusterMetadata.GetCurrentClusterName(),
		dynamicconfig.NamespaceCacheRefreshInterval.Get(dynamicCollection),
		dynamicconfig.ForceSearchAttributesCacheRefreshOnRead.Get(dynamicCollection),
		metricsHandler,
		logger,
		replicationResolverFactory,
	)
}

func ClientFactoryProvider(
	factoryProvider client.FactoryProvider,
	rpcFactory common.RPCFactory,
	membershipMonitor membership.Monitor,
	metricsHandler metrics.Handler,
	dynamicCollection *dynamicconfig.Collection,
	testHooks testhooks.TestHooks,
	persistenceConfig *config.Persistence,
	clusterMetadata cluster.Metadata,
	logger log.SnTaggedLogger,
	throttledLogger log.ThrottledLogger,
) client.Factory {
	return factoryProvider.NewFactory(
		rpcFactory,
		membershipMonitor,
		metricsHandler,
		dynamicCollection,
		testHooks,
		cluster.CurrentShardCount(clusterMetadata, persistenceConfig.NumHistoryShards),
		logger,
		throttledLogger,
	)
}

func ClientBeanProvider(
	clientFactory client.Factory,
	clusterMetadata cluster.Metadata,
) (client.Bean, error) {
	return client.NewClientBean(
		clientFactory,
		clusterMetadata,
	)
}

func FrontendClientProvider(clientBean client.Bean) workflowservice.WorkflowServiceClient {
	frontendRawClient := clientBean.GetFrontendClient()
	return frontend.NewRetryableClient(
		frontendRawClient,
		common.CreateFrontendClientRetryPolicy(),
		common.IsServiceClientTransientError,
	)
}

func AdminClientProvider(clientBean client.Bean, clusterMetadata cluster.Metadata) (adminservice.AdminServiceClient, error) {
	adminRawClient, err := clientBean.GetRemoteAdminClient(clusterMetadata.GetCurrentClusterName())
	if err != nil {
		return nil, err
	}
	return admin.NewRetryableClient(
		adminRawClient,
		common.CreateFrontendClientRetryPolicy(),
		common.IsServiceClientTransientError,
	), nil
}

func RuntimeMetricsReporterProvider(
	params RuntimeMetricsReporterParams,
) *metrics.RuntimeMetricsReporter {
	return metrics.NewRuntimeMetricsReporter(
		params.MetricHandler,
		time.Minute,
		params.Logger,
		string(params.InstanceID),
	)
}

func HistoryRawClientProvider(clientBean client.Bean) HistoryRawClient {
	return clientBean.GetHistoryClient()
}

func HistoryClientProvider(historyRawClient HistoryRawClient) HistoryClient {
	return history.NewRetryableClient(
		historyRawClient,
		common.CreateHistoryClientRetryPolicy(),
		common.IsServiceClientTransientError,
	)
}

func MatchingRawClientProvider(
	clientBean client.Bean,
	namespaceRegistry namespace.Registry,
) (MatchingRawClient, error) {
	return clientBean.GetMatchingClient(namespaceRegistry.GetNamespaceName)
}

func MatchingClientProvider(matchingRawClient MatchingRawClient) MatchingClient {
	return matching.NewRetryableClient(
		matchingRawClient,
		common.CreateMatchingClientRetryPolicy(),
		common.CreateMatchingClientLongPollRetryPolicy(),
		common.IsServiceClientTransientError,
	)
}

func PersistenceConfigProvider(persistenceConfig config.Persistence, dc *dynamicconfig.Collection) *config.Persistence {
	persistenceConfig.TransactionSizeLimit = dynamicconfig.TransactionSizeLimit.Get(dc)
	return &persistenceConfig
}

func ArchivalMetadataProvider(dc *dynamicconfig.Collection, cfg *config.Config) archiver.ArchivalMetadata {
	return archiver.NewArchivalMetadata(
		dc,
		cfg.Archival.History.State,
		cfg.Archival.History.EnableRead,
		cfg.Archival.Visibility.State,
		cfg.Archival.Visibility.EnableRead,
		&cfg.NamespaceDefaults.Archival,
	)
}

func ArchiverProviderProvider(
	cfg *config.Config,
	customHistoryArchiverFactory provider.CustomHistoryArchiverFactory,
	customVisibilityArchiverFactory provider.CustomVisibilityArchiverFactory,
	persistenceExecutionManager persistence.ExecutionManager,
	logger log.SnTaggedLogger,
	metricsHandler metrics.Handler,
) provider.ArchiverProvider {
	return provider.NewArchiverProvider(
		cfg.Archival.History.Provider,
		cfg.Archival.Visibility.Provider,
		customHistoryArchiverFactory,
		customVisibilityArchiverFactory,
		persistenceExecutionManager,
		logger,
		metricsHandler,
	)
}

func SdkClientFactoryProvider(
	cfg *config.Config,
	tlsConfigProvider encryption.TLSConfigProvider,
	metricsHandler metrics.Handler,
	logger log.SnTaggedLogger,
	resolver *membership.GRPCResolver,
	dc *dynamicconfig.Collection,
) (sdk.ClientFactory, error) {
	frontendURL, _, _, frontendTLSConfig, err := getFrontendConnectionDetails(cfg, tlsConfigProvider, resolver)
	if err != nil {
		return nil, err
	}
	return sdk.NewClientFactory(
		frontendURL,
		frontendTLSConfig,
		metricsHandler,
		logger,
		dynamicconfig.WorkerStickyCacheSize.Get(dc),
	), nil
}

func DCRedirectionPolicyProvider(cfg *config.Config) config.DCRedirectionPolicy {
	return cfg.DCRedirectionPolicy
}

func PerServiceDialOptionsProvider() map[primitives.ServiceName][]grpc.DialOption {
	return map[primitives.ServiceName][]grpc.DialOption{}
}

func RPCFactoryProvider(
	cfg *config.Config,
	svcName primitives.ServiceName,
	logger log.Logger,
	metricsHandler metrics.Handler,
	tlsConfigProvider encryption.TLSConfigProvider,
	resolver *membership.GRPCResolver,
	tracingStatsHandler telemetry.ClientStatsHandler,
	perServiceDialOptions map[primitives.ServiceName][]grpc.DialOption,
	monitor membership.Monitor,
	dc *dynamicconfig.Collection,
) (common.RPCFactory, error) {
	frontendURL, frontendHTTPURL, frontendHTTPPort, frontendTLSConfig, err := getFrontendConnectionDetails(cfg, tlsConfigProvider, resolver)
	if err != nil {
		return nil, err
	}

	var options []grpc.DialOption
	if tracingStatsHandler != nil {
		options = append(options, grpc.WithStatsHandler(tracingStatsHandler))
	}
	enableServerKeepalive := dynamicconfig.EnableInternodeServerKeepAlive.Get(dc)()
	enableClientKeepalive := dynamicconfig.EnableInternodeClientKeepAlive.Get(dc)()
	factory := rpc.NewFactory(
		cfg,
		svcName,
		logger,
		metricsHandler,
		tlsConfigProvider,
		frontendURL,
		frontendHTTPURL,
		frontendHTTPPort,
		frontendTLSConfig,
		options,
		perServiceDialOptions,
		monitor,
	)
	factory.EnableInternodeServerKeepalive = enableServerKeepalive
	factory.EnableInternodeClientKeepalive = enableClientKeepalive
	logger.Debug(fmt.Sprintf("RPC factory created. enableServerKeepalive: %v, enableClientKeepalive: %v", enableServerKeepalive, enableClientKeepalive))
	return factory, nil
}

func FrontendHTTPClientCacheProvider(
	metadata cluster.Metadata,
	tlsConfigProvider encryption.TLSConfigProvider,
) *cluster.FrontendHTTPClientCache {
	return cluster.NewFrontendHTTPClientCache(metadata, tlsConfigProvider)
}

func getFrontendConnectionDetails(
	cfg *config.Config,
	tlsConfigProvider encryption.TLSConfigProvider,
	resolver *membership.GRPCResolver,
) (string, string, int, *tls.Config, error) {
	// To simplify the static config, we switch default values based on whether the config
	// defines an "internal-frontend" service. The default for TLS config can be overridden
	// with publicClient.forceTLSConfig.
	_, hasIFE := cfg.Services[string(primitives.InternalFrontendService)]

	forceTLS := cfg.PublicClient.ForceTLSConfig
	if forceTLS == config.ForceTLSConfigAuto {
		if hasIFE {
			forceTLS = config.ForceTLSConfigInternode
		} else {
			forceTLS = config.ForceTLSConfigFrontend
		}
	}

	var frontendTLSConfig *tls.Config
	var err error
	switch forceTLS {
	case config.ForceTLSConfigInternode:
		frontendTLSConfig, err = tlsConfigProvider.GetInternodeClientConfig()
	case config.ForceTLSConfigFrontend:
		frontendTLSConfig, err = tlsConfigProvider.GetFrontendClientConfig()
	default:
		err = fmt.Errorf("invalid forceTLSConfig")
	}
	if err != nil {
		return "", "", 0, nil, fmt.Errorf("unable to load TLS configuration: %w", err)
	}

	frontendURL := cfg.PublicClient.HostPort
	if frontendURL == "" {
		if hasIFE {
			frontendURL = resolver.MakeURL(primitives.InternalFrontendService)
		} else {
			frontendURL = resolver.MakeURL(primitives.FrontendService)
		}
	}
	frontendHTTPURL := cfg.PublicClient.HTTPHostPort
	if frontendHTTPURL == "" {
		if hasIFE {
			frontendHTTPURL = resolver.MakeURL(primitives.InternalFrontendService)
		} else {
			frontendHTTPURL = resolver.MakeURL(primitives.FrontendService)
		}
	}

	var frontendHTTPPort int
	if hasIFE {
		frontendHTTPPort = cfg.Services[string(primitives.InternalFrontendService)].RPC.HTTPPort
	} else {
		frontendHTTPPort = cfg.Services[string(primitives.FrontendService)].RPC.HTTPPort
	}

	return frontendURL, frontendHTTPURL, frontendHTTPPort, frontendTLSConfig, nil
}
