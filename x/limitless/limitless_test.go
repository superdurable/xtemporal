package limitless

import (
	"context"
	"database/sql"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/require"
	"github.com/superdurable/xtemporal/common/config"
	"github.com/superdurable/xtemporal/common/log"
	"github.com/superdurable/xtemporal/common/persistence/sql/sqlplugin"
	"github.com/superdurable/xtemporal/common/primitives"
	limitlessschema "github.com/superdurable/xtemporal/schema/limitless"
	pgschema "github.com/superdurable/xtemporal/schema/postgresql/v12"
)

type fakeDB struct {
	sqlplugin.DB
	innerTx   sqlplugin.Tx
	query     string
	args      []any
	calls     int
	result    sql.Result
	err       error
	limitless bool
}

func (f *fakeDB) BeginTx(context.Context) (sqlplugin.Tx, error) { return f.innerTx, f.err }
func (f *fakeDB) ExecContext(_ context.Context, query string, args ...any) (sql.Result, error) {
	f.calls++
	f.query, f.args = query, args
	return f.result, f.err
}
func (f *fakeDB) GetContext(_ context.Context, dest any, _ string, _ ...any) error {
	if f.err != nil {
		return f.err
	}
	*dest.(*bool) = f.limitless
	return nil
}

type fakeTx struct {
	sqlplugin.Tx
	query string
	args  []any
	calls int
}

func (f *fakeTx) ExecContext(_ context.Context, query string, args ...any) (sql.Result, error) {
	f.calls++
	f.query, f.args = query, args
	return nil, nil
}

func bufferedRows(n int) []sqlplugin.BufferedEventsRow {
	rows := make([]sqlplugin.BufferedEventsRow, n)
	for i := range rows {
		rows[i] = sqlplugin.BufferedEventsRow{ShardID: 7, NamespaceID: primitives.UUID{1}, WorkflowID: "wf", RunID: primitives.UUID{2}, Data: []byte{byte(i + 1)}, DataEncoding: "enc"}
	}
	return rows
}

func TestPluginAndVersion(t *testing.T) {
	_, err := (&plugin{}).CreateDB(sqlplugin.DbKindVisibility, &config.SQL{}, nil, nil, nil)
	require.ErrorContains(t, err, "only supports the default store")
	require.Nil(t, (&plugin{}).GetVisibilityQueryConverter())
	require.Equal(t, pgschema.Version, limitlessschema.Version)
}

func TestBeginTxUsesWrapper(t *testing.T) {
	innerTx := &fakeTx{}
	inner := &fakeDB{innerTx: innerTx}
	wrapper := &db{DB: inner}
	got, err := wrapper.BeginTx(context.Background())
	require.NoError(t, err)
	require.IsType(t, &tx{}, got)
	_, err = got.InsertIntoBufferedEvents(context.Background(), bufferedRows(1))
	require.NoError(t, err)
	require.Equal(t, 1, innerTx.calls)
	require.Contains(t, innerTx.query, "COALESCE(MAX(id), 0)")
	inner.err = errors.New("begin failed")
	_, err = wrapper.BeginTx(context.Background())
	require.ErrorContains(t, err, "begin failed")
}

func TestBufferedInsert(t *testing.T) {
	for _, n := range []int{1, 3} {
		t.Run(string(rune('0'+n)), func(t *testing.T) {
			inner := &fakeDB{}
			rows := bufferedRows(n)
			_, err := (&db{DB: inner}).InsertIntoBufferedEvents(context.Background(), rows)
			require.NoError(t, err)
			require.Equal(t, 1, inner.calls)
			prefix := "INSERT INTO buffered_events (shard_id, namespace_id, workflow_id, run_id, id, data, data_encoding) SELECT $1::integer, $2::bytea, $3::varchar(255), $4::bytea, (SELECT COALESCE(MAX(id), 0) FROM buffered_events WHERE shard_id = $1::integer AND namespace_id = $2::bytea AND workflow_id = $3::varchar(255) AND run_id = $4::bytea) + v.n, v.data, v.data_encoding FROM (VALUES "
			suffix := ") AS v(n, data, data_encoding)"
			values := "($5::bigint, $6::bytea, $7::text)"
			if n == 3 {
				values += ", ($8::bigint, $9::bytea, $10::text), ($11::bigint, $12::bytea, $13::text)"
			}
			require.Equal(t, prefix+values+suffix, inner.query)
			require.Equal(t, n, strings.Count(inner.query, "::bigint"))
			require.Len(t, inner.args, 4+3*n)
			require.Equal(t, rows[0].ShardID, inner.args[0])
			require.Equal(t, rows[0].NamespaceID, inner.args[1])
			require.Equal(t, rows[0].WorkflowID, inner.args[2])
			require.Equal(t, rows[0].RunID, inner.args[3])
			for i := range rows {
				require.Equal(t, int64(i+1), inner.args[4+3*i])
				require.Equal(t, rows[i].Data, inner.args[5+3*i])
				require.Equal(t, rows[i].DataEncoding, inner.args[6+3*i])
			}
		})
	}
	inner := &fakeDB{}
	rows := bufferedRows(2)
	rows[1].WorkflowID = "other"
	_, err := (&db{DB: inner}).InsertIntoBufferedEvents(context.Background(), rows)
	require.ErrorContains(t, err, "one workflow")
	require.Zero(t, inner.calls)
	_, err = insertIntoBufferedEvents(context.Background(), struct{}{}, bufferedRows(1))
	require.ErrorContains(t, err, "ExecContext")
	_, err = insertIntoBufferedEvents(context.Background(), inner, nil)
	require.ErrorContains(t, err, "at least one row")
	inner.err = errors.New("exec failed")
	_, err = insertIntoBufferedEvents(context.Background(), inner, bufferedRows(1))
	require.ErrorContains(t, err, "exec failed")
}

func TestRewriteSchemaStatements(t *testing.T) {
	statements := []string{"CREATE TABLE t (id bigint)", "  CALL rds_aurora.limitless_alter_table_type_sharded('t', 'id')", "CALL other_proc()"}
	inner := &fakeDB{}
	wrapper := &db{DB: inner, logger: log.NewNoopLogger()}
	require.Equal(t, []string{statements[0], statements[2]}, wrapper.RewriteSchemaStatements(statements))
	inner.limitless = true
	require.Equal(t, statements, wrapper.RewriteSchemaStatements(statements))
	inner.err = errors.New("detection failed")
	require.Equal(t, statements, wrapper.RewriteSchemaStatements(statements))
}

func TestBufferedInsertCallSites(t *testing.T) {
	root := filepath.Join("..", "..", "common", "persistence", "sql")
	files, err := filepath.Glob(filepath.Join(root, "*.go"))
	require.NoError(t, err)
	allowed := map[string]bool{
		"execution_state_non_map.go:updateBufferedEvents:InsertIntoBufferedEvents": true,
		"execution_util.go:applyWorkflowMutationTx:updateBufferedEvents":           true,
	}
	seen := make(map[string]bool)
	for _, path := range files {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		require.NoError(t, err)
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			ast.Inspect(fn.Body, func(node ast.Node) bool {
				call, ok := node.(*ast.CallExpr)
				if !ok {
					return true
				}
				var name string
				switch fun := call.Fun.(type) {
				case *ast.Ident:
					name = fun.Name
				case *ast.SelectorExpr:
					name = fun.Sel.Name
				default:
					return true
				}
				if name == "InsertIntoBufferedEvents" || name == "updateBufferedEvents" {
					seen[filepath.Base(path)+":"+fn.Name.Name+":"+name] = true
				}
				return true
			})
		}
	}
	require.Equal(t, allowed, seen, "re-verify the FOR UPDATE precondition for buffered-events call sites")
}

func TestBufferedInsertPostgres(t *testing.T) {
	dsn := os.Getenv("XTEMPORAL_LIMITLESS_TEST_DSN")
	if dsn == "" {
		t.Skip("XTEMPORAL_LIMITLESS_TEST_DSN is unset")
	}
	ctx := context.Background()
	connDB, err := sql.Open("pgx", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, connDB.Close()) })
	conn, err := connDB.Conn(ctx)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, conn.Close()) })
	_, err = conn.ExecContext(ctx, "CREATE TEMP TABLE buffered_events (shard_id INTEGER NOT NULL, namespace_id BYTEA NOT NULL, workflow_id VARCHAR(255) NOT NULL, run_id BYTEA NOT NULL, id BIGINT NOT NULL, data BYTEA NOT NULL, data_encoding VARCHAR(16) NOT NULL, PRIMARY KEY (shard_id, namespace_id, workflow_id, run_id, id))")
	require.NoError(t, err)
	for _, count := range []int{1, 3, 1} {
		_, err = insertIntoBufferedEvents(ctx, conn, bufferedRows(count))
		require.NoError(t, err)
	}
	rows := bufferedRows(1)
	rows[0].WorkflowID = "second"
	_, err = insertIntoBufferedEvents(ctx, conn, rows)
	require.NoError(t, err)
	got, err := conn.QueryContext(ctx, "SELECT workflow_id, id FROM buffered_events ORDER BY workflow_id, id")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, got.Close()) })
	var firstIDs, secondIDs []int64
	for got.Next() {
		var workflow string
		var id int64
		require.NoError(t, got.Scan(&workflow, &id))
		if workflow == "wf" {
			firstIDs = append(firstIDs, id)
		} else {
			secondIDs = append(secondIDs, id)
		}
	}
	require.NoError(t, got.Err())
	require.Equal(t, []int64{1, 2, 3, 4, 5}, firstIDs)
	require.Equal(t, []int64{1}, secondIDs)
}
