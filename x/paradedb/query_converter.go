package paradedb

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/superdurable/xtemporal/common/persistence/sql"
	"github.com/superdurable/xtemporal/common/persistence/sql/sqlplugin"
	"github.com/superdurable/xtemporal/common/persistence/sql/sqlplugin/postgresql"
	"github.com/superdurable/xtemporal/common/persistence/visibility/store/query"
	"github.com/superdurable/xtemporal/common/searchattribute/sadefs"
	"github.com/temporalio/sqlparser"
)

type queryConverter struct{}

var _ sqlplugin.VisibilityQueryConverter = (*queryConverter)(nil)

type sqlExpr struct {
	sqlparser.Expr
	text string
}

func (e *sqlExpr) Format(buf *sqlparser.TrackedBuffer) { buf.Myprintf("%s", e.text) }

func (*queryConverter) GetDatetimeFormat() string { return "2006-01-02 15:04:05.999999" }

func (*queryConverter) ConvertComparisonExpr(operator string, col *query.SAColumn, value sqlparser.Expr) (sqlparser.Expr, error) {
	if col.FieldName == sadefs.NamespaceID && operator == sqlparser.EqualStr {
		return &sqlExpr{text: fmt.Sprintf("%s @@@ %s", sqlparser.String(col), sqlparser.String(value))}, nil
	}
	return &sqlparser.ComparisonExpr{Operator: operator, Left: col, Right: value}, nil
}

func (*queryConverter) ConvertKeywordComparisonExpr(operator string, col *query.SAColumn, value sqlparser.Expr) (sqlparser.Expr, error) {
	if operator != sqlparser.StartsWithStr && operator != sqlparser.NotStartsWithStr {
		return &sqlparser.ComparisonExpr{Operator: operator, Left: col, Right: value}, nil
	}
	v, ok := value.(*query.UnsafeSQLString)
	if !ok {
		return nil, query.NewConverterError("%s: STARTS_WITH requires a string", query.InvalidExpressionErrMessage)
	}
	if !indexedKeywordColumn(col.FieldName) {
		escaped := strings.NewReplacer("!", "!!", "%", "!%", "_", "!_").Replace(v.Val) + "%"
		predicate := &sqlExpr{text: fmt.Sprintf("%s LIKE '%s' ESCAPE '!'", sqlparser.String(col), escaped)}
		if operator == sqlparser.NotStartsWithStr {
			return &sqlparser.NotExpr{Expr: &sqlparser.ParenExpr{Expr: predicate}}, nil
		}
		return predicate, nil
	}
	pattern := regexp.QuoteMeta(v.Val) + ".*"
	match := &sqlExpr{text: fmt.Sprintf("%s @@@ pdb.regex('%s')", sqlparser.String(col), pattern)}
	if operator == sqlparser.NotStartsWithStr {
		return &sqlparser.NotExpr{Expr: &sqlparser.ParenExpr{Expr: match}}, nil
	}
	return match, nil
}

func (*queryConverter) ConvertKeywordListComparisonExpr(operator string, col *query.SAColumn, value sqlparser.Expr) (sqlparser.Expr, error) {
	if col.FieldName != "KeywordList01" && col.FieldName != "KeywordList02" && col.FieldName != "KeywordList03" {
		pgConverter, err := sql.GetPluginVisibilityQueryConverter(postgresql.PluginNamePGX)
		if err != nil {
			return nil, err
		}
		return pgConverter.ConvertKeywordListComparisonExpr(operator, col, value)
	}
	term := func(v sqlparser.Expr) sqlparser.Expr {
		return &sqlExpr{text: fmt.Sprintf("%s @@@ pdb.term(%s)", sqlparser.String(col), sqlparser.String(v))}
	}
	var result sqlparser.Expr
	switch operator {
	case sqlparser.EqualStr, sqlparser.NotEqualStr:
		if _, ok := value.(*query.UnsafeSQLString); !ok {
			return nil, query.NewConverterError("%s: KeywordList comparison requires a string", query.InvalidExpressionErrMessage)
		}
		result = term(value)
	case sqlparser.InStr, sqlparser.NotInStr:
		values, ok := value.(sqlparser.ValTuple)
		if !ok || len(values) == 0 {
			return nil, query.NewConverterError("%s: KeywordList IN requires a nonempty string tuple", query.InvalidExpressionErrMessage)
		}
		for _, v := range values {
			if _, ok := v.(*query.UnsafeSQLString); !ok {
				return nil, query.NewConverterError("%s: KeywordList IN requires strings", query.InvalidExpressionErrMessage)
			}
			if result == nil {
				result = term(v)
			} else {
				result = &sqlparser.OrExpr{Left: result, Right: term(v)}
			}
		}
		result = &sqlparser.ParenExpr{Expr: result}
	default:
		return nil, query.NewConverterError("%s: unsupported KeywordList operator %s", query.InvalidExpressionErrMessage, operator)
	}
	if operator == sqlparser.NotEqualStr || operator == sqlparser.NotInStr {
		return &sqlparser.NotExpr{Expr: &sqlparser.ParenExpr{Expr: result}}, nil
	}
	return result, nil
}

func indexedKeywordColumn(name string) bool {
	switch name {
	case sadefs.NamespaceID, sadefs.RunID, sadefs.WorkflowID, sadefs.WorkflowType, sadefs.TaskQueue,
		"Keyword01", "Keyword02", "Keyword03", "Keyword04", "Keyword05", "Keyword06", "Keyword07", "Keyword08", "Keyword09", "Keyword10":
		return true
	default:
		return false
	}
}

func (*queryConverter) ConvertTextComparisonExpr(operator string, col *query.SAColumn, value sqlparser.Expr) (sqlparser.Expr, error) {
	v, ok := value.(*query.UnsafeSQLString)
	if !ok || len(query.TokenizeTextQueryString(v.Val)) == 0 {
		return nil, query.NewConverterError("%s: Text comparison requires a nonempty string", query.InvalidExpressionErrMessage)
	}
	match := &sqlExpr{text: fmt.Sprintf("%s ||| %s", sqlparser.String(col), sqlparser.String(value))}
	if operator == sqlparser.NotEqualStr {
		return &sqlparser.NotExpr{Expr: &sqlparser.ParenExpr{Expr: match}}, nil
	}
	if operator != sqlparser.EqualStr {
		return nil, query.NewConverterError("%s: unsupported Text operator %s", query.InvalidExpressionErrMessage, operator)
	}
	return match, nil
}

func (*queryConverter) BuildSelectStmt(params *query.QueryParams[sqlparser.Expr], pageSize int, token *sqlplugin.VisibilityPageToken) (string, []any) {
	where := whereClause(params)
	var args []any
	if token != nil {
		predicate := `((sort_close_time = ? AND start_time = ? AND run_id COLLATE "C" > ?) OR (sort_close_time = ? AND start_time < ?) OR sort_close_time < ?)`
		if where == "" {
			where = " WHERE " + predicate
		} else {
			where += " AND " + predicate
		}
		args = append(args, token.CloseTime, token.StartTime, token.RunID, token.CloseTime, token.StartTime, token.CloseTime)
	}
	args = append(args, pageSize)
	return fmt.Sprintf(`SELECT %s FROM executions_visibility%s ORDER BY sort_close_time DESC, start_time DESC, run_id COLLATE "C" LIMIT ?`, strings.Join(sqlplugin.DbFields, ", "), where), args
}

func (*queryConverter) BuildCountStmt(params *query.QueryParams[sqlparser.Expr]) (string, []any) {
	groupBy := make([]string, 0, len(params.GroupBy))
	for _, field := range params.GroupBy {
		groupBy = append(groupBy, sadefs.GetSqlDbColName(field.FieldName))
	}
	selection := append(append([]string{}, groupBy...), "COUNT(*)")
	stmt := fmt.Sprintf("SELECT %s FROM executions_visibility%s", strings.Join(selection, ", "), whereClause(params))
	if len(groupBy) != 0 {
		stmt += " GROUP BY " + strings.Join(groupBy, ", ")
	}
	return stmt, nil
}

func whereClause(params *query.QueryParams[sqlparser.Expr]) string {
	if params == nil || params.QueryExpr == nil {
		return ""
	}
	if expr := sqlparser.String(params.QueryExpr); expr != "" {
		return " WHERE " + expr
	}
	return ""
}
