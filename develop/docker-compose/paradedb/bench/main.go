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
var jsonInsertSQL, jsonUpsertSQL string
var jsonFields = []string{
	"namespace_id", "run_id", "workflow_type_name", "workflow_id", "start_time", "execution_time",
	"status", "close_time", "history_length", "history_size_bytes", "execution_duration",
	"state_transition_count", "memo", "encoding", "task_queue", "sa_keyword", "sa_text",
	"sa_number", "sa_datetime", "sa_bool", "_version",
}

func init() {
	insertSQL, upsertSQL = statements(fields)
	jsonInsertSQL, jsonUpsertSQL = statements(jsonFields)
}

func statements(fields []string) (string, string) {
	params := make([]string, len(fields))
	assignments := make([]string, len(fields))
	for i, field := range fields {
		params[i] = fmt.Sprintf("$%d", i+1)
		assignments[i] = field + " = excluded." + field
	}
	prefix := "INSERT INTO executions_visibility (" + strings.Join(fields, ", ") + ") VALUES (" + strings.Join(params, ", ") + ") "
	return prefix + "ON CONFLICT (namespace_id, run_id) DO NOTHING", prefix + "ON CONFLICT (namespace_id, run_id) DO UPDATE SET " + strings.Join(assignments, ", ") + " WHERE executions_visibility._version < EXCLUDED._version"
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

// attr drives every filterable value except the namespace (id % 10), so each namespace sees the same selectivity.
func attr(id int) int { return id / 10 }

func coreValues(id int, running bool) []any {
	ns, run := key(id)
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC).Add(time.Duration(id%86400) * time.Second)
	var closeTime any
	status := 1
	if !running {
		status = 2 + id%6
		closeTime = start.Add(time.Minute)
	}
	return []any{ns, run, "type-" + fmt.Sprint(attr(id)%10), "workflow-" + fmt.Sprint(id), start, start,
		status, closeTime, id % 10000, id % 100000, id % 3600, id % 100, nil,
		"json/plain", "queue-" + fmt.Sprint(attr(id)%10)}
}

func values(id int, version int64, running bool) []any {
	a := attr(id)
	attrs := fmt.Sprintf(`{"Keyword01":"tenant-%d","Keyword02":"prefix-%d","KeywordList01":["tag-%d"],"Text01":"%s","Int01":%d,"Bool01":%t}`,
		a%100, a%1000, a%20, map[bool]string{true: "hello world", false: "ordinary document"}[a%10 == 0], a%1000, a%2 == 0)
	return append(coreValues(id, running), attrs, nil, nil, "", "", version)
}

func jsonValues(id int, version int64, running bool) []any {
	a := attr(id)
	return append(coreValues(id, running),
		fmt.Sprintf(`{"Keyword01":"tenant-%d","Keyword02":"prefix-%d","KeywordList01":["tag-%d"]}`, a%100, a%1000, a%20),
		map[bool]string{true: `{"Text01":"hello world"}`, false: `{"Text01":"ordinary document"}`}[a%10 == 0],
		fmt.Sprintf(`{"Int01":%d}`, a%1000), `{}`, fmt.Sprintf(`{"Bool01":%t}`, a%2 == 0), version)
}

func connect(ctx context.Context, dsn, variant string) (*pgx.Conn, error) {
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		return nil, err
	}
	if variant == "pdb-json-hybrid" {
		if _, err := conn.Exec(ctx, "SET plan_cache_mode=force_custom_plan"); err != nil {
			closeConn(conn)
			return nil, err
		}
	}
	return conn, nil
}

func closeConn(conn *pgx.Conn) {
	if err := conn.Close(context.Background()); err != nil {
		fmt.Fprintln(os.Stderr, "close connection:", err)
	}
}

func preload(ctx context.Context, dsn string, rows int, variant string) error {
	conn, err := connect(ctx, dsn, variant)
	if err != nil {
		return err
	}
	defer closeConn(conn)
	// Server-side generation keeps preload network cost out of the measured single-row workload.
	selectedFields := fields
	attributes := `jsonb_build_object('Keyword01', 'tenant-' || (g / 10 % 100), 'Keyword02', 'prefix-' || (g / 10 % 1000),
	 'KeywordList01', jsonb_build_array('tag-' || (g / 10 % 20)), 'Text01', CASE WHEN g / 10 % 10 = 0 THEN 'hello world' ELSE 'ordinary document' END,
	 'Int01', g / 10 % 1000, 'Bool01', g / 10 % 2 = 0), NULL, NULL, '', '', 1`
	if variant == "pdb-json-hybrid" {
		selectedFields = jsonFields
		attributes = `jsonb_build_object('Keyword01', 'tenant-' || (g / 10 % 100), 'Keyword02', 'prefix-' || (g / 10 % 1000),
	 'KeywordList01', jsonb_build_array('tag-' || (g / 10 % 20))),
	 jsonb_build_object('Text01', CASE WHEN g / 10 % 10 = 0 THEN 'hello world' ELSE 'ordinary document' END),
	 jsonb_build_object('Int01', g / 10 % 1000), '{}'::jsonb, jsonb_build_object('Bool01', g / 10 % 2 = 0), 1`
	}
	_, err = conn.Exec(ctx, `INSERT INTO executions_visibility (`+strings.Join(selectedFields, ", ")+`)
	SELECT md5((g % 10)::text), md5(g::text), 'type-' || (g / 10 % 10), 'workflow-' || g,
	 timestamp '2026-01-01' + (g % 86400) * interval '1 second', timestamp '2026-01-01' + (g % 86400) * interval '1 second',
	 CASE WHEN g % 7 = 0 THEN 1 ELSE 2 + g % 6 END,
	 CASE WHEN g % 7 = 0 THEN NULL ELSE timestamp '2026-01-01' + (g % 86400) * interval '1 second' + interval '1 minute' END,
	 g % 10000, g % 100000, g % 3600, g % 100, NULL, 'json/plain', 'queue-' || (g / 10 % 10),
	 `+attributes+`
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

func querySQL(q query, variant string) string {
	switch variant {
	case "postgres":
		return q.postgres
	case "pdb-cols":
		return q.parade
	case "pdb-cols-hybrid", "pdb-json-hybrid":
		if q.name == "status_time" || q.name == "type_status" || q.name == "count" || q.name == "group_count" {
			return q.postgres
		}
		if variant == "pdb-cols-hybrid" {
			return q.parade
		}
		switch q.name {
		case "text":
			return strings.Replace(q.parade, "Text01 ||| 'hello'", "sa_text->'Text01' ||| 'hello'", 1)
		case "keyword":
			return strings.Replace(q.parade, "Keyword01 IN ('tenant-0', 'tenant-20')", "sa_keyword->'Keyword01' @@@ pdb.term_set(ARRAY['tenant-0', 'tenant-20'])", 1)
		case "keyword_list":
			return strings.Replace(q.parade, "KeywordList01 @@@ pdb.term('tag-0')", "sa_keyword->'KeywordList01' @@@ pdb.term('tag-0')", 1)
		}
	}
	return ""
}

func readOnce(ctx context.Context, conn *pgx.Conn, sql string, n int) error {
	ns, _ := key(n % 10)
	rows, err := conn.Query(ctx, sql, ns)
	if err != nil {
		return err
	}
	for rows.Next() { /* count and list rows are drained for comparable work */
	}
	err = rows.Err()
	rows.Close()
	return err
}

func run(ctx context.Context, dsn, variant string, preloadRows, writers, readers int, duration time.Duration, writeOp, readQuery string, readRate float64) (map[string]any, error) {
	if writeOp != "mixed" && writeOp != "insert" && writeOp != "upsert" && writeOp != "delete" {
		return nil, fmt.Errorf("unknown write operation %q", writeOp)
	}
	selectedQueries := queries
	if readQuery != "all" {
		selectedQueries = nil
		for _, q := range queries {
			if q.name == readQuery {
				selectedQueries = []query{q}
				break
			}
		}
		if len(selectedQueries) == 0 {
			return nil, fmt.Errorf("unknown read query %q", readQuery)
		}
	}
	var write sample
	var affected atomic.Int64
	var inserts, updates, deletes atomic.Int64
	read := make([]*sample, len(selectedQueries))
	for i := range read {
		read[i] = &sample{}
	}
	missed := make([]atomic.Int64, len(selectedQueries))
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
			conn, err := connect(ctx, dsn, variant)
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
				insert, upsert := insertSQL, upsertSQL
				if variant == "pdb-json-hybrid" {
					insert, upsert = jsonInsertSQL, jsonUpsertSQL
				}
				switch {
				case writeOp == "insert" || (writeOp == "mixed" && op < 3):
					inserts.Add(1)
					id = int(next.Add(1))
					var row []any
					if variant == "pdb-json-hybrid" {
						row = jsonValues(id, 1, true)
					} else {
						row = values(id, 1, true)
					}
					tag, err = conn.Exec(runCtx, insert, row...)
				case writeOp == "upsert" || (writeOp == "mixed" && op < 9):
					updates.Add(1)
					version := time.Now().UnixNano()
					var row []any
					if variant == "pdb-json-hybrid" {
						row = jsonValues(id, version, false)
					} else {
						row = values(id, version, false)
					}
					tag, err = conn.Exec(runCtx, upsert, row...)
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
	if readRate > 0 {
		// A fixed offered rate per query keeps the read pressure on writers identical across variants,
		// and stops the slowest query from pacing the throughput of the others.
		type job struct{ query, n int }
		jobs := make(chan job, readers)
		var schedulers sync.WaitGroup
		for i := range selectedQueries {
			schedulers.Add(1)
			go func(i int) {
				defer schedulers.Done()
				ticker := time.NewTicker(time.Duration(float64(time.Second) / readRate))
				defer ticker.Stop()
				for n := 0; ; n++ {
					select {
					case <-runCtx.Done():
						return
					case <-ticker.C:
					}
					select {
					case jobs <- job{i, n}:
					default:
						missed[i].Add(1)
					}
				}
			}(i)
		}
		go func() {
			schedulers.Wait()
			close(jobs)
		}()
		for r := 0; r < readers; r++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				conn, err := connect(ctx, dsn, variant)
				if err != nil {
					read[0].add(time.Now(), err)
					for j := range jobs {
						missed[j.query].Add(1)
					}
					return
				}
				defer closeConn(conn)
				for j := range jobs {
					if runCtx.Err() != nil || conn.IsClosed() {
						continue
					}
					begun := time.Now()
					err := readOnce(runCtx, conn, querySQL(selectedQueries[j.query], variant), j.n)
					if runCtx.Err() == nil {
						read[j.query].add(begun, err)
					}
				}
			}()
		}
	} else {
		for r := 0; r < readers; r++ {
			wg.Add(1)
			go func(reader int) {
				defer wg.Done()
				conn, err := connect(ctx, dsn, variant)
				if err != nil {
					read[0].add(time.Now(), err)
					return
				}
				defer closeConn(conn)
				for n := 0; runCtx.Err() == nil; n++ {
					i := (n + reader) % len(selectedQueries)
					begun := time.Now()
					err := readOnce(runCtx, conn, querySQL(selectedQueries[i], variant), n)
					if runCtx.Err() == nil {
						read[i].add(begun, err)
					}
					if conn.IsClosed() {
						return
					}
				}
			}(r)
		}
	}
	wg.Wait()
	seconds := time.Since(start).Seconds()
	result := map[string]any{"seconds": seconds, "writers": writers, "readers": readers, "read_rate": readRate, "write": write.result(seconds), "affected_rows": affected.Load(), "attempts": map[string]int64{"insert": inserts.Load(), "update": updates.Load(), "delete": deletes.Load()}, "reads": map[string]any{}}
	for i, q := range selectedQueries {
		r := read[i].result(seconds)
		r["missed"] = missed[i].Load()
		result["reads"].(map[string]any)[q.name] = r
	}
	if write.errors.Load() != 0 {
		return result, fmt.Errorf("write workload had %d errors", write.errors.Load())
	}
	for i, q := range selectedQueries {
		if read[i].errors.Load() != 0 {
			return result, fmt.Errorf("%s query had %d errors", q.name, read[i].errors.Load())
		}
	}
	return result, nil
}

func verify(ctx context.Context, dsn string) (map[string]any, error) {
	conn, err := connect(ctx, dsn, "pdb-cols")
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

func check(ctx context.Context, dsn, variant string) (map[string]any, error) {
	conn, err := connect(ctx, dsn, variant)
	if err != nil {
		return nil, err
	}
	defer closeConn(conn)
	results := make(map[string][]string)
	plans := make(map[string]string)
	for _, q := range queries {
		sql := querySQL(q, variant)
		for n := 0; n < 10; n++ {
			values, err := checkQuery(ctx, conn, sql, n)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", q.name, err)
			}
			if q.name == "group_count" {
				sort.Strings(values)
			}
			results[fmt.Sprintf("%s/ns%d", q.name, n)] = values
		}
		ns, _ := key(0)
		planRows, err := conn.Query(ctx, "EXPLAIN "+sql, ns)
		if err != nil {
			return nil, fmt.Errorf("EXPLAIN %s: %w", q.name, err)
		}
		var lines []string
		for planRows.Next() {
			var line string
			if err := planRows.Scan(&line); err != nil {
				planRows.Close()
				return nil, err
			}
			lines = append(lines, line)
		}
		err = planRows.Err()
		planRows.Close()
		if err != nil {
			return nil, fmt.Errorf("EXPLAIN %s: %w", q.name, err)
		}
		plans[q.name] = strings.Join(lines, "\n")
	}
	return map[string]any{"results": results, "plans": plans}, nil
}

func checkQuery(ctx context.Context, conn *pgx.Conn, sql string, n int) ([]string, error) {
	ns, _ := key(n)
	rows, err := conn.Query(ctx, sql, ns)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := []string{}
	for rows.Next() {
		columns, err := rows.Values()
		if err != nil {
			return nil, err
		}
		parts := make([]string, len(columns))
		for i, column := range columns {
			parts[i] = strings.TrimSpace(fmt.Sprint(column))
		}
		values = append(values, strings.Join(parts, "|"))
	}
	return values, rows.Err()
}

func main() {
	mode := flag.String("mode", "run", "preload, run, check, or verify")
	dsn := flag.String("dsn", "postgres://postgres:spike@127.0.0.1:56432/visibility?sslmode=disable", "database URL")
	parade := flag.Bool("paradedb", true, "use ParadeDB read queries")
	variantFlag := flag.String("variant", "", "postgres, pdb-cols, pdb-cols-hybrid, or pdb-json-hybrid")
	rows := flag.Int("rows", 1000000, "preload rows")
	writers := flag.Int("writers", 64, "concurrent writers")
	readers := flag.Int("readers", 8, "concurrent readers")
	duration := flag.Duration("duration", 15*time.Minute, "write duration")
	allowErrors := flag.Bool("allow-errors", false, "return success despite workload errors (crash probe only)")
	writeOp := flag.String("write-op", "mixed", "mixed, insert, upsert, or delete")
	readRate := flag.Float64("read-rate", 0, "offered reads per second for each query; 0 runs readers in a closed loop")
	readQuery := flag.String("read-query", "all", "all, status_time, type_status, text, keyword, keyword_list, count, or group_count")
	flag.Parse()
	variant := *variantFlag
	if variant == "" {
		variant = "postgres"
		if *parade {
			variant = "pdb-cols"
		}
	}
	if variant != "postgres" && variant != "pdb-cols" && variant != "pdb-cols-hybrid" && variant != "pdb-json-hybrid" {
		fmt.Fprintln(os.Stderr, "unknown variant:", variant)
		os.Exit(1)
	}
	ctx := context.Background()
	var result any
	var err error
	switch *mode {
	case "preload":
		err = preload(ctx, *dsn, *rows, variant)
	case "run":
		result, err = run(ctx, *dsn, variant, *rows, *writers, *readers, *duration, *writeOp, *readQuery, *readRate)
		if *allowErrors {
			err = nil
		}
	case "verify":
		result, err = verify(ctx, *dsn)
	case "check":
		result, err = check(ctx, *dsn, variant)
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
