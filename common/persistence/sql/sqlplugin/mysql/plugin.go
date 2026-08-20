package mysql

import (
	"github.com/jmoiron/sqlx"
	"github.com/superdurable/temporal-server/common/clock"
	"github.com/superdurable/temporal-server/common/config"
	"github.com/superdurable/temporal-server/common/log"
	"github.com/superdurable/temporal-server/common/metrics"
	"github.com/superdurable/temporal-server/common/persistence/sql"
	"github.com/superdurable/temporal-server/common/persistence/sql/sqlplugin"
	"github.com/superdurable/temporal-server/common/persistence/sql/sqlplugin/mysql/session"
	"github.com/superdurable/temporal-server/common/resolver"
)

const (
	// PluginName is the name of the plugin
	PluginName = "mysql8"
)

type plugin struct {
	queryConverter sqlplugin.VisibilityQueryConverter
}

var _ sqlplugin.Plugin = (*plugin)(nil)

func init() {
	sql.RegisterPlugin(PluginName, &plugin{
		queryConverter: &queryConverter{},
	})
}

func (p *plugin) GetVisibilityQueryConverter() sqlplugin.VisibilityQueryConverter {
	return p.queryConverter
}

// CreateDB initialize the db object
func (p *plugin) CreateDB(
	dbKind sqlplugin.DbKind,
	cfg *config.SQL,
	r resolver.ServiceResolver,
	logger log.Logger,
	metricsHandler metrics.Handler,
) (sqlplugin.GenericDB, error) {
	connect := func() (*sqlx.DB, error) {
		if cfg.Connect != nil {
			return cfg.Connect(cfg)
		}
		return p.createDBConnection(dbKind, cfg, r)
	}
	handle := sqlplugin.NewDatabaseHandle(dbKind, connect, isConnNeedsRefreshError, logger, metricsHandler, clock.NewRealTimeSource())
	db := newDB(dbKind, cfg.DatabaseName, handle, nil, logger)
	return db, nil
}

// CreateDBConnection creates a returns a reference to a logical connection to the
// underlying SQL database. The returned object is to tied to a single
// SQL database and the object can be used to perform CRUD operations on
// the tables in the database
func (p *plugin) createDBConnection(
	dbKind sqlplugin.DbKind,
	cfg *config.SQL,
	resolver resolver.ServiceResolver,
) (*sqlx.DB, error) {
	mysqlSession, err := session.NewSession(dbKind, cfg, resolver)
	if err != nil {
		return nil, err
	}
	return mysqlSession.DB, nil
}
