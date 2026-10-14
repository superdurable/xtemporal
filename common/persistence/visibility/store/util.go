package store

import (
	"github.com/superdurable/xtemporal/chasm"
	"github.com/superdurable/xtemporal/common/searchattribute"
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
