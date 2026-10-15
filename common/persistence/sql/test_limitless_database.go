package sql

import (
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/google/uuid"
	"github.com/superdurable/xtemporal/common/config"
	"github.com/superdurable/xtemporal/common/log"
	"github.com/superdurable/xtemporal/common/metrics"
	"github.com/superdurable/xtemporal/common/persistence/sql/sqlplugin"
	"github.com/superdurable/xtemporal/common/resolver"
)

var limitlessDatabaseCreateMu sync.Mutex

// CreateLimitlessTestDatabase creates an isolated database through the existing admin database.
func CreateLimitlessTestDatabase(cfg *config.SQL, adminDatabase string) (name string, err error) {
	if adminDatabase == "" {
		return "", errors.New("LIMITLESS_ADMIN_DATABASE is required when LIMITLESS_TEST=aurora")
	}
	adminCfg := *cfg
	adminCfg.DatabaseName = adminDatabase
	db, err := NewSQLAdminDB(sqlplugin.DbKindUnknown, &adminCfg, resolver.NewNoopResolver(), log.NewTestLogger(), metrics.NoopMetricsHandler)
	if err != nil {
		return "", fmt.Errorf("connect to Limitless admin database: %w", err)
	}
	defer func() { err = errors.Join(err, db.Close()) }()

	limitlessDatabaseCreateMu.Lock()
	defer limitlessDatabaseCreateMu.Unlock()
	return createLimitlessTestDatabase(db, cfg.DatabaseName)
}

func createLimitlessTestDatabase(db sqlplugin.AdminDB, name string) (string, error) {
	for range 5 {
		// Names come from test harness UUIDs; quote them because CREATE DATABASE takes an identifier.
		err := db.Exec(`CREATE DATABASE "` + name + `"`)
		if err == nil {
			return name, nil
		}
		var pgError interface{ SQLState() string }
		if !errors.As(err, &pgError) || pgError.SQLState() != "42P04" {
			return "", fmt.Errorf("create Limitless database %s: %w", name, err)
		}
		name = "test_limitless_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	}
	return "", errors.New("create Limitless database: too many duplicate names")
}
