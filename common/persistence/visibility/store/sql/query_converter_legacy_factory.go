package sql

import (
	"github.com/superdurable/temporal-server/chasm"
	"github.com/superdurable/temporal-server/common/namespace"
	"github.com/superdurable/temporal-server/common/persistence/sql/sqlplugin/mysql"
	"github.com/superdurable/temporal-server/common/persistence/sql/sqlplugin/postgresql"
	"github.com/superdurable/temporal-server/common/persistence/sql/sqlplugin/sqlite"
	"github.com/superdurable/temporal-server/common/searchattribute"
)

func NewQueryConverterLegacy(
	pluginName string,
	namespaceName namespace.Name,
	namespaceID namespace.ID,
	saTypeMap searchattribute.NameTypeMap,
	saMapper searchattribute.Mapper,
	queryString string,
	chasmMapper *chasm.VisibilitySearchAttributesMapper,
	archetypeID chasm.ArchetypeID,
) *QueryConverterLegacy {
	switch pluginName {
	case mysql.PluginName:
		return newMySQLQueryConverter(namespaceName, namespaceID, saTypeMap, saMapper, queryString, chasmMapper, archetypeID)
	case postgresql.PluginName, postgresql.PluginNamePGX:
		return newPostgreSQLQueryConverter(namespaceName, namespaceID, saTypeMap, saMapper, queryString, chasmMapper, archetypeID)
	case sqlite.PluginName:
		return newSqliteQueryConverter(namespaceName, namespaceID, saTypeMap, saMapper, queryString, chasmMapper, archetypeID)
	default:
		return nil
	}
}
