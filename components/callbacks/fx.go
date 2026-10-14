package callbacks

import (
	"fmt"
	"net/http"

	"github.com/superdurable/xtemporal/common"
	"github.com/superdurable/xtemporal/common/cluster"
	"github.com/superdurable/xtemporal/common/collection"
	"github.com/superdurable/xtemporal/common/log"
	"github.com/superdurable/xtemporal/common/namespace"
	commonnexus "github.com/superdurable/xtemporal/common/nexus"
	queuescommon "github.com/superdurable/xtemporal/service/history/queues/common"
	"go.uber.org/fx"
)

var Module = fx.Module(
	"component.callbacks",
	fx.Provide(ConfigProvider),
	fx.Provide(HTTPCallerProviderProvider),
	fx.Invoke(RegisterTaskSerializers),
	fx.Invoke(RegisterStateMachine),
	fx.Invoke(RegisterExecutor),
)

func HTTPCallerProviderProvider(
	clusterMetadata cluster.Metadata,
	namespaceRegistry namespace.Registry,
	rpcFactory common.RPCFactory,
	httpClientCache *cluster.FrontendHTTPClientCache,
	logger log.Logger,
	config *Config,
) (HTTPCallerProvider, error) {
	localClient, err := rpcFactory.CreateLocalFrontendHTTPClient()
	if err != nil {
		return nil, fmt.Errorf("cannot create local frontend HTTP client: %w", err)
	}
	defaultClient := &http.Client{}
	callbackTokenGenerator := commonnexus.NewCallbackTokenGenerator()

	m := collection.NewOnceMap(func(queuescommon.NamespaceIDAndDestination) HTTPCaller {
		return func(r *http.Request) (*http.Response, error) {
			return routeRequest(r,
				clusterMetadata,
				namespaceRegistry,
				httpClientCache,
				callbackTokenGenerator,
				defaultClient,
				localClient,
				logger,
				config.InspectSourceHeader(),
			)
		}
	})
	return m.Get, nil
}
