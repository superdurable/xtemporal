package paradedb

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

const PluginName = "paradedb"

// The server reads ParadeDB visibility through its custom datastore store; this SQL plugin only lets
// the SQL tool manage the schema, so it refuses the main and visibility databases a server opens.
var errSchemaToolingOnly = errors.New("the ParadeDB visibility store is configured through customDatastore (paradedb-dex); the paradedb SQL plugin is for schema tooling only")

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
	if dbKind != sqlplugin.DbKindUnknown {
		return nil, errSchemaToolingOnly
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
	return &db{DB: pgDB, AdminDB: adminDB}, nil
}
