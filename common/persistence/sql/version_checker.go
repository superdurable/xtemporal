package sql

import (
	"github.com/superdurable/temporal-server/common/config"
	"github.com/superdurable/temporal-server/common/log"
	"github.com/superdurable/temporal-server/common/metrics"
	"github.com/superdurable/temporal-server/common/persistence/sql/sqlplugin"
	"github.com/superdurable/temporal-server/common/resolver"
)

// VerifyCompatibleVersion ensures that the installed version of temporal and visibility
// is greater than or equal to the expected version.
func VerifyCompatibleVersion(
	cfg config.Persistence,
	r resolver.ServiceResolver,
	logger log.Logger,
) error {

	if err := checkMainDatabase(cfg, r, logger); err != nil {
		return err
	}
	if cfg.VisibilityConfigExist() {
		return checkVisibilityDatabase(cfg, r, logger)
	}
	return nil
}

func checkMainDatabase(
	cfg config.Persistence,
	r resolver.ServiceResolver,
	logger log.Logger,
) error {
	ds, ok := cfg.DataStores[cfg.DefaultStore]
	if ok && ds.SQL != nil {
		return checkCompatibleVersion(ds.SQL, r, sqlplugin.DbKindMain, logger)
	}
	return nil
}

func checkVisibilityDatabase(
	cfg config.Persistence,
	r resolver.ServiceResolver,
	logger log.Logger,
) error {
	ds, ok := cfg.DataStores[cfg.VisibilityStore]
	if ok && ds.SQL != nil {
		return checkCompatibleVersion(ds.SQL, r, sqlplugin.DbKindVisibility, logger)
	}
	return nil
}

func checkCompatibleVersion(
	cfg *config.SQL,
	r resolver.ServiceResolver,
	dbKind sqlplugin.DbKind,
	logger log.Logger,
) error {
	db, err := NewSQLAdminDB(dbKind, cfg, r, logger, metrics.NoopMetricsHandler)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()

	return db.VerifyVersion()
}
