package testcore

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/superdurable/xtemporal/common/config"
	"github.com/superdurable/xtemporal/common/persistence/visibility"
	"github.com/superdurable/xtemporal/temporal/environment"
	"github.com/superdurable/xtemporal/tests/testutils"
	"github.com/superdurable/xtemporal/x/paradedb"
)

// xtemporal: Each onebox gets an isolated Dex schema while its main store stays on PostgreSQL.
func setupParadeDBVisibility(pConfig *config.Persistence) (func() error, error) {
	host := os.Getenv("PARADEDB_HOST")
	if host == "" {
		host = environment.GetPostgreSQLAddress()
	}
	port := 55432
	if value := os.Getenv("PARADEDB_PORT"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 1 || parsed > 65535 {
			return nil, fmt.Errorf("invalid PARADEDB_PORT %q", value)
		}
		port = parsed
	}
	dbName := "pdb_visibility_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	adminDSN := fmt.Sprintf("postgres://temporal:temporal@%s:%d/postgres?sslmode=disable", host, port)
	dbDSN := fmt.Sprintf("postgres://temporal:temporal@%s:%d/%s?sslmode=disable", host, port, dbName)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	admin, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		return nil, fmt.Errorf("connect ParadeDB admin database: %w", err)
	}
	if _, err = admin.Exec(ctx, "CREATE DATABASE "+dbName); err != nil {
		return nil, errors.Join(fmt.Errorf("create ParadeDB visibility database: %w", err), admin.Close(context.Background()))
	}
	cleanup := func() error {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		conn, connectErr := pgx.Connect(cleanupCtx, adminDSN)
		if connectErr != nil {
			return fmt.Errorf("connect to drop ParadeDB visibility database: %w", connectErr)
		}
		_, dropErr := conn.Exec(cleanupCtx, "DROP DATABASE "+dbName+" WITH (FORCE)")
		closeErr := conn.Close(context.Background())
		return errors.Join(dropErr, closeErr)
	}
	fail := func(cause error) (func() error, error) {
		return nil, errors.Join(cause, cleanup())
	}
	if err = admin.Close(context.Background()); err != nil {
		return fail(fmt.Errorf("close ParadeDB admin connection: %w", err))
	}
	conn, err := pgx.Connect(ctx, dbDSN)
	if err != nil {
		return fail(fmt.Errorf("connect ParadeDB visibility database: %w", err))
	}
	schema, err := os.ReadFile(filepath.Join(testutils.GetRepoRootDirectory(), "schema/paradedb/visibility/schema.sql"))
	if err == nil {
		_, err = conn.Exec(ctx, string(schema))
	}
	if err == nil {
		_, err = conn.Exec(ctx, `CREATE TABLE schema_version (version_partition INT NOT NULL, db_name VARCHAR(255) NOT NULL,
creation_time TIMESTAMP, curr_version VARCHAR(64), min_compatible_version VARCHAR(64), PRIMARY KEY (version_partition, db_name))`)
	}
	if err == nil {
		_, err = conn.Exec(ctx, `INSERT INTO schema_version VALUES (0, $1, now(), '2.0', '2.0')`, dbName)
	}
	err = errors.Join(err, conn.Close(context.Background()))
	if err != nil {
		return fail(fmt.Errorf("initialize ParadeDB visibility schema: %w", err))
	}
	pConfig.VisibilityStore = "test-paradedb-visibility"
	pConfig.DataStores[pConfig.VisibilityStore] = config.DataStore{
		CustomDataStoreConfig: &config.CustomDatastoreConfig{
			Name:      visibility.ParadeDBDexStoreName,
			IndexName: paradedb.StoreName + "_visibility",
			Options: map[string]any{"sql": map[string]any{
				"pluginName": "postgres12_pgx", "connectProtocol": "tcp",
				"connectAddr": fmt.Sprintf("%s:%d", host, port), "databaseName": dbName,
				"user": "temporal", "password": "temporal",
			}},
		},
	}
	return cleanup, nil
}
