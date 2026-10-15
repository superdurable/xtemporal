package sql

import (
	"strings"

	"github.com/superdurable/xtemporal/common/log/tag"
	"github.com/superdurable/xtemporal/common/persistence/sql/sqlplugin"
)

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
}
