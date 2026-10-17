package main

import (
	"os"

	_ "github.com/superdurable/xtemporal/common/persistence/sql/sqlplugin/mysql"      // needed to load mysql plugin
	_ "github.com/superdurable/xtemporal/common/persistence/sql/sqlplugin/postgresql" // needed to load postgresql plugin
	_ "github.com/superdurable/xtemporal/common/persistence/sql/sqlplugin/sqlite"     // needed to load sqlite plugin
	"github.com/superdurable/xtemporal/tools/sql"
	_ "github.com/superdurable/xtemporal/x/limitless" // xtemporal: register Limitless default-store plugin
	_ "github.com/superdurable/xtemporal/x/paradedb"  // xtemporal: register ParadeDB schema tooling plugin
)

func main() {
	if err := sql.RunTool(os.Args); err != nil {
		os.Exit(1)
	}
}
