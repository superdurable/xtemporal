package tests

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"github.com/superdurable/xtemporal/common/config"
	"github.com/superdurable/xtemporal/common/log"
	persistencetests "github.com/superdurable/xtemporal/common/persistence/persistence-tests"
	"github.com/superdurable/xtemporal/common/persistence/visibility"
	"github.com/superdurable/xtemporal/common/testing/await"
	"github.com/superdurable/xtemporal/x/paradedb"
)

// xtemporal: The shared visibility suite exercises the custom ParadeDB factory on a fresh v2 database.
func TestParadeDBVisibilityPersistenceSuite(t *testing.T) {
	if os.Getenv("PARADEDB_TEST") != "1" {
		t.Skip("set PARADEDB_TEST=1 to run ParadeDB integration tests")
	}
	port := os.Getenv("POSTGRES_PORT")
	if port == "" {
		port = "55432"
	}
	ctx := t.Context()
	cleanupCtx := context.WithoutCancel(ctx)
	adminDSN := fmt.Sprintf("postgres://temporal:temporal@127.0.0.1:%s/postgres?sslmode=disable", port)
	var admin *pgx.Conn
	await.Require(ctx, t, func(a *await.T) {
		var err error
		admin, err = pgx.Connect(a.Context(), adminDSN)
		require.NoError(a, err)
	}, 30*time.Second, 200*time.Millisecond)
	t.Cleanup(func() { require.NoError(t, admin.Close(cleanupCtx)) })
	dbName := fmt.Sprintf("pdb_suite_%d", time.Now().UnixNano())
	_, err := admin.Exec(ctx, "CREATE DATABASE "+dbName)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, dropErr := admin.Exec(cleanupCtx, "DROP DATABASE "+dbName+" WITH (FORCE)")
		require.NoError(t, dropErr)
	})
	conn, err := pgx.Connect(ctx, fmt.Sprintf("postgres://temporal:temporal@127.0.0.1:%s/%s?sslmode=disable", port, dbName))
	require.NoError(t, err)
	schema, err := os.ReadFile(filepath.Join("..", "..", "..", "schema", "paradedb", "visibility", "schema.sql"))
	require.NoError(t, err)
	_, err = conn.Exec(ctx, string(schema))
	require.NoError(t, err)
	_, err = conn.Exec(ctx, `CREATE TABLE schema_version (version_partition INT NOT NULL, db_name VARCHAR(255) NOT NULL,
creation_time TIMESTAMP, curr_version VARCHAR(64), min_compatible_version VARCHAR(64), PRIMARY KEY (version_partition, db_name))`)
	require.NoError(t, err)
	_, err = conn.Exec(ctx, `INSERT INTO schema_version VALUES (0, $1, now(), '2.0', '2.0')`, dbName)
	require.NoError(t, err)
	require.NoError(t, conn.Close(ctx))

	cluster := &paradeDBVisibilityTestCluster{port: port, dbName: dbName}
	s := &VisibilityPersistenceSuite{
		TestBase:                     persistencetests.NewTestBaseForCluster(cluster, log.NewTestLogger()),
		CustomVisibilityStoreFactory: paradedb.VisibilityStoreFactory{},
	}
	suite.Run(t, s)
}

type paradeDBVisibilityTestCluster struct{ port, dbName string }

func (*paradeDBVisibilityTestCluster) SetupTestDatabase()    {}
func (*paradeDBVisibilityTestCluster) TearDownTestDatabase() {}

func (c *paradeDBVisibilityTestCluster) Config() config.Persistence {
	return config.Persistence{VisibilityStore: "dex-visibility", DataStores: map[string]config.DataStore{
		"dex-visibility": {CustomDataStoreConfig: &config.CustomDatastoreConfig{
			Name: visibility.ParadeDBDexStoreName, IndexName: "dex_visibility",
			Options: map[string]any{"sql": map[string]any{"pluginName": "postgres12_pgx", "connectProtocol": "tcp",
				"connectAddr": "127.0.0.1:" + c.port, "databaseName": c.dbName,
				"user": "temporal", "password": "temporal"}},
		}},
	}}
}
