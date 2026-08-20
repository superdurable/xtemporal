package temporal

import (
	"errors"
	"fmt"
	"net/http"
	"slices"

	"github.com/superdurable/temporal-server/client"
	"github.com/superdurable/temporal-server/common/archiver/provider"
	"github.com/superdurable/temporal-server/common/authorization"
	"github.com/superdurable/temporal-server/common/config"
	"github.com/superdurable/temporal-server/common/dynamicconfig"
	"github.com/superdurable/temporal-server/common/log"
	"github.com/superdurable/temporal-server/common/membership/static"
	"github.com/superdurable/temporal-server/common/metrics"
	persistenceClient "github.com/superdurable/temporal-server/common/persistence/client"
	"github.com/superdurable/temporal-server/common/persistence/visibility"
	"github.com/superdurable/temporal-server/common/primitives"
	"github.com/superdurable/temporal-server/common/resolver"
	"github.com/superdurable/temporal-server/common/rpc/encryption"
	"github.com/superdurable/temporal-server/common/searchattribute"
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
		searchAttributesMapper          searchattribute.Mapper
		customFrontendInterceptors      []grpc.UnaryServerInterceptor
		metricHandler                   metrics.Handler
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
