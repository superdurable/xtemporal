package paradedb

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/superdurable/xtemporal/common/searchattribute/sadefs"
	"github.com/temporalio/sqlparser"
	enumspb "go.temporal.io/api/enums/v1"
	"go.temporal.io/api/serviceerror"
)

type route string

const (
	routeBTree       route = "btree"
	routeBM25        route = "bm25"
	boundedScanLimit       = 1000
)

const (
	selectColumns = `namespace_id, run_id, workflow_id, workflow_type_name, status, start_time, close_time,
 sort_close_time, namespace_division, flow_type, task_queue, execution_time, parent_workflow_id,
 parent_run_id, root_workflow_id, root_run_id, history_length, history_size_bytes, execution_duration,
 state_transition_count, memo, encoding, _version, sa_keyword, sa_text, sa_number, sa_datetime, sa_bool`
	defaultOrder = `ORDER BY sort_close_time DESC, start_time DESC, run_id COLLATE "C"`
)

type listPlan struct {
	Route   route
	SQL     string
	Args    []any
	Bounded *boundedPlan
}

type boundedPlan struct {
	// SQL returns the normal list columns followed by scanned_count. A null run_id is
	// the single count-only row when no matches are found in the scan.
	SQL          string
	Args         []any
	Bound        int
	FallbackSQL  string
	FallbackArgs []any
}

type countPlan struct {
	Route   route
	SQL     string
	Args    []any
	GroupBy string
}

type pageCursor struct {
	Version       int       `json:"v"`
	SortCloseTime time.Time `json:"c"`
	StartTime     time.Time `json:"s"`
	RunID         string    `json:"r"`
}

func encodePageToken(sortCloseTime, startTime time.Time, runID string) ([]byte, error) {
	if sortCloseTime.IsZero() || startTime.IsZero() || runID == "" {
		return nil, serviceerror.NewInvalidArgument("invalid page cursor")
	}
	data, err := json.Marshal(pageCursor{Version: 1, SortCloseTime: sortCloseTime.UTC(), StartTime: startTime.UTC(), RunID: runID})
	if err != nil {
		return nil, fmt.Errorf("encode ParadeDB page token: %w", err)
	}
	return []byte(base64.RawURLEncoding.EncodeToString(data)), nil
}

func nextPageToken(rowCount, pageSize int, sortCloseTime, startTime time.Time, runID string) ([]byte, error) {
	if pageSize <= 0 || rowCount < 0 || rowCount > pageSize {
		return nil, serviceerror.NewInvalidArgument("invalid page size or row count")
	}
	if rowCount < pageSize {
		return nil, nil
	}
	return encodePageToken(sortCloseTime, startTime, runID)
}

func decodePageToken(token []byte) (*pageCursor, error) {
	if len(token) == 0 {
		return nil, nil
	}
	data, err := base64.RawURLEncoding.DecodeString(string(token))
	if err != nil {
		return nil, serviceerror.NewInvalidArgument("malformed page token")
	}
	var cursor pageCursor
	if err := json.Unmarshal(data, &cursor); err != nil || cursor.Version != 1 || cursor.SortCloseTime.IsZero() ||
		cursor.StartTime.IsZero() || cursor.RunID == "" {
		return nil, serviceerror.NewInvalidArgument("malformed page token")
	}
	return &cursor, nil
}

func classifyRoute(e *expression) route {
	if containsTextMatch(e) || (containsCustom(e) && !hasCorePrefix(e)) {
		return routeBM25
	}
	return routeBTree
}

func containsTextMatch(e *expression) bool {
	if e == nil {
		return false
	}
	if e.kind == atomExpr && e.column.ValueType == enumspb.INDEXED_VALUE_TYPE_TEXT &&
		(e.op == sqlparser.EqualStr || e.op == sqlparser.NotEqualStr) {
		return true
	}
	for _, child := range e.children {
		if containsTextMatch(child) {
			return true
		}
	}
	return false
}

func containsCustom(e *expression) bool {
	if e == nil {
		return false
	}
	if e.kind == atomExpr {
		return !isCoreAttribute(e.column.FieldName)
	}
	for _, child := range e.children {
		if containsCustom(child) {
			return true
		}
	}
	return false
}

func hasCorePrefix(e *expression) bool {
	if e == nil {
		return false
	}
	switch e.kind {
	case atomExpr:
		if e.op != sqlparser.EqualStr && e.op != sqlparser.InStr &&
			(e.column.FieldName != sadefs.TemporalNamespaceDivision || e.op != sqlparser.IsNotNullStr) {
			return false
		}
		switch e.column.FieldName {
		case flowTypeSearchAttribute, sadefs.WorkflowID, sadefs.ExecutionStatus:
			return true
		case sadefs.TemporalNamespaceDivision:
			return e.op == sqlparser.IsNotNullStr || len(e.values) > 0 && e.values[0] != nil
		default:
			return false
		}
	case andExpr:
		for _, child := range e.children {
			if hasCorePrefix(child) {
				return true
			}
		}
		return false
	case orExpr:
		if len(e.children) == 0 {
			return false
		}
		for _, child := range e.children {
			if !hasCorePrefix(child) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func renderList(q *convertedQuery, namespaceID string, pageSize int, token []byte) (*listPlan, error) {
	if q == nil || namespaceID == "" || pageSize <= 0 {
		return nil, serviceerror.NewInvalidArgument("invalid list query parameters")
	}
	if q.groupBy != "" {
		return nil, serviceerror.NewInvalidArgument("GROUP BY is only supported for Count")
	}
	cursor, err := decodePageToken(token)
	if err != nil {
		return nil, err
	}
	r := classifyRoute(q.predicate)
	plain, err := renderListRoute(q.predicate, namespaceID, pageSize, cursor, r)
	if err != nil {
		return nil, err
	}
	if r == routeBTree && containsCustom(q.predicate) && hasCorePrefix(q.predicate) && !containsTextMatch(q.predicate) && bm25Equivalent(q.predicate) {
		bound, err := renderBounded(q.predicate, namespaceID, pageSize, cursor)
		if err != nil {
			return nil, err
		}
		plain.Bounded = bound
	}
	return plain, nil
}

func bm25Equivalent(e *expression) bool {
	if e == nil {
		return true
	}
	if e.kind != atomExpr {
		for _, child := range e.children {
			if !bm25Equivalent(child) {
				return false
			}
		}
		return true
	}
	if isCoreAttribute(e.column.FieldName) {
		return true
	}
	switch e.column.ValueType {
	case enumspb.INDEXED_VALUE_TYPE_KEYWORD, enumspb.INDEXED_VALUE_TYPE_KEYWORD_LIST,
		enumspb.INDEXED_VALUE_TYPE_DOUBLE, enumspb.INDEXED_VALUE_TYPE_BOOL:
		return true
	case enumspb.INDEXED_VALUE_TYPE_INT, enumspb.INDEXED_VALUE_TYPE_DATETIME:
		switch e.op {
		case sqlparser.EqualStr, sqlparser.NotEqualStr, sqlparser.InStr, sqlparser.NotInStr,
			sqlparser.IsNullStr, sqlparser.IsNotNullStr:
			return true
		default:
			return false
		}
	default:
		return false
	}
}

func renderListRoute(e *expression, namespaceID string, pageSize int, cursor *pageCursor, r route) (*listPlan, error) {
	b := &sqlBuilder{args: []any{namespaceID}}
	where := `namespace_id = $1`
	if r == routeBM25 {
		where += ` AND namespace_id @@@ pdb.term($1::text)`
	}
	predicate, err := b.render(e, r)
	if err != nil {
		return nil, err
	}
	if predicate != "" {
		where += " AND (" + predicate + ")"
	}
	if cursor != nil {
		c, s, run := b.bind(cursor.SortCloseTime), b.bind(cursor.StartTime), b.bind(cursor.RunID)
		where += fmt.Sprintf(` AND sort_close_time <= %s AND (sort_close_time < %s OR (sort_close_time = %s AND start_time < %s) OR (sort_close_time = %s AND start_time = %s AND run_id COLLATE "C" > %s))`, c, c, c, s, c, s, run)
	}
	limit := b.bind(pageSize)
	return &listPlan{Route: r, SQL: fmt.Sprintf("SELECT %s FROM %s WHERE %s %s LIMIT %s", selectColumns, visibilityTable, where, defaultOrder, limit), Args: b.args}, nil
}

func renderCount(q *convertedQuery, namespaceID string) (*countPlan, error) {
	if q == nil || namespaceID == "" {
		return nil, serviceerror.NewInvalidArgument("invalid count query parameters")
	}
	r := classifyRoute(q.predicate)
	b := &sqlBuilder{args: []any{namespaceID}}
	where := `namespace_id = $1`
	if r == routeBM25 {
		where += ` AND namespace_id @@@ pdb.term($1::text)`
	}
	predicate, err := b.render(q.predicate, r)
	if err != nil {
		return nil, err
	}
	if predicate != "" {
		where += " AND (" + predicate + ")"
	}
	plan := &countPlan{Route: r, Args: b.args, GroupBy: q.groupBy}
	switch q.groupBy {
	case "":
		plan.SQL = fmt.Sprintf("SELECT count(*) FROM %s WHERE %s", visibilityTable, where)
	case sadefs.ExecutionStatus:
		plan.SQL = fmt.Sprintf("SELECT status, count(*) FROM %s WHERE %s GROUP BY status", visibilityTable, where)
	case sadefs.TemporalNamespaceDivision:
		plan.SQL = fmt.Sprintf("SELECT coalesce(namespace_division, '') AS namespace_division, count(*) FROM %s WHERE %s GROUP BY coalesce(namespace_division, '')", visibilityTable, where)
	default:
		return nil, serviceerror.NewInvalidArgument("unsupported GROUP BY")
	}
	return plan, nil
}

func renderBounded(e *expression, namespaceID string, pageSize int, cursor *pageCursor) (*boundedPlan, error) {
	if !bm25Equivalent(e) {
		return nil, nil
	}
	core := coreOnly(e)
	if core == nil || !hasCorePrefix(core) {
		return nil, nil
	}
	b := &sqlBuilder{args: []any{namespaceID}}
	coreSQL, err := b.render(core, routeBTree)
	if err != nil {
		return nil, err
	}
	where := `namespace_id = $1 AND (` + coreSQL + `)`
	if cursor != nil {
		c, s, run := b.bind(cursor.SortCloseTime), b.bind(cursor.StartTime), b.bind(cursor.RunID)
		where += fmt.Sprintf(` AND sort_close_time <= %s AND (sort_close_time < %s OR (sort_close_time = %s AND start_time < %s) OR (sort_close_time = %s AND start_time = %s AND run_id COLLATE "C" > %s))`, c, c, c, s, c, s, run)
	}
	boundArg := b.bind(boundedScanLimit)
	residual, err := b.render(e, routeBTree)
	if err != nil {
		return nil, err
	}
	fallback, err := renderListRoute(e, namespaceID, pageSize, cursor, routeBM25)
	if err != nil {
		return nil, err
	}
	pageArg := b.bind(pageSize)
	columns := scanColumns(e)
	fullColumns := qualifyColumns(selectColumns, "matched_row")
	return &boundedPlan{
		SQL: fmt.Sprintf(`WITH scan AS MATERIALIZED (SELECT %s FROM %s WHERE %s %s LIMIT %s),
matches AS MATERIALIZED (SELECT namespace_id, run_id, sort_close_time, start_time FROM scan WHERE coalesce((%s), false) %s LIMIT %s),
stats AS (SELECT count(*) AS scanned_count FROM scan)
SELECT %s, stats.scanned_count FROM stats LEFT JOIN matches ON true
LEFT JOIN %s AS matched_row ON matched_row.namespace_id = matches.namespace_id AND matched_row.run_id = matches.run_id
ORDER BY matches.sort_close_time DESC, matches.start_time DESC, matches.run_id COLLATE "C"`, strings.Join(columns, ", "), visibilityTable, where, defaultOrder, boundArg, residual, defaultOrder, pageArg, fullColumns, visibilityTable),
		Args: b.args, Bound: boundedScanLimit, FallbackSQL: fallback.SQL, FallbackArgs: fallback.Args,
	}, nil
}

func scanColumns(e *expression) []string {
	columns := []string{"namespace_id", "run_id", "sort_close_time", "start_time"}
	seen := map[string]bool{"namespace_id": true, "run_id": true, "sort_close_time": true, "start_time": true}
	var collect func(*expression)
	collect = func(e *expression) {
		if e == nil {
			return
		}
		if e.kind == atomExpr {
			col, ok := columnForAttribute(e.column.FieldName, e.column.ValueType)
			if ok && !seen[col] {
				columns = append(columns, col)
				seen[col] = true
			}
			return
		}
		for _, child := range e.children {
			collect(child)
		}
	}
	collect(e)
	return columns
}

func qualifyColumns(columns, alias string) string {
	parts := strings.Split(columns, ",")
	for i, part := range parts {
		parts[i] = alias + "." + strings.TrimSpace(part)
	}
	return strings.Join(parts, ", ")
}

func coreOnly(e *expression) *expression {
	if e == nil {
		return nil
	}
	switch e.kind {
	case atomExpr:
		if isCoreAttribute(e.column.FieldName) {
			return e
		}
		return nil
	case andExpr:
		children := make([]*expression, 0, len(e.children))
		for _, child := range e.children {
			children = append(children, coreOnly(child))
		}
		return joinExpr(andExpr, children...)
	case orExpr:
		children := make([]*expression, 0, len(e.children))
		for _, child := range e.children {
			core := coreOnly(child)
			if core == nil {
				return nil
			}
			children = append(children, core)
		}
		return joinExpr(orExpr, children...)
	case notExpr:
		if containsCustom(e) {
			return nil
		}
		return e
	}
	return nil
}

type sqlBuilder struct{ args []any }

func (b *sqlBuilder) bind(v any) string {
	b.args = append(b.args, v)
	return fmt.Sprintf("$%d", len(b.args))
}

func (b *sqlBuilder) render(e *expression, r route) (string, error) {
	if e == nil {
		return "", nil
	}
	switch e.kind {
	case andExpr, orExpr:
		parts := make([]string, 0, len(e.children))
		for _, child := range e.children {
			part, err := b.render(child, r)
			if err != nil {
				return "", err
			}
			if part != "" {
				parts = append(parts, "("+part+")")
			}
		}
		if e.kind == orExpr {
			return strings.Join(parts, " OR "), nil
		}
		return strings.Join(parts, " AND "), nil
	case notExpr:
		return b.renderNegated(e.children[0], r)
	case atomExpr:
		return b.renderAtom(e, r)
	default:
		return "", serviceerror.NewInvalidArgument("invalid query expression")
	}
}

func (b *sqlBuilder) renderNegated(e *expression, r route) (string, error) {
	if e == nil {
		return "FALSE", nil
	}
	switch e.kind {
	case notExpr:
		return b.render(e.children[0], r)
	case andExpr, orExpr:
		parts := make([]string, 0, len(e.children))
		for _, child := range e.children {
			part, err := b.renderNegated(child, r)
			if err != nil {
				return "", err
			}
			parts = append(parts, "("+part+")")
		}
		if e.kind == andExpr {
			return strings.Join(parts, " OR "), nil
		}
		return strings.Join(parts, " AND "), nil
	case atomExpr:
		positive, err := b.renderAtom(e, r)
		if err != nil {
			return "", err
		}
		if isCoreAttribute(e.column.FieldName) {
			return "coalesce(NOT (" + positive + "), true)", nil
		}
		return "NOT (" + positive + ")", nil
	default:
		return "", serviceerror.NewInvalidArgument("invalid query expression")
	}
}

func quoteAttributeName(name string) (string, error) {
	for i, r := range name {
		if r == '_' || r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || i > 0 && r >= '0' && r <= '9' {
			continue
		}
		return "", serviceerror.NewInvalidArgument("invalid search attribute name")
	}
	if name == "" {
		return "", serviceerror.NewInvalidArgument("invalid search attribute name")
	}
	return "'" + name + "'", nil
}

func (b *sqlBuilder) renderAtom(e *expression, r route) (string, error) {
	name := e.column.FieldName
	col, ok := columnForAttribute(name, e.column.ValueType)
	if !ok {
		return "", serviceerror.NewInvalidArgument("invalid search attribute type")
	}
	if isCoreAttribute(name) {
		return b.renderCoreAtom(col, e)
	}
	quoted, err := quoteAttributeName(name)
	if err != nil {
		return "", invalidQueryError(err)
	}
	path := col + "->" + quoted
	if e.op == sqlparser.IsNullStr {
		return "(" + path + " IS NULL OR " + path + " = '[]'::jsonb)", nil
	}
	if e.op == sqlparser.IsNotNullStr {
		return "(" + path + " IS NOT NULL AND " + path + " <> '[]'::jsonb)", nil
	}
	positiveOp, negate := positiveOperator(e.op)
	if e.column.ValueType == enumspb.INDEXED_VALUE_TYPE_TEXT {
		if positiveOp != sqlparser.EqualStr {
			return "", serviceerror.NewInvalidArgument("unsupported Text operator")
		}
		value := b.bind(e.values[0])
		match := path + " ||| " + value + "::text"
		if negate {
			return "NOT (" + match + ")", nil
		}
		return match, nil
	}
	if r == routeBM25 && e.column.ValueType != enumspb.INDEXED_VALUE_TYPE_DOUBLE {
		match, indexed := b.renderIndexedJSON(path, e.column.ValueType, positiveOp, e.values)
		if indexed {
			if negate {
				return "NOT (" + match + ")", nil
			}
			return match, nil
		}
	}

	match, err := b.renderJSONComparison(path, e.column.ValueType, positiveOp, e.values)
	if err != nil {
		return "", err
	}
	if negate {
		return "NOT (" + match + ")", nil
	}
	return match, nil
}

func (b *sqlBuilder) renderIndexedJSON(path string, typ enumspb.IndexedValueType, op string, values []any) (string, bool) {
	cast := "text"
	switch typ {
	case enumspb.INDEXED_VALUE_TYPE_INT, enumspb.INDEXED_VALUE_TYPE_DATETIME:
		cast = "bigint"
	case enumspb.INDEXED_VALUE_TYPE_DOUBLE:
		cast = "double precision"
	case enumspb.INDEXED_VALUE_TYPE_BOOL:
		cast = "boolean"
	case enumspb.INDEXED_VALUE_TYPE_KEYWORD, enumspb.INDEXED_VALUE_TYPE_KEYWORD_LIST:
	default:
		return "", false
	}
	if op == sqlparser.EqualStr || op == sqlparser.InStr {
		terms := make([]string, 0, len(values))
		for _, v := range values {
			terms = append(terms, path+" @@@ pdb.term("+b.bind(v)+"::"+cast+")")
		}
		return "(" + strings.Join(terms, " OR ") + ")", true
	}
	if cast != "bigint" {
		return "", false
	}
	return b.renderIndexedIntRange(path, op, values)
}

func (b *sqlBuilder) renderIndexedIntRange(path, op string, values []any) (string, bool) {
	if v, ok := values[0].(int64); ok {
		switch {
		case op == sqlparser.GreaterThanStr && v == math.MaxInt64,
			op == sqlparser.LessThanStr && v == math.MinInt64:
			return "FALSE", true
		case op == sqlparser.GreaterEqualStr && v == math.MinInt64,
			op == sqlparser.LessEqualStr && v == math.MaxInt64:
			return "(" + path + " IS NOT NULL AND " + path + " <> '[]'::jsonb)", true
		}
	}
	var bounds string
	switch op {
	case sqlparser.GreaterThanStr:
		bounds = fmt.Sprintf("int8range(%s::bigint, NULL, '()')", b.bind(values[0]))
	case sqlparser.GreaterEqualStr:
		bounds = fmt.Sprintf("int8range(%s::bigint, NULL, '[)')", b.bind(values[0]))
	case sqlparser.LessThanStr:
		bounds = fmt.Sprintf("int8range(NULL, %s::bigint, '()')", b.bind(values[0]))
	case sqlparser.LessEqualStr:
		bounds = fmt.Sprintf("int8range(NULL, %s::bigint, '(]')", b.bind(values[0]))
	case sqlparser.BetweenStr:
		low, lowOK := values[0].(int64)
		high, highOK := values[1].(int64)
		if lowOK && highOK {
			if low > high {
				return "FALSE", true
			}
			if high == math.MaxInt64 {
				bounds = fmt.Sprintf("int8range(%s::bigint, NULL, '[)')", b.bind(low))
				break
			}
		}
		bounds = fmt.Sprintf("int8range(%s::bigint, %s::bigint, '[]')", b.bind(values[0]), b.bind(values[1]))
	default:
		return "", false
	}
	return path + " @@@ pdb.range(" + bounds + ")", true
}

func positiveOperator(op string) (string, bool) {
	switch op {
	case sqlparser.NotEqualStr:
		return sqlparser.EqualStr, true
	case sqlparser.NotInStr:
		return sqlparser.InStr, true
	case sqlparser.NotBetweenStr:
		return sqlparser.BetweenStr, true
	default:
		return op, false
	}
}

func (b *sqlBuilder) renderCoreAtom(col string, e *expression) (string, error) {
	if e.op == sqlparser.IsNullStr {
		return col + " IS NULL", nil
	}
	if e.op == sqlparser.IsNotNullStr {
		return col + " IS NOT NULL", nil
	}
	op, negate := positiveOperator(e.op)
	match, err := b.renderScalarComparison(col, op, e.values, e.column.ValueType == enumspb.INDEXED_VALUE_TYPE_KEYWORD)
	if err != nil {
		return "", err
	}
	if negate {
		return "coalesce(NOT (" + match + "), true)", nil
	}
	return match, nil
}

func (b *sqlBuilder) renderScalarComparison(col, op string, values []any, textRange bool) (string, error) {
	if textRange && (op == sqlparser.LessThanStr || op == sqlparser.LessEqualStr ||
		op == sqlparser.GreaterThanStr || op == sqlparser.GreaterEqualStr || op == sqlparser.BetweenStr) {
		col += ` COLLATE "C"`
	}
	switch op {
	case sqlparser.EqualStr, sqlparser.LessThanStr, sqlparser.LessEqualStr, sqlparser.GreaterThanStr, sqlparser.GreaterEqualStr:
		return col + " " + op + " " + b.bind(values[0]), nil
	case sqlparser.InStr:
		parts := make([]string, 0, len(values))
		for _, v := range values {
			parts = append(parts, col+" = "+b.bind(v))
		}
		return "(" + strings.Join(parts, " OR ") + ")", nil
	case sqlparser.BetweenStr:
		return "(" + col + " >= " + b.bind(values[0]) + " AND " + col + " <= " + b.bind(values[1]) + ")", nil
	default:
		return "", serviceerror.NewInvalidArgument("unsupported comparison operator")
	}
}

func (b *sqlBuilder) renderJSONComparison(path string, typ enumspb.IndexedValueType, op string, values []any) (string, error) {
	cast := "text"
	switch typ {
	case enumspb.INDEXED_VALUE_TYPE_INT, enumspb.INDEXED_VALUE_TYPE_DATETIME:
		cast = "bigint"
	case enumspb.INDEXED_VALUE_TYPE_DOUBLE:
		cast = "double precision"
	case enumspb.INDEXED_VALUE_TYPE_BOOL:
		cast = "boolean"
	default:
		cast = "text"
	}
	// CASE keeps scalar casts away from arrays and array expansion away from scalars.
	var scalar string
	if cast == "text" {
		scalar = fmt.Sprintf("(CASE WHEN jsonb_typeof(%s) = 'string' THEN %s #>> '{}' ELSE NULL END)", path, path)
	}
	array := fmt.Sprintf("jsonb_array_elements_text(CASE WHEN jsonb_typeof(%s) = 'array' THEN %s ELSE '[]'::jsonb END)", path, path)
	// Numeric and boolean JSON scalars have no array index; #>> '{}' gives their text value.
	if cast != "text" {
		scalar = fmt.Sprintf("(CASE WHEN jsonb_typeof(%s) = '%s' THEN (%s #>> '{}')::%s ELSE NULL END)", path, jsonType(typ), path, cast)
	}
	matchScalar, err := b.renderScalarComparison(scalar, op, values, cast == "text")
	if err != nil {
		return "", err
	}
	matchArray, err := b.renderScalarComparison("element.value::"+cast, op, values, cast == "text")
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("(coalesce((%s), false) OR EXISTS (SELECT 1 FROM %s AS element(value) WHERE %s))", matchScalar, array, matchArray), nil
}

func jsonType(typ enumspb.IndexedValueType) string {
	switch typ {
	case enumspb.INDEXED_VALUE_TYPE_INT, enumspb.INDEXED_VALUE_TYPE_DOUBLE, enumspb.INDEXED_VALUE_TYPE_DATETIME:
		return "number"
	case enumspb.INDEXED_VALUE_TYPE_BOOL:
		return "boolean"
	default:
		return "string"
	}
}
