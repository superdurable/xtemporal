package store

import (
	"github.com/superdurable/temporal-server/chasm"
	"github.com/superdurable/temporal-server/common/searchattribute"
)

func CombineTypeMaps(
	customTypeMap searchattribute.NameTypeMap,
	chasmTypeMap *chasm.VisibilitySearchAttributesMapper,
) searchattribute.NameTypeMap {
	return searchattribute.MergeNameTypeMaps(
		customTypeMap,
		searchattribute.NewNameTypeMap(chasmTypeMap.SATypeMap()),
	)
}
