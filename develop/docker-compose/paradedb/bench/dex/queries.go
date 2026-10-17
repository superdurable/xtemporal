package main

import (
	"fmt"
	"strings"
	"time"
)

// The SQL below emulates what the json-hybrid store's converter generates for the Dex profile
// (dex-profile-benchmark.md, "SQL shapes to generate"). Every user query is scoped by
// namespace_id = $1 AND namespace_division IS NULL, and Dex adds the Engine/ContinuedAsNew filters.

const selectList = `namespace_id, run_id, workflow_id, workflow_type_name, status, start_time, close_time,
 sort_close_time, namespace_division, flow_type, task_queue, execution_time, parent_workflow_id,
 parent_run_id, root_workflow_id, root_run_id, history_length, history_size_bytes, execution_duration,
 state_transition_count, memo, encoding, _version, sa_keyword, sa_text, sa_number, sa_datetime, sa_bool`

const (
	userScope   = `namespace_id = $1 AND namespace_division IS NULL`
	bm25Scope   = `namespace_id = $1 AND namespace_id @@@ pdb.term($1::text) AND namespace_division IS NULL`
	dexFilter   = `workflow_type_name = 'Engine' AND status <> 6`
	defaultSort = `ORDER BY sort_close_time DESC, start_time DESC, run_id COLLATE "C"`
)

type route string

const (
	routeBTree route = "btree"
	routeBM25  route = "bm25"
)

// Shape is one query shape of the read mix.
type Shape struct {
	Name     string
	Route    route
	Intended string // index or scan the plan is expected to use
	SQL      string
}

func listSQL(scope, predicate string, limit int) string {
	where := scope
	if predicate != "" {
		where += " AND (" + predicate + ")"
	}
	return fmt.Sprintf("SELECT %s FROM executions_visibility WHERE %s AND %s %s LIMIT %d", selectList, where, dexFilter, defaultSort, limit)
}

// Per-FlowType shapes polled by the console; $2 is always the FlowType.
var flowShapes = []Shape{
	{"main_list", routeBTree, "by_flow_type", listSQL(userScope, `flow_type = $2`, 50)},
	{"need_person", routeBTree, "by_flow_type[_status] + residual", listSQL(userScope, `flow_type = $2 AND status = 1 AND sa_keyword->'DexWorkQueuePermissions' ?| $3::text[]`, 50)},
	{"done_today", routeBTree, "by_flow_type + residual", listSQL(userScope, `flow_type = $2 AND close_time >= $3`, 50)},
	{"running", routeBTree, "by_flow_type[_status]", listSQL(userScope, `flow_type = $2 AND status = 1`, 50)},
	{"custom_eq", routeBTree, "by_flow_type + residual", listSQL(userScope, `flow_type = $2 AND sa_keyword->>'keyword1' = $3`, 50)},
	{"custom_range", routeBTree, "by_flow_type + residual", listSQL(userScope, `flow_type = $2 AND (sa_number->>'int1')::bigint >= $3 AND (sa_number->>'int1')::bigint < $4`, 50)},
}

var (
	workflowIDShape = Shape{"workflow_id", routeBTree, "by_workflow_id", listSQL(userScope, `workflow_id = $2`, 50)}
	// The Dex server's readiness probe sends an empty SearchFlows query; only Dex Web adds the Engine filter.
	readyzShape = Shape{"readyz", routeBTree, "default_idx", fmt.Sprintf(
		"SELECT %s FROM executions_visibility WHERE %s AND status <> 6 %s LIMIT 1", selectList, userScope, defaultSort)}
	textShape       = Shape{"text_bm25", routeBM25, "ParadeDB scan", listSQL(bm25Scope, `flow_type = $2 AND sa_text->'text1' ||| $3::text`, 50)}
	customOnlyShape = Shape{"custom_only_bm25", routeBM25, "ParadeDB scan", listSQL(bm25Scope, `sa_keyword->'keyword3' @@@ pdb.term($2::text)`, 50)}
	scheduleShape   = Shape{"schedules", routeBTree, "by_namespace_division", fmt.Sprintf(
		"SELECT %s FROM executions_visibility WHERE namespace_id = $1 AND namespace_division = 'TemporalScheduler' %s LIMIT 50", selectList, defaultSort)}
)

// AllShapes lists every shape in report order.
func AllShapes() []Shape {
	out := append([]Shape{}, flowShapes...)
	return append(out, workflowIDShape, readyzShape, textShape, customOnlyShape, scheduleShape)
}

// Worst case: FlowType = X matches most of a huge namespace and the residual value is rare or absent.
const worstPredicate = `flow_type = $2 AND sa_keyword->>'keyword3' = $3`

var (
	worstBTree = Shape{"btree_residual", routeBTree, "by_flow_type + residual", listSQL(userScope, worstPredicate, 50)}
	worstBM25  = Shape{"bm25", routeBM25, "ParadeDB scan", listSQL(bm25Scope,
		`flow_type = $2 AND sa_keyword->'keyword3' @@@ pdb.term($3::text)`, 50)}
	// The promoted column is emulated by an expression index on the same JSON path: a B-tree over
	// (namespace_id, keyword3, sort) is what a promoted typed column would carry.
	worstPromoted = Shape{"promoted_column", routeBTree, "wc_promoted_keyword3", listSQL(userScope,
		`(sa_keyword->>'keyword3') = $3 AND flow_type = $2`, 50)}
	promotedIndexDDL = `CREATE INDEX wc_promoted_keyword3 ON executions_visibility
    (namespace_id, (sa_keyword->>'keyword3'), sort_close_time DESC, start_time DESC, run_id COLLATE "C")
    WHERE namespace_division IS NULL`
)

// boundedSQL scans at most $4 rows of the core prefix in order, then applies the residual. The result
// is complete when it holds 50 rows or the scan was exhausted (scanned < bound); otherwise the caller
// falls back to BM25.
var boundedSQL = fmt.Sprintf(`WITH scan AS MATERIALIZED (
  SELECT %s FROM executions_visibility WHERE %s AND (flow_type = $2) AND %s %s LIMIT $4)
SELECT c.scanned, s.run_id FROM (SELECT count(*) AS scanned FROM scan) c
LEFT JOIN LATERAL (SELECT run_id FROM scan WHERE sa_keyword->>'keyword3' = $3 %s LIMIT 50) s ON true`,
	selectList, userScope, dexFilter, defaultSort, defaultSort)

// keysetSQL pages the paging-test predicate after a cursor ($4..$6). The redundant
// sort_close_time <= $4 bound turns the leading sort key into an index condition, so a deep page does
// not re-filter every row before the cursor.
var keysetSQL = fmt.Sprintf(`SELECT run_id, sort_close_time, start_time, status FROM executions_visibility
WHERE %s AND (flow_type = $2 AND sa_keyword->>'keyword1' = $3) AND %s
  AND sort_close_time <= $4
  AND ((sort_close_time < $4) OR (sort_close_time = $4 AND start_time < $5)
    OR (sort_close_time = $4 AND start_time = $5 AND run_id COLLATE "C" > $6))
%s LIMIT 50`, userScope, dexFilter, defaultSort)

var keysetFirstSQL = fmt.Sprintf(`SELECT run_id, sort_close_time, start_time, status FROM executions_visibility
WHERE %s AND (flow_type = $2 AND sa_keyword->>'keyword1' = $3) AND %s %s LIMIT 50`, userScope, dexFilter, defaultSort)

var snapshotSQL = fmt.Sprintf(`SELECT run_id, sort_close_time, start_time, status FROM executions_visibility
WHERE %s AND (flow_type = $2 AND sa_keyword->>'keyword1' = $3) AND %s`, userScope, dexFilter)

const c3IndexDDL = `CREATE INDEX by_flow_type_status ON executions_visibility
    (namespace_id, flow_type, status, sort_close_time DESC, start_time DESC, run_id COLLATE "C")
    WHERE namespace_division IS NULL`

// upsertSQL matches the version-guarded SQL visibility write.
var upsertSQL = func() string {
	params := make([]string, len(writeColumns))
	sets := make([]string, len(writeColumns))
	for i, c := range writeColumns {
		params[i] = fmt.Sprintf("$%d::%s", i+1, writeColumnTypes[i])
		sets[i] = c + " = EXCLUDED." + c
	}
	return fmt.Sprintf(`INSERT INTO executions_visibility (%s) VALUES (%s)
ON CONFLICT (namespace_id, run_id) DO UPDATE SET %s WHERE executions_visibility._version < EXCLUDED._version`,
		strings.Join(writeColumns, ", "), strings.Join(params, ", "), strings.Join(sets, ", "))
}()

const deleteSQL = `DELETE FROM executions_visibility WHERE namespace_id = $1 AND run_id = $2`

func today() time.Time { return time.Now().UTC().Truncate(24 * time.Hour) }
