package limitless

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/superdurable/xtemporal/common/persistence/sql/sqlplugin"
)

type contextExecutor interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

func insertIntoBufferedEvents(ctx context.Context, inner any, rows []sqlplugin.BufferedEventsRow) (sql.Result, error) {
	if len(rows) == 0 {
		return nil, errors.New("buffered events insert requires at least one row")
	}
	first := rows[0]
	for _, row := range rows[1:] {
		if row.ShardID != first.ShardID || !bytes.Equal(row.NamespaceID, first.NamespaceID) || row.WorkflowID != first.WorkflowID || !bytes.Equal(row.RunID, first.RunID) {
			return nil, errors.New("buffered events rows must belong to one workflow")
		}
	}
	executor, ok := inner.(contextExecutor)
	if !ok {
		return nil, errors.New("postgres12_pgx DB does not implement ExecContext")
	}

	var query strings.Builder
	query.WriteString("INSERT INTO buffered_events (shard_id, namespace_id, workflow_id, run_id, id, data, data_encoding) ")
	query.WriteString("SELECT $1::integer, $2::bytea, $3::varchar(255), $4::bytea, (SELECT COALESCE(MAX(id), 0) FROM buffered_events ")
	query.WriteString("WHERE shard_id = $1::integer AND namespace_id = $2::bytea AND workflow_id = $3::varchar(255) AND run_id = $4::bytea) + v.n, ")
	query.WriteString("v.data, v.data_encoding FROM (VALUES ")
	args := make([]any, 0, 4+3*len(rows))
	args = append(args, first.ShardID, first.NamespaceID, first.WorkflowID, first.RunID)
	for i, row := range rows {
		if i > 0 {
			query.WriteString(", ")
		}
		n := 5 + 3*i
		fmt.Fprintf(&query, "($%d::bigint, $%d::bytea, $%d::text)", n, n+1, n+2)
		args = append(args, int64(i+1), row.Data, row.DataEncoding)
	}
	query.WriteString(") AS v(n, data, data_encoding)")
	// The caller holds the execution row FOR UPDATE lock in this transaction, serializing IDs for one workflow.
	return executor.ExecContext(ctx, query.String(), args...)
}
