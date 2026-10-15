package temporal

import (
	"errors"
	"fmt"
	"net/http"
	"slices"

	"github.com/superdurable/xtemporal/client"
	"github.com/superdurable/xtemporal/common/archiver/provider"
	"github.com/superdurable/xtemporal/common/authorization"
	"github.com/superdurable/xtemporal/common/config"
	"github.com/superdurable/xtemporal/common/dynamicconfig"
	"github.com/superdurable/xtemporal/common/log"
	"github.com/superdurable/xtemporal/common/membership/static"
	"github.com/superdurable/xtemporal/common/metrics"
	persistenceClient "github.com/superdurable/xtemporal/common/persistence/client"
	"github.com/superdurable/xtemporal/common/persistence/visibility"
	"github.com/superdurable/xtemporal/common/primitives"
	"github.com/superdurable/xtemporal/common/resolver"
	"github.com/superdurable/xtemporal/common/rpc/auth"
	"github.com/superdurable/xtemporal/common/rpc/encryption"
	"github.com/superdurable/xtemporal/common/searchattribute"
	"github.com/superdurable/xtemporal/common/testing/testhooks"
	otellog "go.opentelemetry.io/otel/log"
	"google.golang.org/grpc"
)

type (
	synchronizationModeParams struct {
		blockingStart bool
		interruptCh   <-chan any
	}

	serverOptions struct {
		serviceNames map[primitives.ServiceName]struct{}

		config         *config.Config
		configDir      string
		env            string
		zone           string
		configFilePath string
		hostsByService map[primitives.ServiceName]static.Hosts

		startupSynchronizationMode synchronizationModeParams

		logger                          log.Logger
		namespaceLogger                 log.Logger
		authorizer                      authorization.Authorizer
		tlsConfigProvider               encryption.TLSConfigProvider
		claimMapper                     authorization.ClaimMapper
		audienceGetter                  authorization.JWTAudienceMapper
		persistenceServiceResolver      resolver.ServiceResolver
		elasticsearchHttpClient         *http.Client //nolint:staticcheck // should be elasticsearchHTTPClient
		dynamicConfigClient             dynamicconfig.Client
		customDataStoreFactory          persistenceClient.AbstractDataStoreFactory
		customVisibilityStoreFactory    visibility.VisibilityStoreFactory
		customHistoryArchiverFactory    provider.CustomHistoryArchiverFactory
		customVisibilityArchiverFactory provider.CustomVisibilityArchiverFactory
		clientFactoryProvider           client.FactoryProvider
		persistenceFactoryProvider      persistenceClient.FactoryProviderFn
		searchAttributesMapper          searchattribute.Mapper
		customFrontendInterceptors      []grpc.UnaryServerInterceptor
		additionalStreamInterceptors    []grpc.StreamServerInterceptor
		metricHandler                   metrics.Handler
		eventLoggerProvider             otellog.LoggerProvider
		tokenProvider                   auth.TokenProvider
		testHooks                       *testhooks.TestHooks
	}
)

func newServerOptions(opts []ServerOption) *serverOptions {
	so := &serverOptions{
		// Set defaults here.
		persistenceServiceResolver: resolver.NewNoopResolver(),
	}
	for _, opt := range opts {
		opt.apply(so)
	}

	return so
}

func (so *serverOptions) loadAndValidate() error {
	for serviceName := range so.serviceNames {
		if !slices.Contains(Services, string(serviceName)) {
			return fmt.Errorf("invalid service %q in service list %v", serviceName, so.serviceNames)
		}
	}

	if so.config == nil {
		err := so.loadConfig()
		if err != nil {
			return fmt.Errorf("unable to load config: %w", err)
		}
	}

	err := so.validateConfig()
	if err != nil {
		return fmt.Errorf("config validation error: %w", err)
	}

	return nil
}

func (so *serverOptions) loadConfig() error {
	if so.configFilePath != "" {
		if so.env != "" || so.configDir != "" || so.zone != "" {
			return errors.New("env, config, zone can not be set if configFilePath is set")
		}
		cfg, err := config.Load(
			config.WithConfigFile(so.configFilePath),
		)
		if err != nil {
			return fmt.Errorf("could not load config file: %w", err)
		}
		so.config = cfg
		return nil
	}
	cfg, err := config.Load(
		config.WithEnv(so.env),
		config.WithConfigDir(so.configDir),
		config.WithZone(so.zone),
	)
	if err != nil {
		return fmt.Errorf("could not load config file: %w", err)
	}
	so.config = cfg
	return nil
}

func (so *serverOptions) validateConfig() error {
	if err := so.config.Validate(); err != nil {
		return err
	}

	for name := range so.serviceNames {
		if _, ok := so.config.Services[string(name)]; !ok {
			return fmt.Errorf("%q service is missing in config", name)
		}
	}
	return nil
}
