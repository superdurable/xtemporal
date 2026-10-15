package tests

import (
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/superdurable/xtemporal/common/config"
	"github.com/superdurable/xtemporal/common/log"
	"github.com/superdurable/xtemporal/common/metrics"
	p "github.com/superdurable/xtemporal/common/persistence"
	"github.com/superdurable/xtemporal/common/persistence/sql"
	"github.com/superdurable/xtemporal/common/persistence/sql/sqlplugin"
	"github.com/superdurable/xtemporal/common/resolver"
)

func limitlessAuroraMode() bool { return os.Getenv("LIMITLESS_TEST") == "aurora" }

func limitlessVisibilityConfig(t *testing.T) *config.SQL {
	t.Helper()
	for _, name := range []string{"LIMITLESS_VISIBILITY_HOST", "LIMITLESS_VISIBILITY_PORT", "LIMITLESS_VISIBILITY_USER", "LIMITLESS_VISIBILITY_PASSWORD"} {
		require.NotEmpty(t, os.Getenv(name), "%s is required when LIMITLESS_TEST=aurora", name)
	}
	port, err := strconv.Atoi(os.Getenv("LIMITLESS_VISIBILITY_PORT"))
	require.NoError(t, err, "LIMITLESS_VISIBILITY_PORT must be a valid port")
	require.Positive(t, port)
	require.LessOrEqual(t, port, 65535)
	sslmode := os.Getenv("LIMITLESS_VISIBILITY_SSLMODE")
	if sslmode == "" {
		sslmode = "disable"
	}
	return &config.SQL{
		User: os.Getenv("LIMITLESS_VISIBILITY_USER"), Password: os.Getenv("LIMITLESS_VISIBILITY_PASSWORD"),
		ConnectAddr:     net.JoinHostPort(os.Getenv("LIMITLESS_VISIBILITY_HOST"), strconv.Itoa(port)),
		ConnectProtocol: "tcp", PluginName: "postgres12_pgx",
		DatabaseName:      "test_visibility_" + strings.ReplaceAll(uuid.NewString(), "-", ""),
		ConnectAttributes: map[string]string{"sslmode": sslmode},
	}
}

func limitlessAuroraConfig() *config.SQL {
	sslmode := os.Getenv("LIMITLESS_SSLMODE")
	if sslmode == "" {
		sslmode = "require"
	}
	return &config.SQL{
		User: os.Getenv("LIMITLESS_USER"), Password: os.Getenv("LIMITLESS_PASSWORD"),
		ConnectAddr:     net.JoinHostPort(os.Getenv("LIMITLESS_HOST"), os.Getenv("LIMITLESS_PORT")),
		ConnectProtocol: "tcp", PluginName: "postgres12_limitless",
		DatabaseName:      "test_limitless_" + strings.ReplaceAll(uuid.NewString(), "-", ""),
		ConnectAttributes: map[string]string{"sslmode": sslmode},
	}
}

func loadLimitlessSchema(t *testing.T, cfg *config.SQL) {
	t.Helper()
	db, err := sql.NewSQLAdminDB(sqlplugin.DbKindUnknown, cfg, resolver.NewNoopResolver(), log.NewTestLogger(), metrics.NoopMetricsHandler)
	require.NoError(t, err)
	defer func() { require.NoError(t, db.Close()) }()
	files := []string{"../../../schema/limitless/temporal/schema.sql"}
	if !limitlessAuroraMode() {
		files = append(files, testPostgreSQLVisibilitySchema)
	}
	for _, file := range files {
		path, err := filepath.Abs(file)
		require.NoError(t, err)
		statements, err := p.LoadAndSplitQuery([]string{path})
		require.NoError(t, err)
		if rewriter, ok := db.(sqlplugin.SchemaStatementRewriter); ok {
			statements = rewriter.RewriteSchemaStatements(statements)
		}
		for _, stmt := range statements {
			require.NoError(t, db.Exec(stmt))
		}
	}
}

func loadLimitlessVisibilitySchema(t *testing.T, cfg *config.SQL) {
	t.Helper()
	db, err := sql.NewSQLAdminDB(sqlplugin.DbKindUnknown, cfg, resolver.NewNoopResolver(), log.NewTestLogger(), metrics.NoopMetricsHandler)
	require.NoError(t, err)
	defer func() { require.NoError(t, db.Close()) }()
	path, err := filepath.Abs(testPostgreSQLVisibilitySchema)
	require.NoError(t, err)
	statements, err := p.LoadAndSplitQuery([]string{path})
	require.NoError(t, err)
	for _, stmt := range statements {
		require.NoError(t, db.Exec(stmt))
	}
}
