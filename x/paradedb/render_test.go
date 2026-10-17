package paradedb

import (
	"encoding/base64"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/superdurable/xtemporal/chasm"
	"github.com/superdurable/xtemporal/common/persistence/visibility/store/query"
	"github.com/temporalio/sqlparser"
	enumspb "go.temporal.io/api/enums/v1"
	"go.temporal.io/api/serviceerror"
)

func TestRenderOperators(t *testing.T) {
	for _, tc := range []struct {
		input, fragment string
		route           route
	}{
		{`KeywordField = 'x'`, `@@@ pdb.term`, routeBM25},
		{`KeywordField != 'x'`, `NOT (`, routeBM25},
		{`KeywordField IN ('x', 'y')`, ` OR `, routeBM25},
		{`KeywordField NOT IN ('x', 'y')`, `NOT (`, routeBM25},
		{`KeywordField > 'x'`, `element.value::text COLLATE "C" >`, routeBM25},
		{`KeywordField >= 'x'`, `element.value::text COLLATE "C" >=`, routeBM25},
		{`KeywordField < 'x'`, `element.value::text COLLATE "C" <`, routeBM25},
		{`KeywordField <= 'x'`, `element.value::text COLLATE "C" <=`, routeBM25},
		{`KeywordField BETWEEN 'a' AND 'z'`, `element.value::text COLLATE "C" >=`, routeBM25},
		{`KeywordField NOT BETWEEN 'a' AND 'z'`, `NOT (`, routeBM25},
		{`Tags = 'x'`, `@@@ pdb.term`, routeBM25},
		{`Tags != 'x'`, `NOT (`, routeBM25},
		{`IntField = 3`, `::bigint`, routeBM25},
		{`IntField >= 3`, `pdb.range(int8range`, routeBM25},
		{`FloatField < 3.5`, `::double precision <`, routeBM25},
		{`DateField >= '2026-01-01T00:00:00Z'`, `pdb.range(int8range`, routeBM25},
		{`Flag = true`, `::boolean`, routeBM25},
		{`Words = 'hello'`, `|||`, routeBM25},
		{`Words != 'hello'`, `NOT (`, routeBM25},
		{`KeywordField IS NULL`, `IS NULL`, routeBM25},
		{`KeywordField IS NOT NULL`, `IS NOT NULL`, routeBM25},
		{`FlowType = 'x' AND Tags = 'a'`, `jsonb_array_elements_text`, routeBTree},
	} {
		t.Run(tc.input, func(t *testing.T) {
			q, err := testConvert(t, tc.input, chasm.UnspecifiedArchetypeID)
			require.NoError(t, err)
			p, err := renderList(q, "ns", 20, nil)
			require.NoError(t, err)
			require.Equal(t, tc.route, p.Route)
			require.Contains(t, p.SQL, tc.fragment)
			require.Contains(t, p.SQL, "namespace_id = $1")
			require.Equal(t, "ns", p.Args[0])
			require.Equal(t, 20, p.Args[len(p.Args)-1])
			if p.Route == routeBTree {
				require.NotContains(t, p.SQL, "@@@")
			} else {
				require.Contains(t, p.SQL, `namespace_id @@@ pdb.term($1::text)`)
			}
		})
	}
}

func TestRenderMissingArrayAndNested(t *testing.T) {
	q, err := testConvert(t, `FlowType = 'x' AND NOT ((Tags = 'a' OR IntField > 4) AND Flag = true)`, chasm.UnspecifiedArchetypeID)
	require.NoError(t, err)
	p, err := renderList(q, "ns", 5, nil)
	require.NoError(t, err)
	require.Equal(t, routeBTree, p.Route)
	require.Contains(t, p.SQL, `jsonb_typeof(sa_keyword->'Tags')`)
	require.Contains(t, p.SQL, `EXISTS (SELECT 1 FROM jsonb_array_elements_text`)
	require.Contains(t, p.SQL, `NOT (`)
	require.Contains(t, p.SQL, ` OR `)
	require.Nil(t, p.Bounded)
}

func TestRenderInjectionAndFullSelect(t *testing.T) {
	q, err := testConvert(t, `KeywordField = 'x''; DROP TABLE executions_visibility; --'`, chasm.UnspecifiedArchetypeID)
	require.NoError(t, err)
	p, err := renderList(q, "ns", 10, nil)
	require.NoError(t, err)
	require.NotContains(t, p.SQL, `DROP TABLE`)
	require.Contains(t, p.Args, "x'; DROP TABLE executions_visibility; --")
	for _, col := range strings.Split(strings.ReplaceAll(selectColumns, "\n", ""), ",") {
		require.Contains(t, p.SQL, strings.TrimSpace(col))
	}
	name, err := quoteAttributeName("key_with_quote")
	require.NoError(t, err)
	require.Equal(t, "'key_with_quote'", name)
	_, err = quoteAttributeName("bad\x00name")
	require.Error(t, err)
}

func TestPageTokenAndKeyset(t *testing.T) {
	closeTime := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	startTime := closeTime.Add(-time.Hour)
	token, err := encodePageToken(closeTime, startTime, "run-1")
	require.NoError(t, err)
	cursor, err := decodePageToken(token)
	require.NoError(t, err)
	require.Equal(t, closeTime, cursor.SortCloseTime)
	require.Equal(t, startTime, cursor.StartTime)
	require.Equal(t, "run-1", cursor.RunID)
	partial, err := nextPageToken(1, 2, closeTime, startTime, "run-1")
	require.NoError(t, err)
	require.Nil(t, partial)
	full, err := nextPageToken(2, 2, closeTime, startTime, "run-1")
	require.NoError(t, err)
	require.Equal(t, token, full)
	q, err := testConvert(t, `FlowType = 'x' AND KeywordField = 'a'`, chasm.UnspecifiedArchetypeID)
	require.NoError(t, err)
	p, err := renderList(q, "ns", 2, token)
	require.NoError(t, err)
	require.Contains(t, p.SQL, "sort_close_time <=")
	require.Contains(t, p.SQL, `run_id COLLATE "C" >`)
	require.Contains(t, p.Args, closeTime)
	require.Contains(t, p.Args, startTime)
	require.Contains(t, p.Args, "run-1")
	require.NotNil(t, p.Bounded)
	require.Contains(t, p.Bounded.SQL, "sort_close_time <=")
	for _, invalid := range [][]byte{[]byte("bad"), []byte(base64.RawURLEncoding.EncodeToString([]byte(`{"v":2}`)))} {
		_, err := renderList(q, "ns", 2, invalid)
		var arg *serviceerror.InvalidArgument
		require.ErrorAs(t, err, &arg)
	}
}

func TestCountPlan(t *testing.T) {
	q, err := testConvert(t, `KeywordField = 'x' GROUP BY ExecutionStatus`, chasm.UnspecifiedArchetypeID)
	require.NoError(t, err)
	p, err := renderCount(q, "ns")
	require.NoError(t, err)
	require.Equal(t, routeBM25, p.Route)
	require.Contains(t, p.SQL, "GROUP BY status")
	require.NotContains(t, p.SQL, "ORDER BY")
	require.NotContains(t, p.SQL, "LIMIT")
	q, err = testConvert(t, `TemporalNamespaceDivision = 'TemporalScheduler' GROUP BY TemporalNamespaceDivision`, chasm.UnspecifiedArchetypeID)
	require.NoError(t, err)
	p, err = renderCount(q, "ns")
	require.NoError(t, err)
	require.Equal(t, routeBTree, p.Route)
	require.NotContains(t, p.SQL, "namespace_division IS NULL")
}

func TestRenderMalformedAst(t *testing.T) {
	bad := &convertedQuery{predicate: &expression{kind: atomExpr, column: query.NewSAColumn("Injected", "bad\x00name", enumspb.INDEXED_VALUE_TYPE_KEYWORD), op: sqlparser.EqualStr, values: []any{"x"}}}
	_, err := renderList(bad, "ns", 10, nil)
	var arg *serviceerror.InvalidArgument
	require.ErrorAs(t, err, &arg)
}

func TestRenderDoubleEqualityUsesTypedSQL(t *testing.T) {
	for _, input := range []string{`FloatField = 2`, `FloatField IN (2, 3.5)`, `FloatField != 2`} {
		t.Run(input, func(t *testing.T) {
			q, err := testConvert(t, input, chasm.UnspecifiedArchetypeID)
			require.NoError(t, err)
			plan, err := renderList(q, "ns", 10, nil)
			require.NoError(t, err)
			require.Contains(t, plan.SQL, "::double precision")
			require.NotContains(t, plan.SQL, `sa_number->'FloatField' @@@ pdb.term`)
		})
	}
}

func TestRenderBoundedEquivalence(t *testing.T) {
	for _, tc := range []struct {
		filter  string
		bounded bool
	}{
		{`FlowType = 'approval' AND FloatField = 1`, true},
		{`FlowType = 'approval' AND KeywordField = 'x'`, true},
		{`FlowType = 'approval' AND IntField > 4`, false},
	} {
		q, err := testConvert(t, tc.filter, chasm.UnspecifiedArchetypeID)
		require.NoError(t, err)
		plan, err := renderList(q, "ns", 2, nil)
		require.NoError(t, err)
		if tc.bounded {
			require.NotNil(t, plan.Bounded)
			require.Contains(t, plan.Bounded.SQL, "scanned_count")
			require.NotContains(t, plan.Bounded.SQL, "memo FROM executions_visibility")
		} else {
			require.Nil(t, plan.Bounded)
		}
	}
}

func TestRenderKeywordRangeCollation(t *testing.T) {
	for _, tc := range []struct{ input, fragment string }{
		{`KeywordField > 'a'`, `element.value::text COLLATE "C" >`},
		{`KeywordField BETWEEN 'a' AND 'z'`, `element.value::text COLLATE "C" >=`},
		{`FlowType = 'approval' AND KeywordField < 'z'`, `element.value::text COLLATE "C" <`},
		{`WorkflowId > 'a'`, `workflow_id COLLATE "C" >`},
		{`WorkflowType BETWEEN 'a' AND 'z'`, `workflow_type_name COLLATE "C" >=`},
		{`RunId < 'z'`, `run_id COLLATE "C" <`},
		{`TaskQueue >= 'a'`, `task_queue COLLATE "C" >=`},
	} {
		t.Run(tc.input, func(t *testing.T) {
			q, err := testConvert(t, tc.input, chasm.UnspecifiedArchetypeID)
			require.NoError(t, err)
			plan, err := renderList(q, "ns", 10, nil)
			require.NoError(t, err)
			require.Contains(t, plan.SQL, tc.fragment)
		})
	}
}

func TestRenderIntRangeExtremes(t *testing.T) {
	for _, input := range []string{`IntField > 9223372036854775807`, `IntField < -9223372036854775808`} {
		q, err := testConvert(t, input, chasm.UnspecifiedArchetypeID)
		require.NoError(t, err)
		plan, err := renderList(q, "ns", 10, nil)
		require.NoError(t, err)
		require.Contains(t, plan.SQL, "FALSE")
		require.NotContains(t, plan.SQL, "int8range")
	}
}

func TestQuoteAttributeNameRejectsUnsafeNames(t *testing.T) {
	for _, name := range []string{"9bad", "bad-name", "bad'name", "bad name", "bad.name", "éclair"} {
		_, err := quoteAttributeName(name)
		var invalid *serviceerror.InvalidArgument
		require.ErrorAs(t, err, &invalid)
	}
	quoted, err := quoteAttributeName("Good_name9")
	require.NoError(t, err)
	require.Equal(t, "'Good_name9'", quoted)
}
