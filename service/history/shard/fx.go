package shard

import (
	"context"
	"sync/atomic"

	"github.com/superdurable/temporal-server/common/log"
	"github.com/superdurable/temporal-server/common/log/tag"
	"github.com/superdurable/temporal-server/common/pingable"
	"github.com/superdurable/temporal-server/common/primitives"
	"github.com/superdurable/temporal-server/service/history/configs"
	"go.uber.org/fx"
)

var Module = fx.Options(
	fx.Provide(
		ControllerProvider,
		func(impl *ControllerImpl) Controller { return impl },
		ContextFactoryProvider,
		fx.Annotate(
			func(p Controller) pingable.Pingable { return p },
			fx.ResultTags(`group:"deadlockDetectorRoots"`),
		),
	),
	ownershipBasedQuotaScalerModule,
)

var ownershipBasedQuotaScalerModule = fx.Options(
	fx.Provide(func(
		impl *ControllerImpl,
		cfg *configs.Config,
	) (*OwnershipBasedQuotaScalerImpl, error) {
		return NewOwnershipBasedQuotaScaler(
			impl,
			int(cfg.NumberOfShards()),
			nil,
		)
	}),
	fx.Provide(func(
		impl *OwnershipBasedQuotaScalerImpl,
	) OwnershipBasedQuotaScaler {
		return impl
	}),
	fx.Provide(func() LazyLoadedOwnershipBasedQuotaScaler {
		return LazyLoadedOwnershipBasedQuotaScaler{
			Value: &atomic.Value{},
		}
	}),
	fx.Invoke(initLazyLoadedOwnershipBasedQuotaScaler),
	fx.Invoke(func(
		lc fx.Lifecycle,
		impl *OwnershipBasedQuotaScalerImpl,
	) {
		lc.Append(fx.Hook{
			OnStop: func(_ context.Context) error {
				impl.Close()
				return nil
			},
		})
	}),
)

func initLazyLoadedOwnershipBasedQuotaScaler(
	serviceName primitives.ServiceName,
	logger log.SnTaggedLogger,
	ownershipBasedQuotaScaler OwnershipBasedQuotaScaler,
	lazyLoadedOwnershipBasedQuotaScaler LazyLoadedOwnershipBasedQuotaScaler,
) {
	lazyLoadedOwnershipBasedQuotaScaler.Store(ownershipBasedQuotaScaler)
	logger.Info("Initialized lazy loaded OwnershipBasedQuotaScaler", tag.Service(serviceName))
}
