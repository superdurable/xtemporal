package auth

import (
	"context"
	"strings"

	"github.com/superdurable/xtemporal/common/authorization"
	"github.com/superdurable/xtemporal/common/config"
	"github.com/superdurable/xtemporal/common/log"
	"github.com/superdurable/xtemporal/common/metrics"
)

func GetClaimMapperFromConfig(cfg *config.Config, logger log.Logger) (authorization.ClaimMapper, metrics.Handler, context.CancelFunc, error) {
	if !strings.EqualFold(cfg.Global.Authorization.ClaimMapper, "jtiDenylist") {
		mapper, err := authorization.GetClaimMapperFromConfig(&cfg.Global.Authorization, logger)
		return mapper, nil, func() {}, err
	}
	source, interval, failClosed, err := ConfigFromEnv()
	if err != nil {
		return nil, nil, nil, err
	}
	handler, err := metrics.MetricsHandlerFromConfig(logger, cfg.Global.Metrics)
	if err != nil {
		return nil, nil, nil, err
	}
	base := authorization.NewDefaultJWTClaimMapper(
		authorization.NewDefaultTokenKeyProvider(&cfg.Global.Authorization, logger),
		&cfg.Global.Authorization,
		logger,
	)
	denylist := NewDenylist(source, logger, handler, failClosed)
	denylist.Reload(context.Background())
	ctx, cancel := context.WithCancel(context.Background())
	go denylist.Poll(ctx, interval)
	return NewMapper(base, denylist), handler, cancel, nil
}
