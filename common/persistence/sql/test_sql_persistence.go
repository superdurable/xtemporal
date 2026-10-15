package sql

import (
	"fmt"
	"os"
	"path"
	"strings"
	"time"

	"github.com/superdurable/xtemporal/common/backoff"
	"github.com/superdurable/xtemporal/common/config"
	"github.com/superdurable/xtemporal/common/dynamicconfig"
	"github.com/superdurable/xtemporal/common/log"
	"github.com/superdurable/xtemporal/common/log/tag"
	"github.com/superdurable/xtemporal/common/metrics"
	p "github.com/superdurable/xtemporal/common/persistence"
	"github.com/superdurable/xtemporal/common/persistence/sql/sqlplugin"
	"github.com/superdurable/xtemporal/common/primitives"
	"github.com/superdurable/xtemporal/common/resolver"
	"github.com/superdurable/xtemporal/tests/testutils"
)

// TestCluster allows executing cassandra operations in testing.
type TestCluster struct {
	dbName         string
	schemaDir      string
	cfg            config.SQL
	faultInjection *config.FaultInjection
	logger         log.Logger
	visibilityCfg  *config.SQL // xtemporal: Aurora visibility uses a separate PostgreSQL database
}

type forceDatabaseDropper interface {
	ForceDropDatabase(database string) error
}

// NewTestCluster returns a new SQL test cluster
func NewTestCluster(
	pluginName string,
	dbName string,
	username string,
	password string,
	host string,
	port int,
	connectAttributes map[string]string,
	schemaDir string,
	faultInjection *config.FaultInjection,
	logger log.Logger,
) *TestCluster {
	var result TestCluster
	result.logger = logger
	result.dbName = dbName

	result.schemaDir = schemaDir
	result.cfg = config.SQL{
		User:               username,
		Password:           password,
		ConnectAddr:        fmt.Sprintf("%v:%v", host, port),
		ConnectProtocol:    "tcp",
		PluginName:         pluginName,
		DatabaseName:       dbName,
		TaskScanPartitions: 4,
		ConnectAttributes:  connectAttributes,
	}

	result.faultInjection = faultInjection
	if pluginName == "postgres12_limitless" && os.Getenv("LIMITLESS_TEST") == "aurora" { // xtemporal: separate visibility from Limitless
		visibilityCfg, err := limitlessVisibilityConfig()
		if err != nil {
			logger.Fatal("Invalid Limitless visibility configuration", tag.Error(err))
		}
		result.visibilityCfg = &visibilityCfg
	}
	return &result
}

// DatabaseName from PersistenceTestCluster interface
func (s *TestCluster) DatabaseName() string {
	return s.dbName
}

// SetupTestDatabase from PersistenceTestCluster interface
func (s *TestCluster) SetupTestDatabase() {
	s.CreateDatabase()

	if s.schemaDir == "" {
		s.logger.Info("No schema directory provided, skipping schema setup")
		return
	}

	schemaDir := s.schemaDir + "/"
	if !strings.HasPrefix(schemaDir, "/") && !strings.HasPrefix(schemaDir, "../") {
		temporalPackageDir := testutils.GetRepoRootDirectory()
		schemaDir = path.Join(temporalPackageDir, schemaDir)
	}
	s.LoadSchema(path.Join(schemaDir, "temporal", "schema.sql"))
	visibilitySchema := path.Join(schemaDir, "visibility", "schema.sql")
	if s.cfg.PluginName == "postgres12_limitless" { // xtemporal: Limitless uses PostgreSQL visibility
		visibilitySchema = path.Join(testutils.GetRepoRootDirectory(), "schema/postgresql/v12/visibility/schema.sql")
	}
	if s.cfg.PluginName == "paradedb" { // xtemporal: ParadeDB has its own visibility schema
		visibilitySchema = path.Join(testutils.GetRepoRootDirectory(), "schema/paradedb/visibility/schema.sql")
	}
	if s.visibilityCfg != nil { // xtemporal: visibility schema is unsupported on Limitless
		s.loadSchemaForConfig(visibilitySchema, s.visibilityCfg)
	} else {
		s.LoadSchema(visibilitySchema)
	}
	s.loadSchemaVersion()
	if s.visibilityCfg != nil { // xtemporal: record the visibility version in its own database
		s.loadSchemaVersionForConfig(sqlplugin.DbKindVisibility, s.visibilityCfg)
	}
	if s.cfg.PluginName == "postgres12_limitless" { // xtemporal: verify both configured stores
		if err := VerifyCompatibleVersion(s.Config(), resolver.NewNoopResolver(), s.logger); err != nil {
			s.logger.Fatal("VerifyCompatibleVersion", tag.Error(err))
		}
	}
}

// Config returns the persistence config for connecting to this test cluster
func (s *TestCluster) Config() config.Persistence {
	cfg := s.cfg
	if cfg.PluginName == "paradedb" { // xtemporal: keep the default store on PostgreSQL
		mainCfg := cfg
		mainCfg.PluginName = "postgres12_pgx"
		return config.Persistence{
			DefaultStore:    "test",
			VisibilityStore: "test-visibility",
			DataStores: map[string]config.DataStore{
				"test":            {SQL: &mainCfg, FaultInjection: s.faultInjection},
				"test-visibility": {SQL: &cfg, FaultInjection: s.faultInjection},
			},
			TransactionSizeLimit: dynamicconfig.GetIntPropertyFn(primitives.DefaultTransactionSizeLimit),
		}
	}
	if cfg.PluginName == "postgres12_limitless" { // xtemporal: keep visibility on PostgreSQL
		visibilityCfg := cfg
		visibilityCfg.PluginName = "postgres12_pgx"
		if s.visibilityCfg != nil { // xtemporal: Aurora visibility is on vanilla PostgreSQL
			visibilityCfg = *s.visibilityCfg
		}
		return config.Persistence{
			DefaultStore:    "test",
			VisibilityStore: "test-visibility",
			DataStores: map[string]config.DataStore{
				"test":            {SQL: &cfg, FaultInjection: s.faultInjection},
				"test-visibility": {SQL: &visibilityCfg, FaultInjection: s.faultInjection},
			},
			TransactionSizeLimit: dynamicconfig.GetIntPropertyFn(primitives.DefaultTransactionSizeLimit),
		}
	}
	return config.Persistence{
		DefaultStore:    "test",
		VisibilityStore: "test",
		DataStores: map[string]config.DataStore{
			"test": {SQL: &cfg, FaultInjection: s.faultInjection},
		},
		TransactionSizeLimit: dynamicconfig.GetIntPropertyFn(primitives.DefaultTransactionSizeLimit),
	}
}

// TearDownTestDatabase from PersistenceTestCluster interface
func (s *TestCluster) TearDownTestDatabase() {
	s.DropDatabase()
	if s.visibilityCfg != nil { // xtemporal: remove each suite's visibility database
		s.dropLimitlessVisibilityDatabase()
	}
}

// CreateDatabase from PersistenceTestCluster interface
func (s *TestCluster) CreateDatabase() {
	if s.cfg.PluginName == "postgres12_limitless" && os.Getenv("LIMITLESS_TEST") == "aurora" { // xtemporal: Aurora databases are pre-created
		s.resetLimitlessTables()
		s.createLimitlessVisibilityDatabase()
		return
	}
	cfg2 := s.cfg
	// NOTE need to connect with empty name to create new database
	if cfg2.PluginName != "sqlite" {
		cfg2.DatabaseName = ""
	}

	db := s.newAdminDB(sqlplugin.DbKindUnknown, &cfg2)
	defer s.closeAdminDB(db)
	err := db.CreateDatabase(s.cfg.DatabaseName)
	if err != nil {
		panic(err)
	}
	s.logger.Info("created database", tag.String("database", s.cfg.DatabaseName))
}

// DropDatabase from PersistenceTestCluster interface
func (s *TestCluster) DropDatabase() {
	if s.cfg.PluginName == "postgres12_limitless" && os.Getenv("LIMITLESS_TEST") == "aurora" { // xtemporal: preserve the pre-created Aurora database
		return
	}
	cfg2 := s.cfg

	if cfg2.PluginName == "sqlite" && cfg2.DatabaseName != ":memory:" && cfg2.ConnectAttributes["mode"] != "memory" {
		if len(cfg2.DatabaseName) > 3 { // 3 should mean not ., .., empty, or /
			// Remove main database file
			_ = os.Remove(cfg2.DatabaseName)
			// Remove WAL mode files (may not exist if WAL wasn't used)
			_ = os.Remove(cfg2.DatabaseName + "-wal")
			_ = os.Remove(cfg2.DatabaseName + "-shm")
		}
		return
	}

	// NOTE need to connect with empty name to drop the database
	cfg2.DatabaseName = ""
	db := s.newAdminDB(sqlplugin.DbKindUnknown, &cfg2)
	defer s.closeAdminDB(db)
	var err error
	if dropper, ok := db.(forceDatabaseDropper); ok {
		err = dropper.ForceDropDatabase(s.cfg.DatabaseName)
	} else {
		err = db.DropDatabase(s.cfg.DatabaseName)
	}
	if err != nil {
		panic(err)
	}
	s.logger.Info("dropped database", tag.String("database", s.cfg.DatabaseName))
}

// LoadSchema from PersistenceTestCluster interface
func (s *TestCluster) LoadSchema(schemaFile string) {
	s.loadSchemaForConfig(schemaFile, &s.cfg)
}

func (s *TestCluster) loadSchemaForConfig(schemaFile string, cfg *config.SQL) { // xtemporal: load visibility on its own PostgreSQL connection
	statements, err := p.LoadAndSplitQuery([]string{schemaFile})
	if err != nil {
		s.logger.Fatal(
			fmt.Sprintf("LoadSchema %s: %v", schemaFile, err),
			tag.Error(err),
			tag.String("database", cfg.DatabaseName),
			tag.String("schema-file", schemaFile),
		)
	}

	db := s.newAdminDB(sqlplugin.DbKindUnknown, cfg)
	defer s.closeAdminDB(db)

	if rewriter, ok := db.(sqlplugin.SchemaStatementRewriter); ok {
		statements = rewriter.RewriteSchemaStatements(statements)
	}

	for i, stmt := range statements {
		if err = db.Exec(stmt); err != nil {
			s.logger.Fatal(
				fmt.Sprintf("LoadSchema statement %d for database %s: %v", i, cfg.DatabaseName, err),
				tag.Error(err),
				tag.String("database", cfg.DatabaseName),
				tag.String("schema-file", schemaFile),
				tag.Int("statement-index", i),
				tag.String("statement", stmt),
			)
		}
	}
	s.logger.Info("loaded schema")
}

func (s *TestCluster) loadSchemaVersion() {
	s.loadSchemaVersionForConfig(sqlplugin.DbKindMain, &s.cfg)
}

func (s *TestCluster) loadSchemaVersionForConfig(kind sqlplugin.DbKind, cfg *config.SQL) { // xtemporal: keep store versions in their respective databases
	db := s.newAdminDB(kind, cfg)
	defer s.closeAdminDB(db)

	expectedVersion := db.ExpectedVersion()
	if err := db.CreateSchemaVersionTables(); err != nil {
		s.logger.Fatal("CreateSchemaVersionTables", tag.Error(err))
	}
	if err := db.UpdateSchemaVersion(cfg.DatabaseName, expectedVersion, expectedVersion); err != nil {
		s.logger.Fatal("UpdateSchemaVersion", tag.Error(err))
	}
	if err := db.WriteSchemaUpdateLog("0", expectedVersion, "", "initial version"); err != nil {
		s.logger.Fatal("WriteSchemaUpdateLog", tag.Error(err))
	}
	s.logger.Info("loaded schema version", tag.String("version", expectedVersion))
}

func (s *TestCluster) newAdminDB(kind sqlplugin.DbKind, cfg *config.SQL) sqlplugin.AdminDB {
	var db sqlplugin.AdminDB
	var err error
	err = backoff.ThrottleRetry(
		func() error {
			db, err = NewSQLAdminDB(kind, cfg, resolver.NewNoopResolver(), log.NewTestLogger(), metrics.NoopMetricsHandler)
			return err
		},
		backoff.NewExponentialRetryPolicy(time.Second).WithExpirationInterval(time.Minute),
		nil,
	)
	if err != nil {
		s.logger.Fatal("NewSQLAdminDB", tag.Error(err))
	}
	return db
}

func (s *TestCluster) closeAdminDB(db sqlplugin.AdminDB) {
	if err := db.Close(); err != nil {
		s.logger.Fatal("Close schema DB", tag.Error(err))
	}
}
