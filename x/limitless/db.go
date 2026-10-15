package limitless

import (
	"context"
	"database/sql"
	"errors"

	"github.com/superdurable/xtemporal/common/log"
	"github.com/superdurable/xtemporal/common/persistence/schema"
	"github.com/superdurable/xtemporal/common/persistence/sql/sqlplugin"
	limitlessschema "github.com/superdurable/xtemporal/schema/limitless"
)

type db struct {
	sqlplugin.DB
	sqlplugin.AdminDB
	logger log.Logger
}

var _ sqlplugin.DB = (*db)(nil)
var _ sqlplugin.AdminDB = (*db)(nil)
var _ sqlplugin.SchemaStatementRewriter = (*db)(nil)

//nolint:staticcheck // sqlplugin.GenericDB requires this spelling.
func (d *db) DbName() string          { return d.DB.DbName() }
func (d *db) Close() error            { return d.DB.Close() }
func (d *db) PluginName() string      { return PluginName }
func (d *db) ExpectedVersion() string { return limitlessschema.Version }
func (d *db) VerifyVersion() error {
	return schema.VerifyCompatibleVersion(d.AdminDB, d.DbName(), d.ExpectedVersion())
}

func (d *db) BeginTx(ctx context.Context) (sqlplugin.Tx, error) {
	inner, err := d.DB.BeginTx(ctx)
	if err != nil {
		return nil, err
	}
	return &tx{Tx: inner}, nil
}

func (d *db) InsertIntoBufferedEvents(ctx context.Context, rows []sqlplugin.BufferedEventsRow) (sql.Result, error) {
	return insertIntoBufferedEvents(ctx, d.DB, rows)
}

func (d *db) ForceDropDatabase(name string) error {
	dropper, ok := d.AdminDB.(interface{ ForceDropDatabase(string) error })
	if !ok {
		return errors.New("postgres12_pgx AdminDB does not implement ForceDropDatabase")
	}
	return dropper.ForceDropDatabase(name)
}
