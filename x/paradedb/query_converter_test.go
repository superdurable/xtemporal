package paradedb

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/superdurable/xtemporal/common/persistence/sql"
	"github.com/superdurable/xtemporal/common/persistence/sql/sqlplugin"
	"github.com/superdurable/xtemporal/common/persistence/visibility/store/query"
	"github.com/superdurable/xtemporal/common/searchattribute/sadefs"
	"github.com/temporalio/sqlparser"
)

func TestRegistration(t *testing.T) {
	converter, err := sql.GetPluginVisibilityQueryConverter(PluginName)
	require.NoError(t, err)
	require.IsType(t, &queryConverter{}, converter)
}

func TestComparisonConversions(t *testing.T) {
	c := &queryConverter{}
	keyword := query.NewSAColumn("Keyword02", "Keyword02", 0)
	list := query.NewSAColumn("KeywordList01", "KeywordList01", 0)
	textCol := query.NewSAColumn("Text01", "Text01", 0)
	for _, tc := range []struct {
		name    string
		convert func() (sqlparser.Expr, error)
		want    string
	}{
		{"namespace", func() (sqlparser.Expr, error) {
			return c.ConvertComparisonExpr(sqlparser.EqualStr, query.NamespaceIDSAColumn, query.NewUnsafeSQLString("ns"))
		}, "namespace_id @@@ 'ns'"},
		{"prefix", func() (sqlparser.Expr, error) {
			return c.ConvertKeywordComparisonExpr(sqlparser.StartsWithStr, keyword, query.NewUnsafeSQLString("pre.+"))
		}, "Keyword02 @@@ pdb.regex('pre\\.\\+.*')"},
		{"unindexed prefix", func() (sqlparser.Expr, error) {
			return c.ConvertKeywordComparisonExpr(sqlparser.StartsWithStr, query.NewSAColumn("TemporalKeyword01", "TemporalKeyword01", 0), query.NewUnsafeSQLString("pre_%"))
		}, "TemporalKeyword01 LIKE 'pre!_!%%' ESCAPE '!'"},
		{"keyword list", func() (sqlparser.Expr, error) {
			return c.ConvertKeywordListComparisonExpr(sqlparser.EqualStr, list, query.NewUnsafeSQLString("tag"))
		}, "KeywordList01 @@@ pdb.term('tag')"},
		{"builtin list", func() (sqlparser.Expr, error) {
			return c.ConvertKeywordListComparisonExpr(sqlparser.EqualStr, query.NewSAColumn("BuildIds", "BuildIds", 0), query.NewUnsafeSQLString("versioned:build"))
		}, "BuildIds @> jsonb_build_array('versioned:build')"},
		{"keyword list in", func() (sqlparser.Expr, error) {
			return c.ConvertKeywordListComparisonExpr(sqlparser.InStr, list, sqlparser.ValTuple{query.NewUnsafeSQLString("a"), query.NewUnsafeSQLString("b")})
		}, "(KeywordList01 @@@ pdb.term('a') or KeywordList01 @@@ pdb.term('b'))"},
		{"text", func() (sqlparser.Expr, error) {
			return c.ConvertTextComparisonExpr(sqlparser.EqualStr, textCol, query.NewUnsafeSQLString("foo bar"))
		}, "Text01 ||| 'foo bar'"},
		{"text exclusion", func() (sqlparser.Expr, error) {
			return c.ConvertTextComparisonExpr(sqlparser.NotEqualStr, textCol, query.NewUnsafeSQLString("foo"))
		}, "not (Text01 ||| 'foo')"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			expr, err := tc.convert()
			require.NoError(t, err)
			require.Equal(t, tc.want, sqlparser.String(expr))
		})
	}
}

func TestInvalidExpressions(t *testing.T) {
	c := &queryConverter{}
	col := query.NewSAColumn("Text01", "Text01", 0)
	list := query.NewSAColumn("KeywordList01", "KeywordList01", 0)
	_, err := c.ConvertTextComparisonExpr(sqlparser.EqualStr, col, query.NewUnsafeSQLString("  "))
	require.Error(t, err)
	_, err = c.ConvertTextComparisonExpr(sqlparser.EqualStr, col, query.NewUnsafeSQLString("valid"))
	require.NoError(t, err)
	_, err = c.ConvertKeywordListComparisonExpr(sqlparser.InStr, list, sqlparser.ValTuple{})
	require.Error(t, err)
	_, err = c.ConvertKeywordListComparisonExpr(sqlparser.EqualStr, list, sqlparser.NewIntVal([]byte("2")))
	require.Error(t, err)
}

func TestStatements(t *testing.T) {
	c := &queryConverter{}
	ns, err := c.ConvertComparisonExpr(sqlparser.EqualStr, query.NamespaceIDSAColumn, query.NewUnsafeSQLString("ns"))
	require.NoError(t, err)
	params := &query.QueryParams[sqlparser.Expr]{QueryExpr: ns, GroupBy: []*query.SAColumn{query.NewSAColumn(sadefs.ExecutionStatus, sadefs.ExecutionStatus, 0)}}
	stmt, args := c.BuildSelectStmt(params, 20, &sqlplugin.VisibilityPageToken{CloseTime: time.Now(), StartTime: time.Now(), RunID: "run"})
	require.Contains(t, stmt, "WHERE namespace_id @@@ 'ns' AND")
	require.Contains(t, stmt, `ORDER BY sort_close_time DESC, start_time DESC, run_id COLLATE "C" LIMIT ?`)
	require.Contains(t, stmt, `run_id COLLATE "C" > ?`)
	require.Len(t, args, 7)
	require.Equal(t, 20, args[6])
	count, countArgs := c.BuildCountStmt(params)
	require.Equal(t, "SELECT status, COUNT(*) FROM executions_visibility WHERE namespace_id @@@ 'ns' GROUP BY status", count)
	require.Empty(t, countArgs)
	require.True(t, strings.HasPrefix(stmt, "SELECT "))
}
