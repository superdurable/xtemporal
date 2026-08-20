package temporal

import (
	"net/http"

	"github.com/superdurable/temporal-server/client"
	"github.com/superdurable/temporal-server/common/archiver/provider"
	"github.com/superdurable/temporal-server/common/authorization"
	"github.com/superdurable/temporal-server/common/config"
	"github.com/superdurable/temporal-server/common/dynamicconfig"
	"github.com/superdurable/temporal-server/common/log"
	"github.com/superdurable/temporal-server/common/membership/static"
	"github.com/superdurable/temporal-server/common/metrics"
	persistenceclient "github.com/superdurable/temporal-server/common/persistence/client"
	"github.com/superdurable/temporal-server/common/persistence/visibility"
	"github.com/superdurable/temporal-server/common/primitives"
	"github.com/superdurable/temporal-server/common/resolver"
	"github.com/superdurable/temporal-server/common/rpc/encryption"
	"github.com/superdurable/temporal-server/common/searchattribute"
	"google.golang.org/grpc"
)

type (
	ServerOption interface {
		apply(*serverOptions)
	}

	applyFunc func(*serverOptions)
)

func (f applyFunc) apply(s *serverOptions) { f(s) }

// WithConfig sets a custom configuration
func WithConfig(cfg *config.Config) ServerOption {
	return applyFunc(func(s *serverOptions) {
		s.config = cfg
	})
}

// WithConfigLoader sets a custom configuration load
func WithConfigLoader(configDir string, env string, zone string) ServerOption {
	return applyFunc(func(s *serverOptions) {
		s.configDir, s.env, s.zone = configDir, env, zone
	})
}

// WithServerConfigFilePath sets a custom configuration load
func WithServerConfigFilePath(filePath string) ServerOption {
	return applyFunc(func(s *serverOptions) {
		s.configFilePath = filePath
	})
}

// ForServices indicates which supplied services (e.g. frontend, history, matching, worker) within the server to start
func ForServices(names []string) ServerOption {
	return applyFunc(func(s *serverOptions) {
		s.serviceNames = make(map[primitives.ServiceName]struct{})
		for _, name := range names {
			s.serviceNames[primitives.ServiceName(name)] = struct{}{}
		}
	})
}

// WithStaticHosts disables dynamic service membership and resolves service hosts statically.
// At least one host must be provided for all required services (frontend, history, matching, worker).
// And a self-address must be provided for all services passed to ForServices.
func WithStaticHosts(hostsByService map[primitives.ServiceName]static.Hosts) ServerOption {
	return applyFunc(func(s *serverOptions) {
		s.hostsByService = hostsByService
	})
}

// InterruptOn interrupts server on the signal from server. If channel is nil Start() will block forever.
func InterruptOn(interruptCh <-chan any) ServerOption {
	return applyFunc(func(s *serverOptions) {
		s.startupSynchronizationMode.blockingStart = true
		s.startupSynchronizationMode.interruptCh = interruptCh
	})
}

// WithLogger sets a custom logger
func WithLogger(logger log.Logger) ServerOption {
	return applyFunc(func(s *serverOptions) {
		s.logger = logger
	})
}

// WithNamespaceLogger sets an optional logger for all frontend operations
func WithNamespaceLogger(namespaceLogger log.Logger) ServerOption {
	return applyFunc(func(s *serverOptions) {
		s.namespaceLogger = namespaceLogger
	})
}

// WithAuthorizer sets a low level authorizer to allow/deny all API calls
func WithAuthorizer(authorizer authorization.Authorizer) ServerOption {
	return applyFunc(func(s *serverOptions) {
		s.authorizer = authorizer
	})
}

// WithTLSConfigFactory overrides default provider of TLS configuration
func WithTLSConfigFactory(tlsConfigProvider encryption.TLSConfigProvider) ServerOption {
	return applyFunc(func(s *serverOptions) {
		s.tlsConfigProvider = tlsConfigProvider
	})
}

// WithClaimMapper configures a role mapper for authorization
func WithClaimMapper(claimMapper func(cfg *config.Config) authorization.ClaimMapper) ServerOption {
	return applyFunc(func(s *serverOptions) {
		s.claimMapper = claimMapper(s.config)
	})
}

// WithAudienceGetter configures JWT audience getter for authorization
func WithAudienceGetter(audienceGetter func(cfg *config.Config) authorization.JWTAudienceMapper) ServerOption {
	return applyFunc(func(s *serverOptions) {
		s.audienceGetter = audienceGetter(s.config)
	})
}

// WithPersistenceServiceResolver sets a custom persistence service resolver which will convert service name or address value from config to another address
func WithPersistenceServiceResolver(r resolver.ServiceResolver) ServerOption {
	return applyFunc(func(s *serverOptions) {
		s.persistenceServiceResolver = r
	})
}

// WithElasticsearchHttpClient sets a custom HTTP client which is used to make requests to Elasticsearch
func WithElasticsearchHttpClient(c *http.Client) ServerOption {
	return applyFunc(func(s *serverOptions) {
		s.elasticsearchHttpClient = c
	})
}

// WithDynamicConfigClient sets custom client for reading dynamic configuration.
func WithDynamicConfigClient(c dynamicconfig.Client) ServerOption {
	return applyFunc(func(s *serverOptions) {
		s.dynamicConfigClient = c
	})
}

// WithCustomDataStoreFactory sets a custom AbstractDataStoreFactory
// NOTE: this option is experimental and may be changed or removed in future release.
func WithCustomDataStoreFactory(customFactory persistenceclient.AbstractDataStoreFactory) ServerOption {
	return applyFunc(func(s *serverOptions) {
		s.customDataStoreFactory = customFactory
	})
}

func WithCustomVisibilityStoreFactory(customFactory visibility.VisibilityStoreFactory) ServerOption {
	return applyFunc(func(s *serverOptions) {
		s.customVisibilityStoreFactory = customFactory
	})
}

// WithCustomHistoryArchiverFactory sets a custom history archiver factory.
// NOTE: this option is experimental and may be changed in future release.
func WithCustomHistoryArchiverFactory(factory provider.CustomHistoryArchiverFactory) ServerOption {
	return applyFunc(func(s *serverOptions) {
		s.customHistoryArchiverFactory = factory
	})
}

// WithCustomVisibilityArchiverFactory sets a custom visibility archiver factory.
// NOTE: this option is experimental and may be changed in future release.
func WithCustomVisibilityArchiverFactory(factory provider.CustomVisibilityArchiverFactory) ServerOption {
	return applyFunc(func(s *serverOptions) {
		s.customVisibilityArchiverFactory = factory
	})
}

// WithClientFactoryProvider sets a custom ClientFactoryProvider
// NOTE: this option is experimental and may be changed or removed in future release.
func WithClientFactoryProvider(clientFactoryProvider client.FactoryProvider) ServerOption {
	return applyFunc(func(s *serverOptions) {
		s.clientFactoryProvider = clientFactoryProvider
	})
}

// WithSearchAttributesMapper sets a custom search attributes mapper which converts search attributes aliases to field names and vice versa.
func WithSearchAttributesMapper(m searchattribute.Mapper) ServerOption {
	return applyFunc(func(s *serverOptions) {
		s.searchAttributesMapper = m
	})
}

// WithChainedFrontendGrpcInterceptors sets a chain of ordered custom grpc interceptors that will be invoked for all
// Frontend gRPC API calls. The list of custom interceptors will be appended to the end of the internal
// ServerInterceptors. The custom interceptors will be invoked in the order as they appear in the supplied list, after
// the internal ServerInterceptors.
func WithChainedFrontendGrpcInterceptors(
	interceptors ...grpc.UnaryServerInterceptor,
) ServerOption {
	return applyFunc(func(s *serverOptions) {
		s.customFrontendInterceptors = interceptors
	})
}

// WithCustomerMetricsProvider sets a custom implementation of the metrics.MetricsHandler interface
// metrics.MetricsHandler is the base interface for publishing metric events
func WithCustomMetricsHandler(provider metrics.Handler) ServerOption {
	return applyFunc(func(s *serverOptions) {
		s.metricHandler = provider
	})
}
