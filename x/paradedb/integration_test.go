package paradedb

import (
	"context"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
	"github.com/superdurable/xtemporal/chasm"
	"github.com/superdurable/xtemporal/common/log"
	"github.com/superdurable/xtemporal/common/namespace"
	"github.com/superdurable/xtemporal/common/searchattribute"
	enumspb "go.temporal.io/api/enums/v1"
)

func TestParadeDBPlans(t *testing.T) {
	ctx := context.Background()
	port, dbName := paradeDBTestDatabase(t)
	conn, err := pgx.Connect(ctx, paradeDBTestDSN(port, dbName))
	require.NoError(t, err)
	defer func() { require.NoError(t, conn.Close(ctx)) }()
	_, err = conn.Exec(ctx, "SET plan_cache_mode = force_custom_plan")
	require.NoError(t, err)
	seedParadeDB(ctx, t, conn)

	for _, tc := range []struct {
		name, filter string
		want         []string
		route        route
	}{
		{"default", "", []string{"r1", "r2", "r3", "r5"}, routeBTree},
		{"flow", `FlowType = 'approval'`, []string{"r1", "r2", "r5"}, routeBTree},
		{"status", `ExecutionStatus = 'Running'`, []string{"r1", "r3", "r5"}, routeBTree},
		{"workflow", `WorkflowId = 'wf-2'`, []string{"r2"}, routeBTree},
		{"division", `TemporalNamespaceDivision = 'TemporalScheduler'`, []string{"r4"}, routeBTree},
		{"keyword", `KeywordField = 'red'`, []string{"r1", "r3", "r5"}, routeBM25},
		{"keyword negative missing", `KeywordField != 'red'`, []string{"r2"}, routeBM25},
		{"keyword list", `Tags = 'x'`, []string{"r1", "r5"}, routeBM25},
		{"keyword list negative", `Tags != 'x'`, []string{"r2", "r3"}, routeBM25},
		{"keyword in", `KeywordField IN ('red','blue')`, []string{"r1", "r2", "r3", "r5"}, routeBM25},
		{"keyword not in", `KeywordField NOT IN ('red')`, []string{"r2"}, routeBM25},
		{"keyword range", `KeywordField >= 'red'`, []string{"r1", "r3", "r5"}, routeBM25},
		{"keyword between", `KeywordField BETWEEN 'blue' AND 'red'`, []string{"r1", "r2", "r3", "r5"}, routeBM25},
		{"int array", `IntField >= 5`, []string{"r2", "r5"}, routeBM25},
		{"int between array", `IntField BETWEEN 3 AND 5`, []string{"r1", "r2"}, routeBM25},
		{"int not between array", `IntField NOT BETWEEN 3 AND 5`, []string{"r3", "r5"}, routeBM25},
		{"double", `FloatField > 2.0`, []string{"r2"}, routeBM25},
		{"boolean", `Flag = true`, []string{"r1", "r3"}, routeBM25},
		{"boolean negative", `Flag != true`, []string{"r2", "r5"}, routeBM25},
		{"datetime", `DateField >= '2026-01-02T00:00:00Z'`, []string{"r2"}, routeBM25},
		{"is null", `Tags IS NULL`, []string{"r3"}, routeBM25},
		{"is not null", `Tags IS NOT NULL`, []string{"r1", "r2", "r5"}, routeBM25},
		{"nested", `FlowType = 'approval' AND (KeywordField = 'red' OR Tags = 'y')`, []string{"r1", "r2", "r5"}, routeBTree},
		{"int residual array", `FlowType = 'approval' AND IntField >= 5`, []string{"r2", "r5"}, routeBTree},
		{"double residual", `FlowType = 'approval' AND FloatField > 2.0`, []string{"r2"}, routeBTree},
		{"boolean residual", `FlowType = 'approval' AND Flag = true`, []string{"r1"}, routeBTree},
		{"datetime residual", `FlowType = 'approval' AND DateField >= '2026-01-02T00:00:00Z'`, []string{"r2"}, routeBTree},
		{"text", `Words = 'hello'`, []string{"r1", "r3"}, routeBM25},
	} {
		t.Run(tc.name, func(t *testing.T) {
			q, convertErr := testConvert(t, tc.filter, chasm.UnspecifiedArchetypeID)
			require.NoError(t, convertErr)
			plan, renderErr := renderList(q, "ns-a", 20, nil)
			require.NoError(t, renderErr)
			require.Equal(t, tc.route, plan.Route)
			got := queryRunIDs(ctx, t, conn, plan.SQL, plan.Args)
			sort.Strings(got)
			sort.Strings(tc.want)
			require.Equal(t, tc.want, got)
			count, countErr := renderCount(q, "ns-a")
			require.NoError(t, countErr)
			var n int64
			require.NoError(t, conn.QueryRow(ctx, count.SQL, count.Args...).Scan(&n))
			require.Equal(t, int64(len(tc.want)), n)
		})
	}

	t.Run("keyset", func(t *testing.T) {
		q, convertErr := testConvert(t, `FlowType = 'approval'`, chasm.UnspecifiedArchetypeID)
		require.NoError(t, convertErr)
		first, renderErr := renderList(q, "ns-a", 1, nil)
		require.NoError(t, renderErr)
		var runID string
		var closeTime, startTime time.Time
		require.NoError(t, conn.QueryRow(ctx, "SELECT run_id, sort_close_time, start_time FROM ("+first.SQL+") p", first.Args...).Scan(&runID, &closeTime, &startTime))
		token, tokenErr := encodePageToken(closeTime, startTime, runID)
		require.NoError(t, tokenErr)
		second, renderErr := renderList(q, "ns-a", 1, token)
		require.NoError(t, renderErr)
		got := queryRunIDs(ctx, t, conn, second.SQL, second.Args)
		require.Len(t, got, 1)
		require.NotEqual(t, runID, got[0])
	})

	t.Run("bounded and fallback", func(t *testing.T) {
		q, convertErr := testConvert(t, `FlowType = 'approval' AND KeywordField = 'red'`, chasm.UnspecifiedArchetypeID)
		require.NoError(t, convertErr)
		plan, renderErr := renderList(q, "ns-a", 2, nil)
		require.NoError(t, renderErr)
		require.NotNil(t, plan.Bounded)
		rows, queryErr := conn.Query(ctx, plan.Bounded.SQL, plan.Bounded.Args...)
		require.NoError(t, queryErr)
		var matches []string
		var scanned int
		for rows.Next() {
			values, valuesErr := rows.Values()
			require.NoError(t, valuesErr)
			scanned++
			if values[len(values)-1].(bool) {
				matches = append(matches, values[1].(string))
			}
		}
		require.NoError(t, rows.Err())
		rows.Close()
		require.Equal(t, 3, scanned)
		sort.Strings(matches)
		require.Equal(t, []string{"r1", "r5"}, matches)
		fallback := queryRunIDs(ctx, t, conn, plan.Bounded.FallbackSQL, plan.Bounded.FallbackArgs)
		sort.Strings(fallback)
		require.Equal(t, matches, fallback)
	})

	t.Run("group by", func(t *testing.T) {
		q, convertErr := testConvert(t, `GROUP BY TemporalNamespaceDivision`, chasm.UnspecifiedArchetypeID)
		require.NoError(t, convertErr)
		plan, renderErr := renderCount(q, "ns-a")
		require.NoError(t, renderErr)
		rows, queryErr := conn.Query(ctx, plan.SQL, plan.Args...)
		require.NoError(t, queryErr)
		groups := map[string]int64{}
		for rows.Next() {
			var name string
			var n int64
			require.NoError(t, rows.Scan(&name, &n))
			groups[name] = n
		}
		require.NoError(t, rows.Err())
		rows.Close()
		require.Equal(t, map[string]int64{"": 4, "TemporalScheduler": 1}, groups)

		statusQuery, convertErr := testConvert(t, `GROUP BY ExecutionStatus`, chasm.UnspecifiedArchetypeID)
		require.NoError(t, convertErr)
		statusPlan, renderErr := renderCount(statusQuery, "ns-a")
		require.NoError(t, renderErr)
		statusRows, queryErr := conn.Query(ctx, statusPlan.SQL, statusPlan.Args...)
		require.NoError(t, queryErr)
		statuses := map[int32]int64{}
		for statusRows.Next() {
			var status int32
			var n int64
			require.NoError(t, statusRows.Scan(&status, &n))
			statuses[status] = n
		}
		require.NoError(t, statusRows.Err())
		statusRows.Close()
		require.Equal(t, map[int32]int64{1: 3, 2: 1}, statuses)
	})

	t.Run("missing key negation", func(t *testing.T) {
		_, insertErr := conn.Exec(ctx, `INSERT INTO executions_visibility
            (namespace_id, run_id, workflow_id, workflow_type_name, status, start_time, execution_time, close_time, flow_type)
            VALUES ('ns-a','missing','missing','Engine',1,'2026-01-01','2026-01-01','2026-01-01','other')`)
		require.NoError(t, insertErr)
		for _, filter := range []string{`KeywordField != 'red'`, `Tags != 'x'`, `NOT (KeywordField = 'red')`, `Words != 'hello'`} {
			q, convertErr := testConvert(t, filter, chasm.UnspecifiedArchetypeID)
			require.NoError(t, convertErr)
			plan, renderErr := renderList(q, "ns-a", 20, nil)
			require.NoError(t, renderErr)
			require.Contains(t, queryRunIDs(ctx, t, conn, plan.SQL, plan.Args), "missing")
		}
	})

	t.Run("chasm mapper", func(t *testing.T) {
		mapper := chasm.NewTestVisibilitySearchAttributesMapper(
			map[string]string{"ChasmField": "ChasmAlias"},
			map[string]enumspb.IndexedValueType{"ChasmField": enumspb.INDEXED_VALUE_TYPE_KEYWORD},
		)
		q, convertErr := convertQuery(`ChasmAlias = 'value'`, namespace.Name("test"),
			searchattribute.NewNameTypeMap(nil), &searchattribute.TestMapper{}, mapper,
			chasm.SchedulerArchetypeID, nil, log.NewNoopLogger())
		require.NoError(t, convertErr)
		_, insertErr := conn.Exec(ctx, `INSERT INTO executions_visibility
            (namespace_id, run_id, workflow_id, workflow_type_name, status, start_time, execution_time, close_time, namespace_division, sa_keyword)
            VALUES ('ns-a','chasm','chasm','Engine',1,'2026-01-01','2026-01-01','2026-01-01',$1,$2::jsonb)`,
			strconv.Itoa(int(chasm.SchedulerArchetypeID)), `{"ChasmField":"value"}`)
		require.NoError(t, insertErr)
		plan, renderErr := renderList(q, "ns-a", 10, nil)
		require.NoError(t, renderErr)
		require.Equal(t, []string{"chasm"}, queryRunIDs(ctx, t, conn, plan.SQL, plan.Args))
	})

	_, err = conn.Exec(ctx, `INSERT INTO executions_visibility
        (namespace_id, run_id, workflow_id, workflow_type_name, status, start_time, execution_time, close_time, flow_type)
        SELECT 'ns-plan', 'filler-' || g, 'filler-' || g, 'Engine', 2,
          '2026-01-01'::timestamp, '2026-01-01'::timestamp, '2026-01-01'::timestamp, 'other'
        FROM generate_series(1, 1200) AS g`)
	require.NoError(t, err)
	_, err = conn.Exec(ctx, `INSERT INTO executions_visibility
        (namespace_id, run_id, workflow_id, workflow_type_name, status, start_time, execution_time, close_time, flow_type, namespace_division)
        VALUES ('ns-plan','target','wf-2','Engine',1,'2026-01-02','2026-01-02','2026-01-02','approval',NULL),
               ('ns-plan','schedule','schedule','Engine',1,'2026-01-02','2026-01-02','2026-01-02','other','TemporalScheduler')`)
	require.NoError(t, err)
	_, err = conn.Exec(ctx, "ANALYZE executions_visibility")
	require.NoError(t, err)
	t.Run("bound reached then fallback", func(t *testing.T) {
		_, insertErr := conn.Exec(ctx, `INSERT INTO executions_visibility
            (namespace_id, run_id, workflow_id, workflow_type_name, status, start_time, execution_time, close_time, flow_type, sa_keyword)
            SELECT 'ns-bound', 'candidate-' || g, 'candidate-' || g, 'Engine', 1,
              '2026-01-02'::timestamp, '2026-01-02'::timestamp, '2026-01-02'::timestamp,
              'approval', '{"KeywordField":"common"}'::jsonb
            FROM generate_series(1, 1000) AS g`)
		require.NoError(t, insertErr)
		_, insertErr = conn.Exec(ctx, `INSERT INTO executions_visibility
            (namespace_id, run_id, workflow_id, workflow_type_name, status, start_time, execution_time, close_time, flow_type, sa_keyword)
            VALUES ('ns-bound','target','target','Engine',1,'2026-01-01','2026-01-01','2026-01-01','approval','{"KeywordField":"rare"}')`)
		require.NoError(t, insertErr)
		q, convertErr := testConvert(t, `FlowType = 'approval' AND KeywordField = 'rare'`, chasm.UnspecifiedArchetypeID)
		require.NoError(t, convertErr)
		plan, renderErr := renderList(q, "ns-bound", 1, nil)
		require.NoError(t, renderErr)
		require.NotNil(t, plan.Bounded)
		rows, queryErr := conn.Query(ctx, plan.Bounded.SQL, plan.Bounded.Args...)
		require.NoError(t, queryErr)
		var scanned, matched int
		for rows.Next() {
			values, valuesErr := rows.Values()
			require.NoError(t, valuesErr)
			scanned++
			if values[len(values)-1].(bool) {
				matched++
			}
		}
		require.NoError(t, rows.Err())
		rows.Close()
		require.Equal(t, boundedScanLimit, scanned)
		require.Zero(t, matched)
		require.Equal(t, []string{"target"}, queryRunIDs(ctx, t, conn,
			plan.Bounded.FallbackSQL, plan.Bounded.FallbackArgs))
	})
	t.Run("bounded nullable residual", func(t *testing.T) {
		_, insertErr := conn.Exec(ctx, `INSERT INTO executions_visibility
            (namespace_id, run_id, workflow_id, workflow_type_name, status, start_time, execution_time, flow_type)
            VALUES ('ns-open','open','open','Engine',1,'2026-01-01','2026-01-01','approval')`)
		require.NoError(t, insertErr)
		q, convertErr := testConvert(t, `FlowType = 'approval' AND (CloseTime > '2026-01-01T00:00:00Z' OR KeywordField = 'red')`, chasm.UnspecifiedArchetypeID)
		require.NoError(t, convertErr)
		plan, renderErr := renderList(q, "ns-open", 1, nil)
		require.NoError(t, renderErr)
		require.NotNil(t, plan.Bounded)
		rows, queryErr := conn.Query(ctx, plan.Bounded.SQL, plan.Bounded.Args...)
		require.NoError(t, queryErr)
		require.True(t, rows.Next())
		values, valuesErr := rows.Values()
		require.NoError(t, valuesErr)
		require.Equal(t, false, values[len(values)-1])
		require.False(t, rows.Next())
		require.NoError(t, rows.Err())
		rows.Close()
	})
	_, err = conn.Exec(ctx, "SET enable_seqscan = off")
	require.NoError(t, err)
	for _, tc := range []struct{ filter, want string }{
		{`FlowType = 'approval'`, "by_flow_type"},
		{"", "default_idx"},
		{`ExecutionStatus = 'Completed'`, "default_idx"},
		{`WorkflowId = 'wf-2'`, "by_workflow_id"},
		{`TemporalNamespaceDivision = 'TemporalScheduler'`, "by_namespace_division"},
	} {
		q, convertErr := testConvert(t, tc.filter, chasm.UnspecifiedArchetypeID)
		require.NoError(t, convertErr)
		plan, renderErr := renderList(q, "ns-plan", 20, nil)
		require.NoError(t, renderErr)
		require.NotContains(t, plan.SQL, "@@@")
		explain := explainPlan(ctx, t, conn, plan.SQL, plan.Args)
		require.Contains(t, explain, tc.want)
		t.Logf("%s: %s", tc.want, explain)
	}
	q, err := testConvert(t, `KeywordField = 'red'`, chasm.UnspecifiedArchetypeID)
	require.NoError(t, err)
	plan, err := renderList(q, "ns-a", 20, nil)
	require.NoError(t, err)
	explain := explainPlan(ctx, t, conn, plan.SQL, plan.Args)
	t.Logf("BM25: %s", explain)
	require.Equal(t, 1, strings.Count(explain, "Custom Scan (ParadeDB Base Scan)"))
}

func seedParadeDB(ctx context.Context, t *testing.T, conn *pgx.Conn) {
	t.Helper()
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, row := range []struct {
		ns, run, workflow, flow, division, keyword, tags, numbers, dates, flags, words string
		status                                                                         int
		close                                                                          time.Time
	}{
		{"ns-a", "r1", "wf-1", "approval", "", `{"KeywordField":"red"}`, `{"Tags":["x","y"]}`, `{"IntField":3,"FloatField":1.5}`, `{"DateField":1767225600000000}`, `{"Flag":true}`, `{"Words":"hello world"}`, 1, start.Add(5 * time.Hour)},
		{"ns-a", "r2", "wf-2", "approval", "", `{"KeywordField":"blue"}`, `{"Tags":["y"]}`, `{"IntField":[1,5],"FloatField":3.5}`, `{"DateField":1767312000000000}`, `{"Flag":false}`, `{"Words":"goodbye world"}`, 2, start.Add(4 * time.Hour)},
		{"ns-a", "r3", "wf-3", "other", "", `{"KeywordField":"red"}`, `{}`, `{"IntField":2}`, `{}`, `{"Flag":true}`, `{"Words":"hello moon"}`, 1, start.Add(3 * time.Hour)},
		{"ns-a", "r4", "wf-4", "approval", "TemporalScheduler", `{"KeywordField":"red"}`, `{}`, `{}`, `{}`, `{}`, `{}`, 1, start.Add(2 * time.Hour)},
		{"ns-a", "r5", "wf-5", "approval", "", `{"KeywordField":"red"}`, `{"Tags":["x"]}`, `{"IntField":[2,8],"FloatField":1.0}`, `{}`, `{"Flag":false}`, `{}`, 1, start.Add(time.Hour)},
		{"ns-b", "other", "wf-6", "approval", "", `{"KeywordField":"red"}`, `{}`, `{}`, `{}`, `{}`, `{}`, 1, start},
	} {
		var division any
		if row.division != "" {
			division = row.division
		}
		_, err := conn.Exec(ctx, `INSERT INTO executions_visibility (namespace_id, run_id, workflow_id, workflow_type_name, status, start_time, execution_time, close_time, flow_type, namespace_division, sa_keyword, sa_number, sa_datetime, sa_bool, sa_text) VALUES ($1,$2,$3,'Engine',$4,$5,$5,$6,$7,$8,$9::jsonb,$10::jsonb,$11::jsonb,$12::jsonb,$13::jsonb)`, row.ns, row.run, row.workflow, row.status, start, row.close, row.flow, division, mergeJSON(row.keyword, row.tags), row.numbers, row.dates, row.flags, row.words)
		require.NoError(t, err)
	}
}

func mergeJSON(keyword, tags string) string {
	if tags == "{}" {
		return keyword
	}
	return strings.TrimSuffix(keyword, "}") + "," + strings.TrimPrefix(tags, "{")
}

func queryRunIDs(ctx context.Context, t *testing.T, conn *pgx.Conn, sql string, args []any) []string {
	t.Helper()
	rows, err := conn.Query(ctx, sql, args...)
	require.NoError(t, err)
	defer rows.Close()
	var result []string
	for rows.Next() {
		values, valuesErr := rows.Values()
		require.NoError(t, valuesErr)
		result = append(result, values[1].(string))
	}
	require.NoError(t, rows.Err())
	return result
}

func explainPlan(ctx context.Context, t *testing.T, conn *pgx.Conn, sql string, args []any) string {
	t.Helper()
	rows, err := conn.Query(ctx, "EXPLAIN "+sql, args...)
	require.NoError(t, err)
	defer rows.Close()
	var lines []string
	for rows.Next() {
		var line string
		require.NoError(t, rows.Scan(&line))
		lines = append(lines, line)
	}
	require.NoError(t, rows.Err())
	return strings.Join(lines, "\n")
}
