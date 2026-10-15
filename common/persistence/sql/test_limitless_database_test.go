package sql

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
	"github.com/superdurable/xtemporal/common/persistence/sql/sqlplugin"
)

type limitlessAdminDBStub struct {
	sqlplugin.AdminDB
	calls []string
	errs  []error
}

func (d *limitlessAdminDBStub) Exec(stmt string, _ ...any) error {
	d.calls = append(d.calls, stmt)
	if len(d.errs) == 0 {
		return nil
	}
	err := d.errs[0]
	d.errs = d.errs[1:]
	return err
}

func TestCreateLimitlessTestDatabase(t *testing.T) {
	t.Run("new database", func(t *testing.T) {
		db := &limitlessAdminDBStub{}
		name, err := createLimitlessTestDatabase(db, "test_initial")
		require.NoError(t, err)
		require.Equal(t, "test_initial", name)
		require.Equal(t, []string{`CREATE DATABASE "test_initial"`}, db.calls)
	})

	t.Run("duplicate name", func(t *testing.T) {
		db := &limitlessAdminDBStub{errs: []error{&pgconn.PgError{Code: "42P04"}}}
		name, err := createLimitlessTestDatabase(db, "test_initial")
		require.NoError(t, err)
		require.NotEqual(t, "test_initial", name)
		require.Equal(t, []string{`CREATE DATABASE "test_initial"`, `CREATE DATABASE "` + name + `"`}, db.calls)
	})

	t.Run("other error", func(t *testing.T) {
		db := &limitlessAdminDBStub{errs: []error{errors.New("connection lost")}}
		_, err := createLimitlessTestDatabase(db, "test_initial")
		require.ErrorContains(t, err, "connection lost")
		require.Len(t, db.calls, 1)
	})
}
