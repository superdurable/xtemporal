package archival

import (
	"github.com/superdurable/xtemporal/common/quotas"
	"github.com/superdurable/xtemporal/service/history/configs"
	"go.uber.org/fx"
)

var Module = fx.Options(
	fx.Provide(NewArchiver),
	fx.Provide(func(config *configs.Config) quotas.RateLimiter {
		return quotas.NewDefaultOutgoingRateLimiter(quotas.RateFn(config.ArchivalBackendMaxRPS))
	}),
)
