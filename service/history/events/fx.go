package events

import (
	"github.com/superdurable/xtemporal/common/log"
	"github.com/superdurable/xtemporal/common/metrics"
	"github.com/superdurable/xtemporal/common/persistence"
	"github.com/superdurable/xtemporal/service/history/configs"
	"go.uber.org/fx"
)

var Module = fx.Options(
	fx.Provide(func(executionManager persistence.ExecutionManager, config *configs.Config, handler metrics.Handler, logger log.Logger) Cache {
		return NewHostLevelEventsCache(executionManager, config, handler, logger, false)
	}),
)
