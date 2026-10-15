package tests

import (
	"net"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/superdurable/xtemporal/common/config"
	"github.com/superdurable/xtemporal/common/log"
	"github.com/superdurable/xtemporal/common/metrics"
	"github.com/superdurable/xtemporal/common/metrics/metricstest"
	p "github.com/superdurable/xtemporal/common/persistence"
	"github.com/superdurable/xtemporal/common/persistence/serialization"
	"github.com/superdurable/xtemporal/common/persistence/sql"
	"github.com/superdurable/xtemporal/common/persistence/sql/sqlplugin"
	"github.com/superdurable/xtemporal/common/resolver"
	"github.com/superdurable/xtemporal/common/shuffle"
	"github.com/superdurable/xtemporal/temporal/environment"
	"go.uber.org/zap/zaptest"
)

// TODO merge the initialization with existing persistence setup
const (
	testPostgreSQLClusterName = "temporal_postgresql_cluster"

	testPostgreSQLUser               = "temporal"
	testPostgreSQLPassword           = "temporal"
	testPostgreSQLConnectionProtocol = "tcp"
	testPostgreSQLDatabaseNamePrefix = "test_"
	testPostgreSQLDatabaseNameSuffix = "temporal_persistence"

	// TODO hard code this dir for now
	//  need to merge persistence test config / initialization in one place
	testPostgreSQLExecutionSchema  = "../../../schema/postgresql/v12/temporal/schema.sql"
	testPostgreSQLVisibilitySchema = "../../../schema/postgresql/v12/visibility/schema.sql"
)

type (
	PostgreSQLTestData struct {
		Cfg           *config.SQL
		Factory       *sql.Factory
		Logger        log.Logger
		Metrics       *metricstest.Capture
		VisibilityCfg *config.SQL // xtemporal: Aurora visibility uses vanilla PostgreSQL
	}
)

func setUpPostgreSQLTest(t *testing.T, pluginName string, connectAttrs map[string]string) (PostgreSQLTestData, func()) {
	var testData PostgreSQLTestData
	testData.Cfg = NewPostgreSQLConfig(pluginName, connectAttrs)
	testData.Logger = log.NewZapLogger(zaptest.NewLogger(t))
	mh := metricstest.NewCaptureHandler()
	testData.Metrics = mh.StartCapture()
	SetupPostgreSQLDatabase(t, testData.Cfg)
	SetupPostgreSQLSchema(t, testData.Cfg)
	if pluginName == "postgres12_limitless" && limitlessAuroraMode() { // xtemporal: visibility cannot use Limitless
		testData.VisibilityCfg = limitlessVisibilityConfig(t)
		SetupPostgreSQLDatabase(t, testData.VisibilityCfg)
		loadLimitlessVisibilitySchema(t, testData.VisibilityCfg)
	}

	testData.Factory = sql.NewFactory(
		*testData.Cfg,
		resolver.NewNoopResolver(),
		testPostgreSQLClusterName,
		testData.Logger,
		mh,
		serialization.NewSerializer(),
	)

	tearDown := func() {
		testData.Factory.Close()
		mh.StopCapture(testData.Metrics)
		if testData.VisibilityCfg != nil { // xtemporal: remove the suite's visibility database
			TearDownPostgreSQLDatabase(t, testData.VisibilityCfg)
		}
		TearDownPostgreSQLDatabase(t, testData.Cfg)
	}

	return testData, tearDown
}

// NewPostgreSQLConfig returns a new MySQL config for test
func NewPostgreSQLConfig(pluginName string, connectAttrs map[string]string) *config.SQL {
	if pluginName == "postgres12_limitless" && limitlessAuroraMode() {
		return limitlessAuroraConfig()
	}
	return &config.SQL{
		User:     testPostgreSQLUser,
		Password: testPostgreSQLPassword,
		ConnectAddr: net.JoinHostPort(
			environment.GetPostgreSQLAddress(),
			strconv.Itoa(environment.GetPostgreSQLPort()),
		),
		ConnectProtocol:   testPostgreSQLConnectionProtocol,
		PluginName:        pluginName,
		DatabaseName:      testPostgreSQLDatabaseNamePrefix + shuffle.String(testPostgreSQLDatabaseNameSuffix),
		ConnectAttributes: connectAttrs,
	}
}

func SetupPostgreSQLDatabase(t *testing.T, cfg *config.SQL) {
	if cfg.PluginName == "postgres12_limitless" && limitlessAuroraMode() {
		name, err := sql.CreateLimitlessTestDatabase(cfg, os.Getenv("LIMITLESS_ADMIN_DATABASE"))
		require.NoError(t, err)
		cfg.DatabaseName = name
		return
	}
	adminCfg := *cfg
	// NOTE need to connect with empty name to create new database
	adminCfg.DatabaseName = ""

	db, err := sql.NewSQLAdminDB(sqlplugin.DbKindUnknown, &adminCfg, resolver.NewNoopResolver(), log.NewTestLogger(), metrics.NoopMetricsHandler)
	if err != nil {
		t.Fatalf("unable to create PostgreSQL admin DB: %v", err)
	}
	defer func() { _ = db.Close() }()

	err = db.CreateDatabase(cfg.DatabaseName)
	if err != nil {
		t.Fatalf("unable to create PostgreSQL database: %v", err)
	}
}

func SetupPostgreSQLSchema(t *testing.T, cfg *config.SQL) {
	if cfg.PluginName == "postgres12_limitless" { // xtemporal: use the Limitless schema
		loadLimitlessSchema(t, cfg)
		return
	}
	db, err := sql.NewSQLAdminDB(sqlplugin.DbKindUnknown, cfg, resolver.NewNoopResolver(), log.NewTestLogger(), metrics.NoopMetricsHandler)
	if err != nil {
		t.Fatalf("unable to create PostgreSQL admin DB: %v", err)
	}
	defer func() { _ = db.Close() }()

	schemaPath, err := filepath.Abs(testPostgreSQLExecutionSchema)
	if err != nil {
		t.Fatal(err)
	}

	statements, err := p.LoadAndSplitQuery([]string{schemaPath})
	if err != nil {
		t.Fatal(err)
	}

	for _, stmt := range statements {
		if err = db.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}

	schemaPath, err = filepath.Abs(testPostgreSQLVisibilitySchema)
	if err != nil {
		t.Fatal(err)
	}

	statements, err = p.LoadAndSplitQuery([]string{schemaPath})
	if err != nil {
		t.Fatal(err)
	}

	for _, stmt := range statements {
		if err = db.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}
}

func TearDownPostgreSQLDatabase(t *testing.T, cfg *config.SQL) {
	adminCfg := *cfg
	// NOTE need to connect with empty name to create new database
	adminCfg.DatabaseName = ""
	if cfg.PluginName == "postgres12_limitless" && limitlessAuroraMode() {
		adminCfg.DatabaseName = os.Getenv("LIMITLESS_ADMIN_DATABASE")
	}

	db, err := sql.NewSQLAdminDB(sqlplugin.DbKindUnknown, &adminCfg, resolver.NewNoopResolver(), log.NewTestLogger(), metrics.NoopMetricsHandler)
	if err != nil {
		t.Fatalf("unable to create PostgreSQL admin DB: %v", err)
	}
	defer func() { _ = db.Close() }()

	err = db.DropDatabase(cfg.DatabaseName)
	if err != nil {
		t.Fatalf("unable to drop PostgreSQL database: %v", err)
	}
}
