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

func TestQueryTranslationTable(t *testing.T) {
	c := &queryConverter{}
	keyword := query.NewSAColumn("Keyword01", "Keyword01", 0)
	textCol := query.NewSAColumn("Text01", "Text01", 0)
	list := query.NewSAColumn("KeywordList01", "KeywordList01", 0)
	numeric := query.NewSAColumn("Int01", "Int01", 0)
	for _, tc := range []struct {
		name    string
		convert func() (sqlparser.Expr, error)
		want    string
	}{
		{"ordinary comparison", func() (sqlparser.Expr, error) {
			return c.ConvertComparisonExpr(sqlparser.EqualStr, numeric, sqlparser.NewIntVal([]byte("42")))
		}, "Int01 = 42"},
		{"namespace non-equality", func() (sqlparser.Expr, error) {
			return c.ConvertComparisonExpr(sqlparser.NotEqualStr, query.NamespaceIDSAColumn, query.NewUnsafeSQLString("ns"))
		}, "namespace_id != 'ns'"},
		{"keyword in", func() (sqlparser.Expr, error) {
			return c.ConvertKeywordComparisonExpr(sqlparser.InStr, keyword, sqlparser.ValTuple{query.NewUnsafeSQLString("a"), query.NewUnsafeSQLString("b")})
		}, "Keyword01 in ('a', 'b')"},
		{"indexed negative prefix", func() (sqlparser.Expr, error) {
			return c.ConvertKeywordComparisonExpr(sqlparser.NotStartsWithStr, keyword, query.NewUnsafeSQLString("pre.+"))
		}, "not (Keyword01 @@@ pdb.regex('pre\\.\\+.*'))"},
		{"fallback negative prefix", func() (sqlparser.Expr, error) {
			return c.ConvertKeywordComparisonExpr(sqlparser.NotStartsWithStr, query.NewSAColumn("TemporalKeyword01", "TemporalKeyword01", 0), query.NewUnsafeSQLString("pre!_%"))
		}, "not (TemporalKeyword01 LIKE 'pre!!!_!%%' ESCAPE '!')"},
		{"indexed list not equal", func() (sqlparser.Expr, error) {
			return c.ConvertKeywordListComparisonExpr(sqlparser.NotEqualStr, list, query.NewUnsafeSQLString("tag"))
		}, "not (KeywordList01 @@@ pdb.term('tag'))"},
		{"indexed list not in", func() (sqlparser.Expr, error) {
			return c.ConvertKeywordListComparisonExpr(sqlparser.NotInStr, list, sqlparser.ValTuple{query.NewUnsafeSQLString("a"), query.NewUnsafeSQLString("b")})
		}, "not ((KeywordList01 @@@ pdb.term('a') or KeywordList01 @@@ pdb.term('b')))"},
		{"fallback list in", func() (sqlparser.Expr, error) {
			return c.ConvertKeywordListComparisonExpr(sqlparser.InStr, query.NewSAColumn("BuildIds", "BuildIds", 0), sqlparser.ValTuple{query.NewUnsafeSQLString("a"), query.NewUnsafeSQLString("b")})
		}, "(BuildIds @> jsonb_build_array('a') or BuildIds @> jsonb_build_array('b'))"},
		{"fallback list not equal", func() (sqlparser.Expr, error) {
			return c.ConvertKeywordListComparisonExpr(sqlparser.NotEqualStr, query.NewSAColumn("BuildIds", "BuildIds", 0), query.NewUnsafeSQLString("a"))
		}, "not BuildIds @> jsonb_build_array('a')"},
		{"text equality", func() (sqlparser.Expr, error) {
			return c.ConvertTextComparisonExpr(sqlparser.EqualStr, textCol, query.NewUnsafeSQLString("foo bar"))
		}, "Text01 ||| 'foo bar'"},
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

func TestInvalidQueryTranslations(t *testing.T) {
	c := &queryConverter{}
	keyword := query.NewSAColumn("Keyword01", "Keyword01", 0)
	list := query.NewSAColumn("KeywordList01", "KeywordList01", 0)
	textCol := query.NewSAColumn("Text01", "Text01", 0)
	for _, tc := range []struct {
		name    string
		convert func() (sqlparser.Expr, error)
	}{
		{"prefix needs string", func() (sqlparser.Expr, error) {
			return c.ConvertKeywordComparisonExpr(sqlparser.StartsWithStr, keyword, sqlparser.NewIntVal([]byte("1")))
		}},
		{"list equality needs string", func() (sqlparser.Expr, error) {
			return c.ConvertKeywordListComparisonExpr(sqlparser.NotEqualStr, list, sqlparser.NewIntVal([]byte("1")))
		}},
		{"list in needs tuple", func() (sqlparser.Expr, error) {
			return c.ConvertKeywordListComparisonExpr(sqlparser.InStr, list, query.NewUnsafeSQLString("a"))
		}},
		{"list in needs strings", func() (sqlparser.Expr, error) {
			return c.ConvertKeywordListComparisonExpr(sqlparser.InStr, list, sqlparser.ValTuple{query.NewUnsafeSQLString("a"), sqlparser.NewIntVal([]byte("1"))})
		}},
		{"list rejects operator", func() (sqlparser.Expr, error) {
			return c.ConvertKeywordListComparisonExpr(sqlparser.GreaterThanStr, list, query.NewUnsafeSQLString("a"))
		}},
		{"text needs string", func() (sqlparser.Expr, error) {
			return c.ConvertTextComparisonExpr(sqlparser.EqualStr, textCol, sqlparser.NewIntVal([]byte("1")))
		}},
		{"text rejects operator", func() (sqlparser.Expr, error) {
			return c.ConvertTextComparisonExpr(sqlparser.GreaterThanStr, textCol, query.NewUnsafeSQLString("foo"))
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			expr, err := tc.convert()
			require.Error(t, err)
			require.Nil(t, expr)
		})
	}
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

func TestSelectPaginationAndCount(t *testing.T) {
	c := &queryConverter{}
	stmt, args := c.BuildSelectStmt(nil, 25, nil)
	require.NotContains(t, stmt, " WHERE ")
	require.Contains(t, stmt, `ORDER BY sort_close_time DESC, start_time DESC, run_id COLLATE "C" LIMIT ?`)
	require.Equal(t, []any{25}, args)

	closeTime := time.Date(2026, time.October, 14, 10, 0, 0, 0, time.UTC)
	startTime := closeTime.Add(-time.Hour)
	stmt, args = c.BuildSelectStmt(&query.QueryParams[sqlparser.Expr]{}, 10, &sqlplugin.VisibilityPageToken{
		CloseTime: closeTime,
		StartTime: startTime,
		RunID:     "run",
	})
	require.Contains(t, stmt, `WHERE ((sort_close_time = ? AND start_time = ? AND run_id COLLATE "C" > ?) OR (sort_close_time = ? AND start_time < ?) OR sort_close_time < ?)`)
	require.Equal(t, []any{closeTime, startTime, "run", closeTime, startTime, closeTime, 10}, args)

	count, countArgs := c.BuildCountStmt(&query.QueryParams[sqlparser.Expr]{})
	require.Equal(t, "SELECT COUNT(*) FROM executions_visibility", count)
	require.Empty(t, countArgs)
	count, countArgs = c.BuildCountStmt(&query.QueryParams[sqlparser.Expr]{
		GroupBy: []*query.SAColumn{
			query.NewSAColumn(sadefs.ExecutionStatus, sadefs.ExecutionStatus, 0),
			query.NewSAColumn("Keyword01", "Keyword01", 0),
		},
	})
	require.Equal(t, "SELECT status, Keyword01, COUNT(*) FROM executions_visibility GROUP BY status, Keyword01", count)
	require.Empty(t, countArgs)
}
