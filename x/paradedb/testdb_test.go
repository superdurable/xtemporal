package paradedb

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
	"github.com/superdurable/xtemporal/common/testing/await"
)

// paradeDBTestDatabase creates a fresh database with the v2.0 visibility schema on the ParadeDB
// container of develop/docker-compose/paradedb and drops it when the test ends. Set
// PARADEDB_TEST=1 to run; POSTGRES_PORT overrides the compose port.
func paradeDBTestDatabase(t *testing.T) (port, dbName string) {
	t.Helper()
	if os.Getenv("PARADEDB_TEST") != "1" {
		t.Skip("set PARADEDB_TEST=1 to run ParadeDB integration tests")
	}
	port = os.Getenv("POSTGRES_PORT")
	if port == "" {
		port = "55432"
	}
	ctx := context.Background()
	// The compose healthcheck can pass while the entrypoint still restarts the server after init.
	var admin *pgx.Conn
	await.Require(ctx, t, func(a *await.T) {
		conn, connectErr := pgx.Connect(a.Context(), paradeDBTestDSN(port, "postgres"))
		require.NoError(a, connectErr)
		admin = conn
	}, 30*time.Second, 200*time.Millisecond)
	defer func() { require.NoError(t, admin.Close(ctx)) }()
	dbName = fmt.Sprintf("pdb_test_%d", time.Now().UnixNano())
	_, err := admin.Exec(ctx, "CREATE DATABASE "+dbName)
	require.NoError(t, err)
	t.Cleanup(func() {
		cleanupCtx := context.Background()
		conn, connErr := pgx.Connect(cleanupCtx, paradeDBTestDSN(port, "postgres"))
		require.NoError(t, connErr)
		defer func() { require.NoError(t, conn.Close(cleanupCtx)) }()
		_, dropErr := conn.Exec(cleanupCtx, "DROP DATABASE "+dbName+" WITH (FORCE)")
		require.NoError(t, dropErr)
	})

	conn, err := pgx.Connect(ctx, paradeDBTestDSN(port, dbName))
	require.NoError(t, err)
	defer func() { require.NoError(t, conn.Close(ctx)) }()
	schema, err := os.ReadFile(filepath.Join("..", "..", "schema", "paradedb", "visibility", "schema.sql"))
	require.NoError(t, err)
	_, err = conn.Exec(ctx, string(schema))
	require.NoError(t, err)
	// The SQL tool creates this table during setup-schema; the store checks it at startup.
	_, err = conn.Exec(ctx, `CREATE TABLE schema_version (version_partition INT NOT NULL, db_name VARCHAR(255) NOT NULL,
creation_time TIMESTAMP, curr_version VARCHAR(64), min_compatible_version VARCHAR(64), PRIMARY KEY (version_partition, db_name))`)
	require.NoError(t, err)
	_, err = conn.Exec(ctx, `INSERT INTO schema_version VALUES (0, $1, now(), '2.0', '2.0')`, dbName)
	require.NoError(t, err)
	return port, dbName
}

func paradeDBTestDSN(port, dbName string) string {
	return "postgres://temporal:temporal@127.0.0.1:" + port + "/" + dbName + "?sslmode=disable"
}
