package testcore

import (
	"flag"

	"github.com/superdurable/xtemporal/common/config"
	"github.com/superdurable/xtemporal/common/persistence/sql/sqlplugin/mysql"
	"github.com/superdurable/xtemporal/common/persistence/sql/sqlplugin/postgresql"
	"github.com/superdurable/xtemporal/common/persistence/sql/sqlplugin/sqlite"
)

// cliFlags contains the feature flags for functional tests
var cliFlags struct {
	persistenceType      string
	persistenceDriver    string
	enableFaultInjection string
}

func init() {
	flag.StringVar(&cliFlags.persistenceType, "persistenceType", "sql", "type of persistence - [nosql or sql]")
	flag.StringVar(&cliFlags.persistenceDriver, "persistenceDriver", "sqlite", "driver of nosql/sql - [cassandra, mysql8, postgres12, sqlite]")
	flag.StringVar(&cliFlags.enableFaultInjection, "enableFaultInjection", "", "enable global fault injection")
}

func UseSQLVisibility() bool {
	switch cliFlags.persistenceDriver {
	case mysql.PluginName, postgresql.PluginName, postgresql.PluginNamePGX, sqlite.PluginName, "paradedb": // xtemporal: ParadeDB visibility
		return true
	// If the main storage is Cassandra, Elasticsearch is used for visibility.
	default:
		return false
	}
}

func UseCassandraPersistence() bool {
	return cliFlags.persistenceType == config.StoreTypeNoSQL && cliFlags.persistenceDriver == "cassandra"
}
