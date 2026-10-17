package main

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
)

// schemaStatements splits the DDL file into executable statements, dropping comments.
func schemaStatements(path string) ([]string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var lines []string
	for _, line := range strings.Split(string(raw), "\n") {
		if t := strings.TrimSpace(line); t == "" || strings.HasPrefix(t, "--") {
			continue
		}
		lines = append(lines, line)
	}
	var out []string
	for _, stmt := range strings.Split(strings.Join(lines, "\n"), ";") {
		if s := strings.TrimSpace(stmt); s != "" {
			out = append(out, s)
		}
	}
	return out, nil
}

func isIndex(stmt string) bool { return strings.HasPrefix(strings.ToUpper(stmt), "CREATE INDEX") }

func indexName(stmt string) string {
	f := strings.Fields(stmt)
	if len(f) >= 3 {
		return f[2]
	}
	return stmt
}

func writeMeta(ctx context.Context, conn *pgx.Conn, d *Dataset) error {
	if _, err := conn.Exec(ctx, `CREATE TABLE IF NOT EXISTS bench_meta (key text PRIMARY KEY, value text NOT NULL)`); err != nil {
		return err
	}
	for k, v := range map[string]string{
		"namespaces": strconv.Itoa(d.Namespaces),
		"rows":       strconv.FormatInt(d.Rows, 10),
		"heavy_rows": strconv.FormatInt(d.HeavyRows, 10),
		"base":       d.Base.Format(time.RFC3339Nano),
	} {
		if _, err := conn.Exec(ctx, `INSERT INTO bench_meta VALUES ($1, $2) ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value`, k, v); err != nil {
			return err
		}
	}
	return nil
}

// readMeta restores the dataset a previous load wrote, so later phases regenerate identical rows.
func readMeta(ctx context.Context, conn *pgx.Conn) (*Dataset, error) {
	rows, err := conn.Query(ctx, `SELECT key, value FROM bench_meta`)
	if err != nil {
		return nil, err
	}
	meta, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) ([2]string, error) {
		var kv [2]string
		err := r.Scan(&kv[0], &kv[1])
		return kv, err
	})
	if err != nil {
		return nil, err
	}
	d := &Dataset{}
	for _, kv := range meta {
		switch kv[0] {
		case "namespaces":
			d.Namespaces, err = strconv.Atoi(kv[1])
		case "rows":
			d.Rows, err = strconv.ParseInt(kv[1], 10, 64)
		case "heavy_rows":
			d.HeavyRows, err = strconv.ParseInt(kv[1], 10, 64)
		case "base":
			d.Base, err = time.Parse(time.RFC3339Nano, kv[1])
		}
		if err != nil {
			return nil, fmt.Errorf("bench_meta %s: %w", kv[0], err)
		}
	}
	if d.Namespaces == 0 || d.Rows == 0 {
		return nil, fmt.Errorf("bench_meta is incomplete: %+v", d)
	}
	return d, nil
}

// load creates the tables, bulk-loads the dataset with parallel COPY, then builds the indexes so
// each build is timed separately.
func load(ctx context.Context, dsn, schemaPath string, d *Dataset, parallel int) (map[string]any, error) {
	stmts, err := schemaStatements(schemaPath)
	if err != nil {
		return nil, err
	}
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		return nil, err
	}
	defer closeConn(conn)
	for _, s := range stmts {
		if !isIndex(s) {
			if _, err := conn.Exec(ctx, s); err != nil {
				return nil, fmt.Errorf("%s: %w", firstLine(s), err)
			}
		}
	}
	if err := writeMeta(ctx, conn, d); err != nil {
		return nil, err
	}

	started := time.Now()
	var wg sync.WaitGroup
	errs := make(chan error, parallel)
	total := d.Total()
	for p := 0; p < parallel; p++ {
		lo, hi := total*int64(p)/int64(parallel), total*int64(p+1)/int64(parallel)
		wg.Add(1)
		go func() {
			defer wg.Done()
			c, err := pgx.Connect(ctx, dsn)
			if err != nil {
				errs <- err
				return
			}
			defer closeConn(c)
			g := lo
			_, err = c.CopyFrom(ctx, pgx.Identifier{"executions_visibility"}, writeColumns, pgx.CopyFromFunc(func() ([]any, error) {
				if g >= hi {
					return nil, nil
				}
				r := d.Row(g)
				g++
				return r.Values(), nil
			}))
			if err != nil {
				errs <- err
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		return nil, err
	}
	result := map[string]any{"rows": total, "copy_seconds": time.Since(started).Seconds()}

	builds := map[string]float64{}
	for _, s := range stmts {
		if isIndex(s) {
			t := time.Now()
			if _, err := conn.Exec(ctx, s); err != nil {
				return nil, fmt.Errorf("%s: %w", indexName(s), err)
			}
			builds[indexName(s)] = time.Since(t).Seconds()
		}
	}
	result["index_build_seconds"] = builds
	t := time.Now()
	if _, err := conn.Exec(ctx, `VACUUM ANALYZE executions_visibility`); err != nil {
		return nil, err
	}
	if _, err := conn.Exec(ctx, `CHECKPOINT`); err != nil {
		return nil, err
	}
	result["vacuum_seconds"] = time.Since(t).Seconds()
	return result, nil
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// sizes reports heap, per-index sizes and the BM25 segment count.
func sizes(ctx context.Context, conn *pgx.Conn) (map[string]any, error) {
	out := map[string]any{}
	var heap, toast int64
	if err := conn.QueryRow(ctx, `SELECT pg_relation_size('executions_visibility'),
		coalesce(pg_total_relation_size(reltoastrelid), 0)
		FROM pg_class WHERE relname = 'executions_visibility'`).Scan(&heap, &toast); err != nil {
		return nil, err
	}
	out["heap_bytes"], out["toast_bytes"] = heap, toast
	rows, err := conn.Query(ctx, `SELECT indexrelname, pg_relation_size(indexrelid) FROM pg_stat_user_indexes
		WHERE relname = 'executions_visibility' ORDER BY 1`)
	if err != nil {
		return nil, err
	}
	idx := map[string]int64{}
	for rows.Next() {
		var name string
		var size int64
		if err := rows.Scan(&name, &size); err != nil {
			rows.Close()
			return nil, err
		}
		idx[name] = size
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out["index_bytes"] = idx
	var segments int64
	if err := conn.QueryRow(ctx, `SELECT count(*) FROM paradedb.index_info('visibility_core_bm25')`).Scan(&segments); err != nil {
		return nil, err
	}
	out["bm25_segments"] = segments
	var live int64
	if err := conn.QueryRow(ctx, `SELECT n_live_tup FROM pg_stat_user_tables WHERE relname = 'executions_visibility'`).Scan(&live); err != nil {
		return nil, err
	}
	out["live_rows_estimate"] = live
	return out, nil
}

func closeConn(conn *pgx.Conn) {
	if err := conn.Close(context.Background()); err != nil {
		fmt.Fprintln(os.Stderr, "close connection:", err)
	}
}
