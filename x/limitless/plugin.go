package limitless

import (
	"errors"

	"github.com/superdurable/xtemporal/common/config"
	"github.com/superdurable/xtemporal/common/log"
	"github.com/superdurable/xtemporal/common/metrics"
	"github.com/superdurable/xtemporal/common/persistence/sql"
	"github.com/superdurable/xtemporal/common/persistence/sql/sqlplugin"
	"github.com/superdurable/xtemporal/common/persistence/sql/sqlplugin/postgresql"
	"github.com/superdurable/xtemporal/common/resolver"
)

const PluginName = "postgres12_limitless"

type plugin struct{}

var _ sqlplugin.Plugin = (*plugin)(nil)

func init() {
	sql.RegisterPlugin(PluginName, &plugin{})
}

func (*plugin) GetVisibilityQueryConverter() sqlplugin.VisibilityQueryConverter {
	return nil
}

func (*plugin) CreateDB(
	dbKind sqlplugin.DbKind,
	cfg *config.SQL,
	r resolver.ServiceResolver,
	l log.Logger,
	mh metrics.Handler,
) (sqlplugin.GenericDB, error) {
	if dbKind == sqlplugin.DbKindVisibility {
		return nil, errors.New("postgres12_limitless only supports the default store")
	}
	pgConfig := *cfg
	pgConfig.PluginName = postgresql.PluginNamePGX
	pgDB, err := sql.NewSQLDB(dbKind, &pgConfig, r, l, mh)
	if err != nil {
		return nil, err
	}
	adminDB, ok := pgDB.(sqlplugin.AdminDB)
	if !ok {
		return nil, errors.Join(errors.New("postgres12_pgx DB does not implement AdminDB"), pgDB.Close())
	}
	return &db{DB: pgDB, AdminDB: adminDB, logger: l}, nil
}
