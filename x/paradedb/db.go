package paradedb

import (
	"errors"
	"fmt"

	"github.com/superdurable/xtemporal/common/persistence/schema"
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

func (d *db) ExpectedVersion() string { return "2.0" }

func (d *db) VerifyVersion() error {
	version, err := d.ReadSchemaVersion(d.DbName())
	if err != nil {
		return fmt.Errorf("read ParadeDB visibility schema version: %w", err)
	}
	if err := schema.VerifyCompatibleVersion(d, d.DbName(), d.ExpectedVersion()); err != nil {
		return err
	}
	if version != d.ExpectedVersion() {
		return fmt.Errorf("ParadeDB visibility schema requires version %s, found %s", d.ExpectedVersion(), version)
	}
	return nil
}

func (d *db) ForceDropDatabase(name string) error {
	dropper, ok := d.AdminDB.(interface{ ForceDropDatabase(string) error })
	if !ok {
		return errors.New("postgres12_pgx AdminDB does not implement ForceDropDatabase")
	}
	return dropper.ForceDropDatabase(name)
}
