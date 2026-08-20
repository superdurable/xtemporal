package workercomponent

import (
	"github.com/superdurable/temporal-server/common/dynamicconfig"
	"github.com/superdurable/temporal-server/common/sdk"
	workercommon "github.com/superdurable/temporal-server/service/worker/common"
	"go.uber.org/fx"
)

type (
	componentDeps struct {
		fx.In
		ClientFactory sdk.ClientFactory
	}

	fxResult struct {
		fx.Out
		Component workercommon.PerNSWorkerComponent `group:"perNamespaceWorkerComponent"`
	}
)

var Module = fx.Options(
	fx.Provide(NewResult),
)

func NewResult(
	dc *dynamicconfig.Collection,
	params componentDeps,
) fxResult {
	return fxResult{
		Component: NewWCIPerNSWorkerComponent(dc, params.ClientFactory),
	}
}
