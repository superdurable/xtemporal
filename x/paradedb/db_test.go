package paradedb

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/superdurable/xtemporal/common/config"
	"github.com/superdurable/xtemporal/common/persistence/sql/sqlplugin"
)

type versionDB struct {
	sqlplugin.DB
}

//nolint:staticcheck // sqlplugin.GenericDB requires this spelling.
func (*versionDB) DbName() string { return "visibility" }

type versionAdminDB struct {
	sqlplugin.AdminDB
	version string
	err     error
}

func (d *versionAdminDB) ReadSchemaVersion(string) (string, error) { return d.version, d.err }

func TestVerifyVersion(t *testing.T) {
	for _, tc := range []struct {
		version   string
		err       error
		wantError bool
	}{
		{version: "2.0"},
		{version: "1.14", wantError: true},
		{version: "2.1", wantError: true},
		{err: errors.New("unavailable"), wantError: true},
	} {
		d := &db{DB: &versionDB{}, AdminDB: &versionAdminDB{version: tc.version, err: tc.err}}
		require.Equal(t, "2.0", d.ExpectedVersion())
		err := d.VerifyVersion()
		if tc.wantError {
			require.Error(t, err)
		} else {
			require.NoError(t, err)
		}
	}
}

func TestCreateDBRejectsServerDatabases(t *testing.T) {
	for _, kind := range []sqlplugin.DbKind{sqlplugin.DbKindMain, sqlplugin.DbKindVisibility} {
		_, err := (&plugin{}).CreateDB(kind, &config.SQL{PluginName: PluginName}, nil, nil, nil)
		require.ErrorIs(t, err, errSchemaToolingOnly)
	}
}
