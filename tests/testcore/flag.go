package testcore

import (
	"flag"

	"github.com/superdurable/xtemporal/common/config"
	"github.com/superdurable/xtemporal/common/persistence/sql/sqlplugin/mysql"
	"github.com/superdurable/xtemporal/common/persistence/sql/sqlplugin/postgresql"
	"github.com/superdurable/xtemporal/common/persistence/sql/sqlplugin/sqlite"
	"github.com/superdurable/xtemporal/common/persistence/visibility"
)

// cliFlags contains the feature flags for functional tests
var cliFlags struct {
	persistenceType      string
	persistenceDriver    string
	visibilityStore      string // xtemporal: select the custom visibility backend independently of the SQL driver
	enableFaultInjection string
}

func init() {
	flag.StringVar(&cliFlags.persistenceType, "persistenceType", "sql", "type of persistence - [nosql or sql]")
	flag.StringVar(&cliFlags.persistenceDriver, "persistenceDriver", "sqlite", "driver of nosql/sql - [cassandra, mysql8, postgres12, postgres12_limitless, postgres12_pgx, sqlite]") // xtemporal: Limitless functional tests
	flag.StringVar(&cliFlags.visibilityStore, "visibilityStore", "", "visibility backend override - [paradedb-dex]")                                                                  // xtemporal: dedicated Dex test mode
	flag.StringVar(&cliFlags.enableFaultInjection, "enableFaultInjection", "", "enable global fault injection")
}

func UseSQLVisibility() bool {
	if UseParadeDBVisibility() {
		return false // xtemporal: the custom store is not SQL visibility even when its main store is PostgreSQL
	}
	switch cliFlags.persistenceDriver {
	case mysql.PluginName, postgresql.PluginName, postgresql.PluginNamePGX, sqlite.PluginName, "postgres12_limitless": // xtemporal: Limitless uses PostgreSQL visibility
		return true
	// If the main storage is Cassandra, Elasticsearch is used for visibility.
	default:
		return false
	}
}

// UseParadeDBVisibility reports the dedicated Dex test mode.
// xtemporal: It is independent of the SQL persistence driver.
func UseParadeDBVisibility() bool {
	return cliFlags.visibilityStore == visibility.ParadeDBDexStoreName
}

func UseESVisibility() bool {
	return !UseSQLVisibility() && !UseParadeDBVisibility()
}

func UseCassandraPersistence() bool {
	return cliFlags.persistenceType == config.StoreTypeNoSQL && cliFlags.persistenceDriver == "cassandra"
}
