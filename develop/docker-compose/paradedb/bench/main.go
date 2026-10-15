package main

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"math/rand"
	"os"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// The field order and conflict clauses mirror postgresql/visibility.go and sqlplugin.VisibilityRow.
var fields = []string{
	"namespace_id", "run_id", "workflow_type_name", "workflow_id", "start_time", "execution_time",
	"status", "close_time", "history_length", "history_size_bytes", "execution_duration",
	"state_transition_count", "memo", "encoding", "task_queue", "search_attributes",
	"parent_workflow_id", "parent_run_id", "root_workflow_id", "root_run_id", "_version",
}

var insertSQL, upsertSQL string

func init() {
	params := make([]string, len(fields))
	assignments := make([]string, len(fields))
	for i, field := range fields {
		params[i] = fmt.Sprintf("$%d", i+1)
		assignments[i] = field + " = excluded." + field
	}
	prefix := "INSERT INTO executions_visibility (" + strings.Join(fields, ", ") + ") VALUES (" + strings.Join(params, ", ") + ") "
	insertSQL = prefix + "ON CONFLICT (namespace_id, run_id) DO NOTHING"
	upsertSQL = prefix + "ON CONFLICT (namespace_id, run_id) DO UPDATE SET " + strings.Join(assignments, ", ") + " WHERE executions_visibility._version < EXCLUDED._version"
}

const deleteSQL = "DELETE FROM executions_visibility WHERE namespace_id = $1 AND run_id = $2"

type sample struct {
	mu       sync.Mutex
	latency  []float64
	count    atomic.Int64
	errors   atomic.Int64
	firstErr error
}

func (s *sample) add(start time.Time, err error) {
	if err != nil {
		s.errors.Add(1)
		s.mu.Lock()
		if s.firstErr == nil {
			s.firstErr = err
		}
		s.mu.Unlock()
		return
	}
	s.count.Add(1)
	s.mu.Lock()
	s.latency = append(s.latency, float64(time.Since(start).Microseconds())/1000)
	s.mu.Unlock()
}

func (s *sample) result(seconds float64) map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	sort.Float64s(s.latency)
	result := map[string]any{"count": s.count.Load(), "errors": s.errors.Load(), "per_second": float64(s.count.Load()) / seconds}
	if s.firstErr != nil {
		result["first_error"] = s.firstErr.Error()
	}
	if len(s.latency) > 0 {
		for _, p := range []int{50, 95, 99} {
			result[fmt.Sprintf("p%d_ms", p)] = s.latency[(len(s.latency)-1)*p/100]
		}
	}
	return result
}

func key(id int) (string, string) {
	ns := md5.Sum([]byte(fmt.Sprint(id % 10)))
	run := md5.Sum([]byte(fmt.Sprint(id)))
	return hex.EncodeToString(ns[:]), hex.EncodeToString(run[:])
}

func values(id int, version int64, running bool) []any {
	ns, run := key(id)
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC).Add(time.Duration(id%86400) * time.Second)
	var closeTime any
	status := 1
	if !running {
		status = 2 + id%6
		closeTime = start.Add(time.Minute)
	}
	attrs := fmt.Sprintf(`{"Keyword01":"tenant-%d","Keyword02":"prefix-%d","KeywordList01":["tag-%d"],"Text01":"%s","Int01":%d,"Bool01":%t}`,
		id%100, id%1000, id%20, map[bool]string{true: "hello world", false: "ordinary document"}[id%10 == 0], id%1000, id%2 == 0)
	return []any{ns, run, "type-" + fmt.Sprint(id%10), "workflow-" + fmt.Sprint(id), start, start,
		status, closeTime, id % 10000, id % 100000, id % 3600, id % 100, nil,
		"json/plain", "queue-" + fmt.Sprint(id%10), attrs, nil, nil, "", "", version}
}

func connect(ctx context.Context, dsn string) (*pgx.Conn, error) {
	return pgx.Connect(ctx, dsn)
}

func closeConn(conn *pgx.Conn) {
	if err := conn.Close(context.Background()); err != nil {
		fmt.Fprintln(os.Stderr, "close connection:", err)
	}
}

func preload(ctx context.Context, dsn string, rows int) error {
	conn, err := connect(ctx, dsn)
	if err != nil {
		return err
	}
	defer closeConn(conn)
	// Server-side generation keeps preload network cost out of the measured single-row workload.
	_, err = conn.Exec(ctx, `INSERT INTO executions_visibility (`+strings.Join(fields, ", ")+`)
	SELECT md5((g % 10)::text), md5(g::text), 'type-' || (g % 10), 'workflow-' || g,
	 timestamp '2026-01-01' + (g % 86400) * interval '1 second', timestamp '2026-01-01' + (g % 86400) * interval '1 second',
	 CASE WHEN g % 7 = 0 THEN 1 ELSE 2 + g % 6 END,
	 CASE WHEN g % 7 = 0 THEN NULL ELSE timestamp '2026-01-01' + (g % 86400) * interval '1 second' + interval '1 minute' END,
	 g % 10000, g % 100000, g % 3600, g % 100, NULL, 'json/plain', 'queue-' || (g % 10),
	 jsonb_build_object('Keyword01', 'tenant-' || (g % 100), 'Keyword02', 'prefix-' || (g % 1000),
	 'KeywordList01', jsonb_build_array('tag-' || (g % 20)), 'Text01', CASE WHEN g % 10 = 0 THEN 'hello world' ELSE 'ordinary document' END,
	 'Int01', g % 1000, 'Bool01', g % 2 = 0), NULL, NULL, '', '', 1
	FROM generate_series(1, $1) g`, rows)
	if err != nil {
		return err
	}
	_, err = conn.Exec(ctx, "ANALYZE executions_visibility")
	return err
}

type query struct{ name, postgres, parade string }

var queries = []query{
	{"status_time", `SELECT run_id FROM executions_visibility WHERE namespace_id = $1 AND status = 1 AND start_time >= '2026-01-01' AND start_time < '2026-01-02' ORDER BY coalesce(close_time, '9999-12-31 23:59:59') DESC, start_time DESC, run_id LIMIT 20`, `SELECT run_id FROM executions_visibility WHERE namespace_id @@@ $1 AND status = 1 AND start_time >= '2026-01-01' AND start_time < '2026-01-02' ORDER BY sort_close_time DESC, start_time DESC, run_id COLLATE "C" LIMIT 20`},
	{"type_status", `SELECT run_id FROM executions_visibility WHERE namespace_id = $1 AND workflow_type_name = 'type-0' AND status = 1 ORDER BY coalesce(close_time, '9999-12-31 23:59:59') DESC, start_time DESC, run_id LIMIT 20`, `SELECT run_id FROM executions_visibility WHERE namespace_id @@@ $1 AND workflow_type_name = 'type-0' AND status = 1 ORDER BY sort_close_time DESC, start_time DESC, run_id COLLATE "C" LIMIT 20`},
	{"text", `SELECT run_id FROM executions_visibility WHERE namespace_id = $1 AND Text01 @@ 'hello'::tsquery ORDER BY coalesce(close_time, '9999-12-31 23:59:59') DESC, start_time DESC, run_id LIMIT 20`, `SELECT run_id FROM executions_visibility WHERE namespace_id @@@ $1 AND Text01 ||| 'hello' ORDER BY sort_close_time DESC, start_time DESC, run_id COLLATE "C" LIMIT 20`},
	{"keyword", `SELECT run_id FROM executions_visibility WHERE namespace_id = $1 AND Keyword01 IN ('tenant-0', 'tenant-20') ORDER BY coalesce(close_time, '9999-12-31 23:59:59') DESC, start_time DESC, run_id LIMIT 20`, `SELECT run_id FROM executions_visibility WHERE namespace_id @@@ $1 AND Keyword01 IN ('tenant-0', 'tenant-20') ORDER BY sort_close_time DESC, start_time DESC, run_id COLLATE "C" LIMIT 20`},
	{"keyword_list", `SELECT run_id FROM executions_visibility WHERE namespace_id = $1 AND KeywordList01 @> '["tag-0"]'::jsonb ORDER BY coalesce(close_time, '9999-12-31 23:59:59') DESC, start_time DESC, run_id LIMIT 20`, `SELECT run_id FROM executions_visibility WHERE namespace_id @@@ $1 AND KeywordList01 @@@ pdb.term('tag-0') ORDER BY sort_close_time DESC, start_time DESC, run_id COLLATE "C" LIMIT 20`},
	{"count", `SELECT count(*) FROM executions_visibility WHERE namespace_id = $1 AND status = 1`, `SELECT count(*) FROM executions_visibility WHERE namespace_id @@@ $1 AND status = 1`},
	{"group_count", `SELECT status, count(*) FROM executions_visibility WHERE namespace_id = $1 GROUP BY status`, `SELECT status, count(*) FROM executions_visibility WHERE namespace_id @@@ $1 GROUP BY status`},
}

func run(ctx context.Context, dsn string, parade bool, preloadRows, writers, readers int, duration time.Duration) (map[string]any, error) {
	var write sample
	var affected atomic.Int64
	var inserts, updates, deletes atomic.Int64
	read := make([]*sample, len(queries))
	for i := range read {
		read[i] = &sample{}
	}
	var wg sync.WaitGroup
	runCtx, cancel := context.WithTimeout(ctx, duration)
	defer cancel()
	var next atomic.Int64
	next.Store(int64(preloadRows))
	start := time.Now()
	for w := 0; w < writers; w++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			conn, err := connect(ctx, dsn)
			if err != nil {
				write.add(time.Now(), err)
				return
			}
			defer closeConn(conn)
			rng := rand.New(rand.NewSource(int64(worker) + 12345))
			for runCtx.Err() == nil {
				op := rng.Intn(10)
				id := rng.Intn(preloadRows) + 1
				begun := time.Now()
				var err error
				var tag pgconn.CommandTag
				switch {
				case op < 3:
					inserts.Add(1)
					id = int(next.Add(1))
					tag, err = conn.Exec(runCtx, insertSQL, values(id, 1, true)...)
				case op < 9:
					updates.Add(1)
					tag, err = conn.Exec(runCtx, upsertSQL, values(id, time.Now().UnixNano(), false)...)
				default:
					deletes.Add(1)
					ns, runID := key(id)
					tag, err = conn.Exec(runCtx, deleteSQL, ns, runID)
				}
				if err == nil {
					affected.Add(tag.RowsAffected())
				}
				if runCtx.Err() == nil {
					write.add(begun, err)
				}
				if conn.IsClosed() {
					return
				}
			}
		}(w)
	}
	for r := 0; r < readers; r++ {
		wg.Add(1)
		go func(reader int) {
			defer wg.Done()
			conn, err := connect(ctx, dsn)
			if err != nil {
				read[0].add(time.Now(), err)
				return
			}
			defer closeConn(conn)
			for n := 0; runCtx.Err() == nil; n++ {
				i := (n + reader) % len(queries)
				q := queries[i].postgres
				if parade {
					q = queries[i].parade
				}
				ns, _ := key(n % 10)
				begun := time.Now()
				rows, err := conn.Query(runCtx, q, ns)
				if err == nil {
					for rows.Next() { /* count and list rows are drained for comparable work */
					}
					err = rows.Err()
					rows.Close()
				}
				if runCtx.Err() == nil {
					read[i].add(begun, err)
				}
				if conn.IsClosed() {
					return
				}
			}
		}(r)
	}
	wg.Wait()
	seconds := time.Since(start).Seconds()
	result := map[string]any{"seconds": seconds, "writers": writers, "readers": readers, "write": write.result(seconds), "affected_rows": affected.Load(), "attempts": map[string]int64{"insert": inserts.Load(), "update": updates.Load(), "delete": deletes.Load()}, "reads": map[string]any{}}
	for i, q := range queries {
		result["reads"].(map[string]any)[q.name] = read[i].result(seconds)
	}
	if write.errors.Load() != 0 {
		return result, fmt.Errorf("write workload had %d errors", write.errors.Load())
	}
	for i, q := range queries {
		if read[i].errors.Load() != 0 {
			return result, fmt.Errorf("%s query had %d errors", q.name, read[i].errors.Load())
		}
	}
	return result, nil
}

func verify(ctx context.Context, dsn string) (map[string]any, error) {
	conn, err := connect(ctx, dsn)
	if err != nil {
		return nil, err
	}
	defer closeConn(conn)
	checks := []struct{ name, indexed, heap string }{
		{"namespace", "namespace_id @@@ $1", "namespace_id = $1"},
		{"status", "namespace_id @@@ $1 AND status = 1", "namespace_id = $1 AND status = 1"},
		{"workflow_type", "namespace_id @@@ $1 AND workflow_type_name = 'type-0'", "namespace_id = $1 AND workflow_type_name = 'type-0'"},
		{"keyword", "namespace_id @@@ $1 AND Keyword01 = 'tenant-0'", "namespace_id = $1 AND Keyword01 = 'tenant-0'"},
	}
	out := map[string]any{}
	for _, check := range checks {
		ns, _ := key(0)
		var indexed, heap int64
		if err := conn.QueryRow(ctx, "SELECT count(*) FROM executions_visibility WHERE "+check.indexed, ns).Scan(&indexed); err != nil {
			return nil, err
		}
		if err := conn.QueryRow(ctx, "WITH heap AS MATERIALIZED (SELECT namespace_id, status, workflow_type_name, Keyword01 FROM executions_visibility OFFSET 0) SELECT count(*) FROM heap WHERE "+check.heap, ns).Scan(&heap); err != nil {
			return nil, err
		}
		out[check.name] = map[string]int64{"index": indexed, "heap": heap}
		if indexed != heap {
			return out, fmt.Errorf("%s: index %d != heap %d", check.name, indexed, heap)
		}
	}
	return out, nil
}

func main() {
	mode := flag.String("mode", "run", "preload, run, or verify")
	dsn := flag.String("dsn", "postgres://postgres:spike@127.0.0.1:56432/visibility?sslmode=disable", "database URL")
	parade := flag.Bool("paradedb", true, "use ParadeDB read queries")
	rows := flag.Int("rows", 1000000, "preload rows")
	writers := flag.Int("writers", 64, "concurrent writers")
	readers := flag.Int("readers", 8, "concurrent readers")
	duration := flag.Duration("duration", 15*time.Minute, "write duration")
	allowErrors := flag.Bool("allow-errors", false, "return success despite workload errors (crash probe only)")
	flag.Parse()
	ctx := context.Background()
	var result any
	var err error
	switch *mode {
	case "preload":
		err = preload(ctx, *dsn, *rows)
	case "run":
		result, err = run(ctx, *dsn, *parade, *rows, *writers, *readers, *duration)
		if *allowErrors {
			err = nil
		}
	case "verify":
		result, err = verify(ctx, *dsn)
	default:
		err = errors.New("unknown mode")
	}
	if result != nil {
		if encodeErr := json.NewEncoder(os.Stdout).Encode(result); encodeErr != nil {
			fmt.Fprintln(os.Stderr, encodeErr)
			os.Exit(1)
		}
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
