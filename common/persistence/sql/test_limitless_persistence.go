package sql

import (
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/superdurable/xtemporal/common/config"
	"github.com/superdurable/xtemporal/common/log/tag"
	"github.com/superdurable/xtemporal/common/persistence/sql/sqlplugin"
)

func limitlessVisibilityConfig() (config.SQL, error) {
	for _, name := range []string{"LIMITLESS_VISIBILITY_HOST", "LIMITLESS_VISIBILITY_PORT", "LIMITLESS_VISIBILITY_USER", "LIMITLESS_VISIBILITY_PASSWORD"} {
		if os.Getenv(name) == "" {
			return config.SQL{}, fmt.Errorf("%s is required when LIMITLESS_TEST=aurora", name)
		}
	}
	port, err := strconv.Atoi(os.Getenv("LIMITLESS_VISIBILITY_PORT"))
	if err != nil || port <= 0 || port > 65535 {
		return config.SQL{}, errors.New("LIMITLESS_VISIBILITY_PORT must be a valid port when LIMITLESS_TEST=aurora")
	}
	sslmode := os.Getenv("LIMITLESS_VISIBILITY_SSLMODE")
	if sslmode == "" {
		sslmode = "disable"
	}
	return config.SQL{
		User: os.Getenv("LIMITLESS_VISIBILITY_USER"), Password: os.Getenv("LIMITLESS_VISIBILITY_PASSWORD"),
		ConnectAddr:     net.JoinHostPort(os.Getenv("LIMITLESS_VISIBILITY_HOST"), strconv.Itoa(port)),
		ConnectProtocol: "tcp", PluginName: "postgres12_pgx",
		DatabaseName:      "test_visibility_" + strings.ReplaceAll(uuid.NewString(), "-", ""),
		ConnectAttributes: map[string]string{"sslmode": sslmode},
	}, nil
}

func (s *TestCluster) createLimitlessVisibilityDatabase() {
	cfg := *s.visibilityCfg
	cfg.DatabaseName = ""
	db := s.newAdminDB(sqlplugin.DbKindUnknown, &cfg)
	defer s.closeAdminDB(db)
	if err := db.CreateDatabase(s.visibilityCfg.DatabaseName); err != nil {
		s.logger.Fatal("Create visibility database", tag.Error(err))
	}
}

func (s *TestCluster) dropLimitlessVisibilityDatabase() {
	cfg := *s.visibilityCfg
	cfg.DatabaseName = ""
	db := s.newAdminDB(sqlplugin.DbKindUnknown, &cfg)
	defer s.closeAdminDB(db)
	if err := db.DropDatabase(s.visibilityCfg.DatabaseName); err != nil {
		s.logger.Fatal("Drop visibility database", tag.Error(err))
	}
}

func (s *TestCluster) resetLimitlessTables() { // xtemporal: reset Aurora suites sequentially without CREATE DATABASE
	db := s.newAdminDB(sqlplugin.DbKindUnknown, &s.cfg)
	defer s.closeAdminDB(db)
	tables, err := db.ListTables(s.cfg.DatabaseName)
	if err != nil {
		s.logger.Fatal("ListTables", tag.Error(err))
	}
	for _, table := range tables {
		quoted := `"` + strings.ReplaceAll(table, `"`, `""`) + `"`
		if err := db.Exec("DROP TABLE " + quoted + " CASCADE"); err != nil {
			s.logger.Fatal("DropTable", tag.Error(err))
		}
	}
	if err := db.Exec(`DO $$ DECLARE f record; BEGIN FOR f IN
		SELECT p.proname, pg_get_function_identity_arguments(p.oid) AS args
		FROM pg_proc p JOIN pg_namespace n ON n.oid = p.pronamespace WHERE n.nspname = 'public'
		LOOP EXECUTE format('DROP FUNCTION public.%I(%s) CASCADE', f.proname, f.args); END LOOP; END $$`); err != nil {
		s.logger.Fatal("Drop public functions", tag.Error(err))
	}
}
