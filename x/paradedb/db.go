package paradedb

import (
	"errors"

	"github.com/superdurable/xtemporal/common/persistence/sql/sqlplugin"
)

type db struct {
	sqlplugin.DB
	sqlplugin.AdminDB
}

var _ sqlplugin.DB = (*db)(nil)
var _ sqlplugin.AdminDB = (*db)(nil)

//nolint:staticcheck // sqlplugin.GenericDB requires this spelling.
func (d *db) DbName() string { return d.DB.DbName() }
func (d *db) Close() error   { return d.DB.Close() }
func (d *db) PluginName() string {
	return PluginName
}

func (d *db) ForceDropDatabase(name string) error {
	dropper, ok := d.AdminDB.(interface{ ForceDropDatabase(string) error })
	if !ok {
		return errors.New("postgres12_pgx AdminDB does not implement ForceDropDatabase")
	}
	return dropper.ForceDropDatabase(name)
}
