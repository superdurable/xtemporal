package paradedb

import (
	"errors"
	"math"
	"time"

	"github.com/superdurable/xtemporal/chasm"
	"github.com/superdurable/xtemporal/common/log"
	"github.com/superdurable/xtemporal/common/metrics"
	"github.com/superdurable/xtemporal/common/namespace"
	"github.com/superdurable/xtemporal/common/persistence/visibility/store/query"
	"github.com/superdurable/xtemporal/common/searchattribute"
	"github.com/superdurable/xtemporal/common/searchattribute/sadefs"
	"github.com/temporalio/sqlparser"
	enumspb "go.temporal.io/api/enums/v1"
)

type exprKind uint8

const (
	atomExpr exprKind = iota
	andExpr
	orExpr
	notExpr
)

type expression struct {
	kind     exprKind
	children []*expression
	column   *query.SAColumn
	op       string
	values   []any
}

type convertedQuery struct {
	predicate *expression
	groupBy   string
}

type astConverter struct{}

var _ query.StoreQueryConverter[*expression] = (*astConverter)(nil)

func (*astConverter) GetDatetimeFormat() string                         { return time.RFC3339Nano }
func (*astConverter) BuildParenExpr(e *expression) (*expression, error) { return e, nil }
func (*astConverter) BuildNotExpr(e *expression) (*expression, error) {
	return &expression{kind: notExpr, children: []*expression{e}}, nil
}
func (*astConverter) BuildAndExpr(es ...*expression) (*expression, error) {
	return joinExpr(andExpr, es...), nil
}
func (*astConverter) BuildOrExpr(es ...*expression) (*expression, error) {
	return joinExpr(orExpr, es...), nil
}
func joinExpr(kind exprKind, es ...*expression) *expression {
	children := make([]*expression, 0, len(es))
	for _, e := range es {
		if e != nil {
			children = append(children, e)
		}
	}
	if len(children) == 0 {
		return nil
	}
	if len(children) == 1 {
		return children[0]
	}
	return &expression{kind: kind, children: children}
}

func (*astConverter) ConvertComparisonExpr(op string, col *query.SAColumn, value any) (*expression, error) {
	return makeAtom(op, col, value)
}
func (*astConverter) ConvertKeywordComparisonExpr(op string, col *query.SAColumn, value any) (*expression, error) {
	if op == sqlparser.StartsWithStr || op == sqlparser.NotStartsWithStr {
		return nil, query.NewOperatorNotSupportedError(col.Alias, col.ValueType, op)
	}
	return makeAtom(op, col, value)
}
func (*astConverter) ConvertKeywordListComparisonExpr(op string, col *query.SAColumn, value any) (*expression, error) {
	return makeAtom(op, col, value)
}
func (*astConverter) ConvertTextComparisonExpr(op string, col *query.SAColumn, value any) (*expression, error) {
	return makeAtom(op, col, value)
}
func (*astConverter) ConvertRangeExpr(op string, col *query.SAColumn, from, to any) (*expression, error) {
	return makeAtom(op, col, []any{from, to})
}
func (*astConverter) ConvertIsExpr(op string, col *query.SAColumn) (*expression, error) {
	return makeAtom(op, col, nil)
}

func makeAtom(op string, col *query.SAColumn, value any) (*expression, error) {
	if col == nil {
		return nil, query.NewConverterError("%s: empty search attribute", query.InvalidExpressionErrMessage)
	}
	if _, ok := columnForAttribute(col.FieldName, col.ValueType); !ok {
		return nil, query.NewConverterError("%s: %s", query.InvalidSearchAttribute, col.Alias)
	}
	var values []any
	switch op {
	case sqlparser.IsNullStr, sqlparser.IsNotNullStr:
	case sqlparser.InStr, sqlparser.NotInStr, sqlparser.BetweenStr, sqlparser.NotBetweenStr:
		var ok bool
		values, ok = value.([]any)
		if !ok || len(values) == 0 {
			return nil, query.NewConverterError("%s: empty values for %s", query.InvalidExpressionErrMessage, col.Alias)
		}
		if (op == sqlparser.BetweenStr || op == sqlparser.NotBetweenStr) && len(values) != 2 {
			return nil, query.NewConverterError("%s: invalid range for %s", query.InvalidExpressionErrMessage, col.Alias)
		}
	default:
		values = []any{value}
	}
	for i, v := range values {
		var err error
		values[i], err = normalizeQueryValue(col, v)
		if err != nil {
			return nil, err
		}
	}
	return &expression{kind: atomExpr, column: col, op: op, values: values}, nil
}

func normalizeQueryValue(col *query.SAColumn, value any) (any, error) {
	name := col.FieldName
	if name == sadefs.ExecutionStatus {
		statusName, ok := value.(string)
		if !ok {
			return nil, query.NewConverterError("%s: invalid status", query.InvalidExpressionErrMessage)
		}
		status, err := enumspb.WorkflowExecutionStatusFromString(statusName)
		if err != nil {
			return nil, query.NewConverterError("%s: invalid status", query.InvalidExpressionErrMessage)
		}
		return int32(status), nil
	}
	if col.ValueType == enumspb.INDEXED_VALUE_TYPE_DATETIME {
		v, ok := value.(string)
		if !ok {
			return nil, query.NewConverterError("%s: invalid datetime", query.InvalidExpressionErrMessage)
		}
		tm, err := time.Parse(time.RFC3339Nano, v)
		if err != nil {
			return nil, query.NewConverterError("%s: invalid datetime", query.InvalidExpressionErrMessage)
		}
		if isCoreAttribute(name) {
			return tm.UTC(), nil
		}
		return tm.UnixMicro(), nil
	}
	if col.ValueType == enumspb.INDEXED_VALUE_TYPE_DOUBLE {
		switch v := value.(type) {
		case int64:
			return float64(v), nil
		}
	}
	if col.ValueType == enumspb.INDEXED_VALUE_TYPE_INT {
		if v, ok := value.(float64); ok {
			if math.Trunc(v) != v || v < -0x1p63 || v >= 0x1p63 {
				return nil, query.NewConverterError("%s: fractional or out-of-range Int value for %s", query.InvalidExpressionErrMessage, col.Alias)
			}
			return int64(v), nil
		}
	}
	return value, nil
}

func columnForAttribute(name string, typ enumspb.IndexedValueType) (string, bool) {
	switch name {
	case sadefs.NamespaceID:
		return colNamespaceID, true
	case sadefs.RunID:
		return colRunID, true
	case sadefs.WorkflowID:
		return colWorkflowID, true
	case sadefs.WorkflowType:
		return colWorkflowTypeName, true
	case sadefs.ExecutionStatus:
		return colStatus, true
	case sadefs.StartTime:
		return colStartTime, true
	case sadefs.CloseTime:
		return colCloseTime, true
	case sadefs.ExecutionTime:
		return colExecutionTime, true
	case sadefs.TemporalNamespaceDivision:
		return colNamespaceDivision, true
	case flowTypeSearchAttribute:
		if typ == enumspb.INDEXED_VALUE_TYPE_KEYWORD {
			return colFlowType, true
		}
		return "", false
	case sadefs.TaskQueue:
		return colTaskQueue, true
	case sadefs.ParentWorkflowID:
		return colParentWorkflowID, true
	case sadefs.ParentRunID:
		return colParentRunID, true
	case sadefs.RootWorkflowID:
		return colRootWorkflowID, true
	case sadefs.RootRunID:
		return colRootRunID, true
	case sadefs.HistoryLength:
		return colHistoryLength, true
	case sadefs.HistorySizeBytes:
		return colHistorySizeBytes, true
	case sadefs.ExecutionDuration:
		return colExecutionDuration, true
	case sadefs.StateTransitionCount:
		return colStateTransitionCount, true
	default:
		return searchAttributeColumn(typ)
	}
}

func isCoreAttribute(name string) bool {
	col, ok := columnForAttribute(name, enumspb.INDEXED_VALUE_TYPE_KEYWORD)
	return ok && col != colSAKeyword && col != colSAText && col != colSANumber && col != colSADatetime && col != colSABool
}

func convertQuery(
	queryString string,
	namespaceName namespace.Name,
	saTypeMap searchattribute.NameTypeMap,
	saMapper searchattribute.Mapper,
	chasmMapper *chasm.VisibilitySearchAttributesMapper,
	archetypeID chasm.ArchetypeID,
	metricsHandler metrics.Handler,
	logger log.Logger,
) (*convertedQuery, error) {
	c := query.NewQueryConverter(&astConverter{}, namespaceName, saTypeMap, saMapper, metricsHandler, logger).
		WithChasmMapper(chasmMapper).WithArchetypeID(archetypeID)
	params, err := c.Convert(queryString)
	if err != nil {
		return nil, invalidQueryError(err)
	}
	if len(params.OrderBy) != 0 {
		return nil, invalidQueryError(query.NewConverterError("%s: 'ORDER BY' clause", query.NotSupportedErrMessage))
	}
	out := &convertedQuery{predicate: params.QueryExpr}
	if len(params.GroupBy) > 0 {
		out.groupBy = params.GroupBy[0].FieldName
		if out.groupBy != sadefs.ExecutionStatus && out.groupBy != sadefs.TemporalNamespaceDivision {
			return nil, invalidQueryError(query.NewConverterError("%s: 'GROUP BY' %s", query.NotSupportedErrMessage, out.groupBy))
		}
	}
	return out, nil
}

func invalidQueryError(err error) error {
	var converterErr *query.ConverterError
	if errors.As(err, &converterErr) {
		return converterErr.ToInvalidArgument()
	}
	return err
}
