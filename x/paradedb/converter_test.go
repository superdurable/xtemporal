package paradedb

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/superdurable/xtemporal/chasm"
	"github.com/superdurable/xtemporal/common/log"
	"github.com/superdurable/xtemporal/common/namespace"
	"github.com/superdurable/xtemporal/common/searchattribute"
	"github.com/superdurable/xtemporal/common/searchattribute/sadefs"
	enumspb "go.temporal.io/api/enums/v1"
	"go.temporal.io/api/serviceerror"
)

func testConvert(t *testing.T, s string, archetype chasm.ArchetypeID) (*convertedQuery, error) {
	t.Helper()
	return convertQuery(s, namespace.Name("test"), searchattribute.NewNameTypeMap(map[string]enumspb.IndexedValueType{
		flowTypeSearchAttribute: enumspb.INDEXED_VALUE_TYPE_KEYWORD,
		"KeywordField":          enumspb.INDEXED_VALUE_TYPE_KEYWORD,
		"Tags":                  enumspb.INDEXED_VALUE_TYPE_KEYWORD_LIST,
		"Words":                 enumspb.INDEXED_VALUE_TYPE_TEXT,
		"IntField":              enumspb.INDEXED_VALUE_TYPE_INT,
		"FloatField":            enumspb.INDEXED_VALUE_TYPE_DOUBLE,
		"DateField":             enumspb.INDEXED_VALUE_TYPE_DATETIME,
		"Flag":                  enumspb.INDEXED_VALUE_TYPE_BOOL,
	}), &searchattribute.TestMapper{}, nil, archetype, nil, log.NewNoopLogger())
}

func TestConvertAndClassify(t *testing.T) {
	for _, tc := range []struct {
		name, input string
		want        route
		division    bool
	}{
		{"default", "", routeBTree, true},
		{"status only", `ExecutionStatus = 'Running'`, routeBTree, true},
		{"status negative", `ExecutionStatus != 'ContinuedAsNew' AND KeywordField = 'x'`, routeBM25, true},
		{"flow prefix", `FlowType = 'approval' AND KeywordField = 'x'`, routeBTree, true},
		{"flow in prefix", `FlowType IN ('approval','review') AND KeywordField = 'x'`, routeBTree, true},
		{"status in prefix", `ExecutionStatus IN ('Running','Completed') AND KeywordField = 'x'`, routeBTree, true},
		{"workflow prefix", `WorkflowId IN ('a','b') AND Tags = 'x'`, routeBTree, true},
		{"division prefix", `TemporalNamespaceDivision = 'TemporalScheduler' AND KeywordField = 'x'`, routeBTree, false},
		{"division in prefix", `TemporalNamespaceDivision IN ('A','B') AND KeywordField = 'x'`, routeBTree, false},
		{"text anywhere", `FlowType = 'approval' AND NOT (Words = 'hello')`, routeBM25, true},
		{"custom only", `IntField > 3`, routeBM25, true},
		{"or both prefixes", `(FlowType = 'a' AND KeywordField = 'x') OR (WorkflowId = 'b' AND KeywordField = 'y')`, routeBTree, true},
		{"or incomplete prefix", `(FlowType = 'a' AND KeywordField = 'x') OR KeywordField = 'y'`, routeBM25, true},
		{"outer prefix", `FlowType = 'a' AND (KeywordField = 'x' OR KeywordField = 'y')`, routeBTree, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			q, err := testConvert(t, tc.input, chasm.UnspecifiedArchetypeID)
			require.NoError(t, err)
			require.Equal(t, tc.want, classifyRoute(q.predicate))
			if tc.division {
				require.Contains(t, renderedSQL(t, q), "namespace_division IS NULL")
			} else {
				require.NotContains(t, renderedSQL(t, q), "namespace_division IS NULL")
			}
		})
	}
}

func renderedSQL(t *testing.T, q *convertedQuery) string {
	t.Helper()
	plan, err := renderList(q, "ns", 10, nil)
	require.NoError(t, err)
	return plan.SQL
}

func TestConvertCHASMAndGroupBy(t *testing.T) {
	q, err := testConvert(t, `ExecutionStatus = 'Running'`, chasm.SchedulerArchetypeID)
	require.NoError(t, err)
	plan, err := renderList(q, "ns", 10, nil)
	require.NoError(t, err)
	require.Contains(t, plan.SQL, "namespace_division")
	require.Contains(t, plan.Args, strconv.Itoa(int(chasm.SchedulerArchetypeID)))

	q, err = testConvert(t, `GROUP BY TemporalNamespaceDivision`, chasm.UnspecifiedArchetypeID)
	require.NoError(t, err)
	require.Equal(t, sadefs.TemporalNamespaceDivision, q.groupBy)
	count, err := renderCount(q, "ns")
	require.NoError(t, err)
	require.NotContains(t, count.SQL, "namespace_division IS NULL")
	require.Contains(t, count.SQL, "coalesce(namespace_division, '')")

	q, err = testConvert(t, `GROUP BY ExecutionStatus`, chasm.UnspecifiedArchetypeID)
	require.NoError(t, err)
	count, err = renderCount(q, "ns")
	require.NoError(t, err)
	require.Contains(t, count.SQL, "GROUP BY status")
}

func TestConvertRejects(t *testing.T) {
	for _, input := range []string{
		`KeywordField STARTS_WITH 'a'`,
		`KeywordField NOT STARTS_WITH 'a'`,
		`ORDER BY KeywordField`,
		`Unknown = 'x'`,
		`IntField = 'abc'`,
		`Words IN ('a','b')`,
		`GROUP BY KeywordField`,
		`DateField = 'not a time'`,
		`Flag = 1`,
	} {
		t.Run(input, func(t *testing.T) {
			_, err := testConvert(t, input, chasm.UnspecifiedArchetypeID)
			var invalid *serviceerror.InvalidArgument
			require.ErrorAs(t, err, &invalid)
		})
	}
}

func TestConvertCHASMMapper(t *testing.T) {
	mapper := chasm.NewTestVisibilitySearchAttributesMapper(
		map[string]string{"ChasmField": "ChasmAlias"},
		map[string]enumspb.IndexedValueType{"ChasmField": enumspb.INDEXED_VALUE_TYPE_KEYWORD},
	)
	q, err := convertQuery(`ChasmAlias = 'value'`, namespace.Name("test"),
		searchattribute.NewNameTypeMap(nil), &searchattribute.TestMapper{}, mapper,
		chasm.SchedulerArchetypeID, nil, log.NewNoopLogger())
	require.NoError(t, err)
	plan, err := renderList(q, "ns", 10, nil)
	require.NoError(t, err)
	require.Contains(t, plan.SQL, `sa_keyword->'ChasmField'`)
	require.Contains(t, plan.Args, strconv.Itoa(int(chasm.SchedulerArchetypeID)))
	require.Contains(t, plan.Args, "value")
}

func TestFlowTypeMustBeScalar(t *testing.T) {
	_, err := convertQuery(`FlowType = 'x'`, namespace.Name("test"),
		searchattribute.NewNameTypeMap(map[string]enumspb.IndexedValueType{
			flowTypeSearchAttribute: enumspb.INDEXED_VALUE_TYPE_KEYWORD_LIST,
		}), &searchattribute.TestMapper{}, nil, chasm.UnspecifiedArchetypeID, nil, log.NewNoopLogger())
	var invalid *serviceerror.InvalidArgument
	require.ErrorAs(t, err, &invalid)
}
