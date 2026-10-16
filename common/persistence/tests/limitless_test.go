package tests

// Run from the repository root:
//   POSTGRES_SEEDS=127.0.0.1 POSTGRES_PORT=55432 LIMITLESS_TEST=pg go test -tags test_dep -count=1 ./common/persistence/tests/ -run Limitless -timeout 60m
//   LIMITLESS_TEST=aurora LIMITLESS_HOST=router.example.com LIMITLESS_PORT=5432 LIMITLESS_USER=temporal LIMITLESS_PASSWORD=... LIMITLESS_ADMIN_DATABASE=postgres_limitless LIMITLESS_VISIBILITY_HOST=postgres.example.com LIMITLESS_VISIBILITY_PORT=5432 LIMITLESS_VISIBILITY_USER=temporal LIMITLESS_VISIBILITY_PASSWORD=... go test -tags test_dep -count=1 ./common/persistence/tests/ -run Limitless -timeout 60m
// LIMITLESS_SSLMODE defaults to require; LIMITLESS_VISIBILITY_SSLMODE defaults to disable.
// Aurora creates a separate database per test cluster or suite using LIMITLESS_ADMIN_DATABASE.
// SQL-tool on Aurora (requires LIMITLESS_SQL_TOOL_DATABASE for the dedicated database):
//   limitless-verify/limitless-sql-tool-verify.sh

import (
	"os"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"github.com/superdurable/xtemporal/common/log"
	"github.com/superdurable/xtemporal/common/metrics"
	"github.com/superdurable/xtemporal/common/persistence/sql"
	"github.com/superdurable/xtemporal/common/persistence/sql/sqlplugin"
	"github.com/superdurable/xtemporal/common/resolver"
	_ "github.com/superdurable/xtemporal/x/limitless"
)

type LimitlessSuite struct{ PostgreSQLSuite }

func TestLimitless(t *testing.T) {
	mode := os.Getenv("LIMITLESS_TEST")
	if mode != "pg" && mode != "aurora" {
		t.Skip("set LIMITLESS_TEST=pg or LIMITLESS_TEST=aurora")
	}
	if mode == "aurora" {
		for _, name := range []string{"LIMITLESS_HOST", "LIMITLESS_PORT", "LIMITLESS_USER", "LIMITLESS_PASSWORD", "LIMITLESS_ADMIN_DATABASE"} {
			require.NotEmpty(t, os.Getenv(name), "%s is required", name)
		}
		port, err := strconv.Atoi(os.Getenv("LIMITLESS_PORT"))
		require.NoError(t, err)
		require.Positive(t, port)
		_ = limitlessVisibilityConfig(t)
	}
	suite.Run(t, &LimitlessSuite{PostgreSQLSuite: PostgreSQLSuite{pluginName: "postgres12_limitless"}})
}

func TestLimitlessSQLToolVerifyVersion(t *testing.T) {
	if os.Getenv("LIMITLESS_SQL_TOOL_VERIFY") != "1" || !limitlessAuroraMode() {
		t.Skip("run limitless-verify/limitless-sql-tool-verify.sh against Aurora")
	}
	cfg := limitlessAuroraConfig()
	cfg.DatabaseName = os.Getenv("LIMITLESS_SQL_TOOL_DATABASE")
	require.NotEmpty(t, cfg.DatabaseName)
	db, err := sql.NewSQLAdminDB(sqlplugin.DbKindMain, cfg, resolver.NewNoopResolver(), log.NewTestLogger(), metrics.NoopMetricsHandler)
	require.NoError(t, err)
	defer func() { require.NoError(t, db.Close()) }()
	require.NoError(t, db.VerifyVersion())
}

func (s *LimitlessSuite) TestPostgreSQLVisibilityPersistenceSuite() {
	s.T().Skip("Limitless supports only the default store; visibility uses postgres12_pgx in the functional harness")
}

func (s *LimitlessSuite) TestPostgreSQLVisibilitySuite() {
	s.T().Skip("Limitless supports only the default store; visibility uses postgres12_pgx in the functional harness")
}

func (s *LimitlessSuite) TestPostgreSQLTestClusterDropsDatabaseWithOpenConnection() {
	if limitlessAuroraMode() {
		s.T().Skip("Aurora uses plain DROP DATABASE; open connections are not forcibly terminated")
	}
	s.PostgreSQLSuite.TestPostgreSQLTestClusterDropsDatabaseWithOpenConnection()
}
