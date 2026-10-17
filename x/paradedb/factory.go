package paradedb

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jmoiron/sqlx"
	"github.com/superdurable/xtemporal/chasm"
	"github.com/superdurable/xtemporal/common/config"
	"github.com/superdurable/xtemporal/common/log"
	"github.com/superdurable/xtemporal/common/metrics"
	"github.com/superdurable/xtemporal/common/namespace"
	"github.com/superdurable/xtemporal/common/persistence/sql/sqlplugin/postgresql/driver"
	"github.com/superdurable/xtemporal/common/persistence/sql/sqlplugin/postgresql/session"
	"github.com/superdurable/xtemporal/common/persistence/visibility"
	"github.com/superdurable/xtemporal/common/persistence/visibility/store"
	"github.com/superdurable/xtemporal/common/resolver"
	"github.com/superdurable/xtemporal/common/searchattribute"
	"gopkg.in/yaml.v3"
)

type VisibilityStoreFactory struct{}

var _ visibility.VisibilityStoreFactory = VisibilityStoreFactory{}

type storeOptions struct {
	Profile string     `yaml:"profile"`
	SQL     config.SQL `yaml:"sql"`
}

func parseStoreOptions(options map[string]any) (storeOptions, error) {
	var result storeOptions
	if len(options) == 0 {
		return result, errors.New("ParadeDB visibility options.sql is required")
	}
	data, err := yaml.Marshal(options)
	if err != nil {
		return result, fmt.Errorf("encode ParadeDB visibility options: %w", err)
	}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&result); err != nil {
		return result, fmt.Errorf("invalid ParadeDB visibility options: %w", err)
	}
	if result.Profile != "" && result.Profile != "dex" {
		return result, fmt.Errorf("unsupported ParadeDB visibility profile %q", result.Profile)
	}
	if result.SQL.PluginName != "" && result.SQL.PluginName != "postgres12_pgx" {
		return result, errors.New("ParadeDB visibility options.sql.pluginName must be postgres12_pgx")
	}
	if result.SQL.ConnectAddr == "" || result.SQL.DatabaseName == "" {
		return result, errors.New("ParadeDB visibility options.sql requires connectAddr and databaseName")
	}
	if result.SQL.ConnectProtocol != "" && result.SQL.ConnectProtocol != "tcp" {
		return result, errors.New("ParadeDB visibility options.sql.connectProtocol must be tcp")
	}
	result.SQL.PluginName = "postgres12_pgx"
	result.SQL.ConnectProtocol = "tcp"
	return result, nil
}

func (VisibilityStoreFactory) NewVisibilityStore(
	cfg config.CustomDatastoreConfig,
	saProvider searchattribute.Provider,
	mapperProvider searchattribute.MapperProvider,
	namespaceRegistry namespace.Registry,
	chasmRegistry *chasm.Registry,
	r resolver.ServiceResolver,
	logger log.Logger,
	metricsHandler metrics.Handler,
) (store.VisibilityStore, error) {
	if cfg.Name != StoreName || strings.TrimSpace(cfg.IndexName) == "" {
		return nil, fmt.Errorf("ParadeDB visibility requires customDatastore.name=%q and nonempty indexName", StoreName)
	}
	options, err := parseStoreOptions(cfg.Options)
	if err != nil {
		return nil, err
	}
	if r == nil {
		r = resolver.NewNoopResolver()
	}
	if logger == nil {
		logger = log.NewNoopLogger()
	}
	base := options.SQL
	base.ConnectAttributes = make(map[string]string, len(options.SQL.ConnectAttributes)+2)
	for k, v := range options.SQL.ConnectAttributes {
		base.ConnectAttributes[k] = v
	}
	base.ConnectAttributes["jit"] = "off"
	base.ConnectAttributes["plan_cache_mode"] = "auto"
	btree, err := session.NewSession(&base, &driver.PGXDriver{}, r)
	if err != nil {
		return nil, fmt.Errorf("connect ParadeDB visibility: %w", err)
	}
	if err := checkSchema(context.Background(), btree.DB, options.SQL.DatabaseName); err != nil {
		return nil, closeWithError(btree.DB, err)
	}
	bm25Cfg := options.SQL
	bm25Cfg.ConnectAttributes = make(map[string]string, len(options.SQL.ConnectAttributes)+1)
	for k, v := range options.SQL.ConnectAttributes {
		bm25Cfg.ConnectAttributes[k] = v
	}
	bm25Cfg.ConnectAttributes["plan_cache_mode"] = "force_custom_plan"
	bm25, err := session.NewSession(&bm25Cfg, &driver.PGXDriver{}, r)
	if err != nil {
		return nil, closeWithError(btree.DB, fmt.Errorf("connect ParadeDB BM25 visibility route: %w", err))
	}
	return &VisibilityStore{btree: btree.DB, bm25: bm25.DB, indexName: cfg.IndexName, saProvider: saProvider,
		mapperProvider: mapperProvider, namespaceRegistry: namespaceRegistry, chasmRegistry: chasmRegistry,
		metricsHandler: metricsHandler, logger: logger}, nil
}

func closeWithError(db *sqlx.DB, cause error) error {
	if err := db.Close(); err != nil {
		return fmt.Errorf("%w; close ParadeDB connection: %v", cause, err)
	}
	return cause
}

func checkSchema(ctx context.Context, db *sqlx.DB, databaseName string) error {
	var version string
	if err := db.GetContext(ctx, &version, `SELECT curr_version FROM schema_version WHERE version_partition = 0 AND db_name = $1`, databaseName); err != nil {
		return fmt.Errorf("ParadeDB visibility requires schema version 2.0 (missing or unreadable schema_version): %w", err)
	}
	if version != "2.0" {
		return fmt.Errorf("ParadeDB visibility requires schema version 2.0, found %q", version)
	}
	var installed bool
	if err := db.GetContext(ctx, &installed, `SELECT EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'pg_search')`); err != nil {
		return fmt.Errorf("check ParadeDB pg_search extension: %w", err)
	}
	if !installed {
		return errors.New("ParadeDB visibility requires installed pg_search extension")
	}
	return nil
}
