package tests

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

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

func limitlessAuroraConfig() *config.SQL {
	return &config.SQL{
		User: os.Getenv("LIMITLESS_USER"), Password: os.Getenv("LIMITLESS_PASSWORD"),
		ConnectAddr:     net.JoinHostPort(os.Getenv("LIMITLESS_HOST"), os.Getenv("LIMITLESS_PORT")),
		ConnectProtocol: "tcp", PluginName: "postgres12_limitless",
		DatabaseName:      os.Getenv("LIMITLESS_DATABASE"),
		ConnectAttributes: map[string]string{"sslmode": "require"},
	}
}

func resetLimitlessDatabase(t *testing.T, cfg *config.SQL) {
	t.Helper()
	db, err := sql.NewSQLAdminDB(sqlplugin.DbKindUnknown, cfg, resolver.NewNoopResolver(), log.NewTestLogger(), metrics.NoopMetricsHandler)
	require.NoError(t, err)
	defer func() { require.NoError(t, db.Close()) }()
	tables, err := db.ListTables(cfg.DatabaseName)
	require.NoError(t, err)
	for _, table := range tables {
		quoted := `"` + strings.ReplaceAll(table, `"`, `""`) + `"`
		require.NoError(t, db.Exec("DROP TABLE "+quoted+" CASCADE"))
	}
}

func loadLimitlessSchema(t *testing.T, cfg *config.SQL) {
	t.Helper()
	db, err := sql.NewSQLAdminDB(sqlplugin.DbKindUnknown, cfg, resolver.NewNoopResolver(), log.NewTestLogger(), metrics.NoopMetricsHandler)
	require.NoError(t, err)
	defer func() { require.NoError(t, db.Close()) }()
	for _, file := range []string{"../../../schema/limitless/temporal/schema.sql", testPostgreSQLVisibilitySchema} {
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
