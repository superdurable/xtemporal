package dynamicconfig

import (
	"github.com/superdurable/temporal-server/common/log"
	"github.com/superdurable/temporal-server/common/pingable"
	"go.uber.org/fx"
)

var Module = fx.Options(
	fx.Provide(func(client Client, logger log.Logger, lc fx.Lifecycle) *Collection {
		col := NewCollection(client, logger)
		lc.Append(fx.StartStopHook(col.Start, col.Stop))
		return col
	}),
	fx.Provide(fx.Annotate(
		func(c *Collection) pingable.Pingable { return c },
		fx.ResultTags(`group:"deadlockDetectorRoots"`),
	)),
)
