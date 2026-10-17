package visibility

import (
	"github.com/superdurable/xtemporal/common/dynamicconfig"
	"github.com/superdurable/xtemporal/common/persistence/sql/sqlplugin/mysql"
	"github.com/superdurable/xtemporal/common/persistence/sql/sqlplugin/postgresql"
	"github.com/superdurable/xtemporal/common/persistence/sql/sqlplugin/sqlite"
)

const (
	// xtemporal: ParadeDB Dex uses cluster-level search attributes.
	ParadeDBDexStoreName = "paradedb-dex"
	// SecondaryVisibilityWritingModeOff means do not write to advanced visibility store
	SecondaryVisibilityWritingModeOff = "off"
	// SecondaryVisibilityWritingModeOn means only write to advanced visibility store
	SecondaryVisibilityWritingModeOn = "on"
	// SecondaryVisibilityWritingModeDual means write to both normal visibility and advanced visibility store
	SecondaryVisibilityWritingModeDual = "dual"
)

// UsesClusterSearchAttributes reports whether a store registers real names in cluster metadata.
// xtemporal: ParadeDB Dex shares this capability with Elasticsearch.
func UsesClusterSearchAttributes(storeName string) bool {
	return storeName == "elasticsearch" || storeName == ParadeDBDexStoreName
}

func AllowListForValidation(
	storeNames []string,
	allowList dynamicconfig.BoolPropertyFnWithNamespaceFilter,
) dynamicconfig.BoolPropertyFnWithNamespaceFilter {
	if len(storeNames) == 0 {
		return dynamicconfig.GetBoolPropertyFnFilteredByNamespace(false)
	}

	switch storeNames[0] {
	case mysql.PluginName, postgresql.PluginName, postgresql.PluginNamePGX, sqlite.PluginName:
		// Advanced visibility with SQL DB don't support list of values
		return dynamicconfig.GetBoolPropertyFnFilteredByNamespace(false)
	default:
		// Otherwise (ES), check dynamic config
		return allowList
	}
}
