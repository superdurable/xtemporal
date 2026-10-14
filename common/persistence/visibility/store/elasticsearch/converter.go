package elasticsearch

import (
	"github.com/temporalio/sqlparser"
	"github.com/superdurable/xtemporal/chasm"
	"github.com/superdurable/xtemporal/common/persistence/visibility/store/query"
	"github.com/superdurable/xtemporal/common/searchattribute"
)

var allowedComparisonOperators = map[string]struct{}{
	sqlparser.EqualStr:         {},
	sqlparser.NotEqualStr:      {},
	sqlparser.GreaterThanStr:   {},
	sqlparser.GreaterEqualStr:  {},
	sqlparser.LessThanStr:      {},
	sqlparser.LessEqualStr:     {},
	sqlparser.InStr:            {},
	sqlparser.NotInStr:         {},
	sqlparser.StartsWithStr:    {},
	sqlparser.NotStartsWithStr: {},
}

func NewQueryConverterLegacy(
	fnInterceptor query.FieldNameInterceptor,
	fvInterceptor query.FieldValuesInterceptor,
	saNameType searchattribute.NameTypeMap,
	chasmMapper *chasm.VisibilitySearchAttributesMapper,
) *query.ConverterLegacy {
	if fnInterceptor == nil {
		fnInterceptor = &query.NopFieldNameInterceptor{}
	}

	if fvInterceptor == nil {
		fvInterceptor = &query.NopFieldValuesInterceptor{}
	}

	rangeCond := query.NewRangeCondConverter(fnInterceptor, fvInterceptor, true)
	comparisonExpr := query.NewComparisonExprConverter(fnInterceptor, fvInterceptor, allowedComparisonOperators, saNameType, chasmMapper)
	is := query.NewIsConverter(fnInterceptor)

	whereConverter := &query.WhereConverter{
		RangeCond:      rangeCond,
		ComparisonExpr: comparisonExpr,
		Is:             is,
	}
	whereConverter.And = query.NewAndConverter(whereConverter)
	whereConverter.Or = query.NewOrConverter(whereConverter)

	return query.NewConverterLegacy(fnInterceptor, whereConverter)
}
