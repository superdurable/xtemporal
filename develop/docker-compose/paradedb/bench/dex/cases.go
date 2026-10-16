package main

import (
	"context"
	"fmt"
	"math/rand"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
)

// literal renders a bind value for PREPARE/EXECUTE, which the generic-plan EXPLAIN needs.
func literal(v any) (string, string) {
	switch x := v.(type) {
	case string:
		return "'" + strings.ReplaceAll(x, "'", "''") + "'", "text"
	case []string:
		q := make([]string, len(x))
		for i, s := range x {
			q[i] = "'" + strings.ReplaceAll(s, "'", "''") + "'"
		}
		return "ARRAY[" + strings.Join(q, ",") + "]::text[]", "text[]"
	case time.Time:
		return "'" + x.UTC().Format("2006-01-02 15:04:05.999999") + "'::timestamp", "timestamp"
	case int:
		return fmt.Sprint(x), "bigint"
	case int64:
		return fmt.Sprint(x), "bigint"
	}
	panic(fmt.Sprintf("unsupported literal %T", v))
}

func explainText(ctx context.Context, conn *pgx.Conn, sql string, args []any) (string, error) {
	rows, err := conn.Query(ctx, "EXPLAIN (ANALYZE, BUFFERS) "+sql, args...)
	if err != nil {
		return "", err
	}
	lines, err := pgx.CollectRows(rows, pgx.RowTo[string])
	return strings.Join(lines, "\n"), err
}

// explainGeneric shows the plan a cached prepared statement settles on once PostgreSQL switches to a
// generic plan.
func explainGeneric(ctx context.Context, conn *pgx.Conn, sql string, args []any) (string, error) {
	types := make([]string, len(args))
	vals := make([]string, len(args))
	for i, a := range args {
		vals[i], types[i] = literal(a)
	}
	if _, err := conn.Exec(ctx, fmt.Sprintf("PREPARE gp(%s) AS %s", strings.Join(types, ","), sql)); err != nil {
		return "", err
	}
	defer func() { _, _ = conn.Exec(ctx, "DEALLOCATE gp") }()
	if _, err := conn.Exec(ctx, "SET plan_cache_mode = force_generic_plan"); err != nil {
		return "", err
	}
	defer func() { _, _ = conn.Exec(ctx, "RESET plan_cache_mode") }()
	rows, err := conn.Query(ctx, fmt.Sprintf("EXPLAIN (ANALYZE, BUFFERS) EXECUTE gp(%s)", strings.Join(vals, ",")), pgx.QueryExecModeSimpleProtocol)
	if err != nil {
		return "", err
	}
	lines, err := pgx.CollectRows(rows, pgx.RowTo[string])
	return strings.Join(lines, "\n"), err
}

func connectRoute(ctx context.Context, dsn string, r route) (*pgx.Conn, error) {
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		return nil, err
	}
	if r == routeBM25 {
		if _, err := conn.Exec(ctx, "SET plan_cache_mode = force_custom_plan"); err != nil {
			closeConn(conn)
			return nil, err
		}
	}
	return conn, nil
}

type explainTarget struct {
	label string
	ns    int
}

func (d *Dataset) explainTargets() []explainTarget {
	t := []explainTarget{{"regular", 1}}
	if d.HeavyRows > 0 {
		t = append(t, explainTarget{"heavy", d.HeavyIndex()})
	}
	return t
}

func fixedArgs(d *Dataset, s Shape, ns int) []any {
	b := &browser{ns: ns, nsID: d.NamespaceIDByIndex(ns), perms: []string{"perm-ops", "perm-legal"},
		rng: rand.New(rand.NewSource(1)), rowsIn: d.RowsIn(ns)}
	if s.Name == "workflow_id" {
		return []any{b.nsID, b.workflowID(d)}
	}
	return b.args(s, "approval")
}

// explainAll records custom-plan EXPLAIN ANALYZE for every shape, plus the generic plan for the
// B-tree shapes (pgx caches prepared statements, so production may run the generic plan).
func explainAll(ctx context.Context, dsn string, d *Dataset) (map[string]any, error) {
	out := map[string]any{}
	for _, s := range AllShapes() {
		conn, err := connectRoute(ctx, dsn, s.Route)
		if err != nil {
			return nil, err
		}
		for _, t := range d.explainTargets() {
			args := fixedArgs(d, s, t.ns)
			plan, err := explainText(ctx, conn, s.SQL, args)
			if err != nil {
				closeConn(conn)
				return nil, fmt.Errorf("%s/%s: %w", s.Name, t.label, err)
			}
			entry := map[string]any{"route": string(s.Route), "intended": s.Intended, "custom": plan}
			if s.Route == routeBTree {
				if entry["generic"], err = explainGeneric(ctx, conn, s.SQL, args); err != nil {
					closeConn(conn)
					return nil, fmt.Errorf("%s/%s generic: %w", s.Name, t.label, err)
				}
			}
			out[s.Name+"/"+t.label] = entry
		}
		closeConn(conn)
	}
	return out, nil
}

func timeQuery(ctx context.Context, conn *pgx.Conn, sql string, args ...any) (float64, int, error) {
	t := time.Now()
	rows, err := conn.Query(ctx, sql, args...)
	if err != nil {
		return 0, 0, err
	}
	n, err := drain(rows)
	return float64(time.Since(t).Microseconds()) / 1000, n, err
}

func percentiles(v []float64) map[string]any {
	sort.Float64s(v)
	n := len(v)
	if n == 0 {
		return map[string]any{}
	}
	return map[string]any{"p50_ms": v[(n-1)*50/100], "p95_ms": v[(n-1)*95/100], "p99_ms": v[(n-1)*99/100], "max_ms": v[n-1], "runs": n}
}

// bounded runs the bounded B-tree scan and falls back to BM25 when the bound is hit without a full page.
func bounded(ctx context.Context, bt, bm *pgx.Conn, nsID, flow, value string, bound int) (float64, int, bool, error) {
	t := time.Now()
	rows, err := bt.Query(ctx, boundedSQL, nsID, flow, value, bound)
	if err != nil {
		return 0, 0, false, err
	}
	scanned, found := 0, 0
	for rows.Next() {
		var s int
		var run *string
		if err := rows.Scan(&s, &run); err != nil {
			rows.Close()
			return 0, 0, false, err
		}
		scanned = s
		if run != nil {
			found++
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, 0, false, err
	}
	if found >= 50 || scanned < bound {
		return float64(time.Since(t).Microseconds()) / 1000, found, false, nil
	}
	_, n, err := timeQuery(ctx, bm, worstBM25.SQL, nsID, flow, value)
	return float64(time.Since(t).Microseconds()) / 1000, n, true, err
}

type worstConfig struct {
	Reps    int
	Writers int
	Bounds  []int
	Flow    string
}

// worstCase measures FlowType = X (most rows) plus a rare or absent residual keyword, on the B-tree
// residual route, the BM25 route and a bounded B-tree scan with BM25 fallback, then on a promoted
// column. The promoted index is built only after the other routes are measured, because the planner
// would otherwise use it for the residual query too.
func worstCase(ctx context.Context, dsn string, d *Dataset, cfg worstConfig) (map[string]any, error) {
	bt, err := connectRoute(ctx, dsn, routeBTree)
	if err != nil {
		return nil, err
	}
	defer closeConn(bt)
	bm, err := connectRoute(ctx, dsn, routeBM25)
	if err != nil {
		return nil, err
	}
	defer closeConn(bm)
	out := map[string]any{"flow": cfg.Flow, "reps": cfg.Reps}
	targets := d.explainTargets()
	values := map[string]string{"rare": rareKeyword3, "absent": absentKeyword3}
	for _, tg := range targets {
		var flowRows, matches int64
		if err := bt.QueryRow(ctx, `SELECT count(*), count(*) FILTER (WHERE sa_keyword->>'keyword3' = $3)
			FROM executions_visibility WHERE namespace_id = $1 AND namespace_division IS NULL AND flow_type = $2`,
			d.NamespaceIDByIndex(tg.ns), cfg.Flow, rareKeyword3).Scan(&flowRows, &matches); err != nil {
			return nil, err
		}
		out[tg.label+"_flow_rows"], out[tg.label+"_rare_matches"] = flowRows, matches
	}

	plans := map[string]string{}
	explainShapes := func(shapes []Shape) error {
		for _, tg := range targets {
			for vlabel, v := range values {
				for _, s := range shapes {
					c := bt
					if s.Route == routeBM25 {
						c = bm
					}
					p, err := explainText(ctx, c, s.SQL, []any{d.NamespaceIDByIndex(tg.ns), cfg.Flow, v})
					if err != nil {
						return fmt.Errorf("explain %s: %w", s.Name, err)
					}
					plans[fmt.Sprintf("%s/%s/%s", s.Name, tg.label, vlabel)] = p
				}
			}
		}
		return nil
	}

	results := map[string]map[string]any{}
	measure := func(phase string, shapes []Shape, bounds []int) error {
		res := results[phase]
		if res == nil {
			res = map[string]any{}
			results[phase] = res
		}
		for _, tg := range targets {
			nsID := d.NamespaceIDByIndex(tg.ns)
			for vlabel, v := range values {
				for _, s := range shapes {
					c := bt
					if s.Route == routeBM25 {
						c = bm
					}
					var lat []float64
					rowsOut := 0
					for i := 0; i < cfg.Reps; i++ {
						ms, n, err := timeQuery(ctx, c, s.SQL, nsID, cfg.Flow, v)
						if err != nil {
							return fmt.Errorf("%s: %w", s.Name, err)
						}
						lat, rowsOut = append(lat, ms), n
					}
					r := percentiles(lat)
					r["rows"] = rowsOut
					res[fmt.Sprintf("%s/%s/%s", s.Name, tg.label, vlabel)] = r
				}
				for _, k := range bounds {
					var lat []float64
					fallbacks, rowsOut := 0, 0
					for i := 0; i < cfg.Reps; i++ {
						ms, n, fb, err := bounded(ctx, bt, bm, nsID, cfg.Flow, v, k)
						if err != nil {
							return fmt.Errorf("bounded: %w", err)
						}
						lat, rowsOut = append(lat, ms), n
						if fb {
							fallbacks++
						}
					}
					r := percentiles(lat)
					r["rows"], r["fallbacks"] = rowsOut, fallbacks
					res[fmt.Sprintf("bounded_%d/%s/%s", k, tg.label, vlabel)] = r
				}
			}
		}
		return nil
	}
	underWrites := fmt.Sprintf("under_%d_writers", cfg.Writers)
	withWriters := func(label string, f func() error) error {
		if cfg.Writers == 0 {
			return nil
		}
		wctx, cancel := context.WithCancel(ctx)
		defer cancel()
		rec := newRecorder()
		var wg sync.WaitGroup
		for w := 0; w < cfg.Writers; w++ {
			wr, err := newWriter(wctx, dsn, d, w, cfg.Writers, rec)
			if err != nil {
				return err
			}
			wr.chooseNS = func(rng *rand.Rand) int {
				if d.HeavyRows > 0 && rng.Intn(2) == 0 {
					return d.HeavyIndex()
				}
				return rng.Intn(d.Namespaces)
			}
			wg.Add(1)
			go func() {
				defer wg.Done()
				defer closeConn(wr.conn)
				wr.loop(wctx, defaultWriteMix)
			}()
		}
		started := time.Now()
		err := f()
		cancel()
		wg.Wait()
		out["writes_"+label] = rec.results(time.Since(started).Seconds())
		return err
	}

	routes := []Shape{worstBTree, worstBM25}
	if err := explainShapes(routes); err != nil {
		return nil, err
	}
	if err := measure("quiet", routes, cfg.Bounds); err != nil {
		return nil, err
	}
	if err := withWriters("routes", func() error { return measure(underWrites, routes, cfg.Bounds) }); err != nil {
		return nil, err
	}

	t := time.Now()
	if _, err := bt.Exec(ctx, promotedIndexDDL); err != nil {
		return nil, err
	}
	out["promoted_index_build_seconds"] = time.Since(t).Seconds()
	defer func() { _, _ = bt.Exec(context.Background(), `DROP INDEX IF EXISTS wc_promoted_keyword3`) }()
	if _, err := bt.Exec(ctx, `ANALYZE executions_visibility`); err != nil {
		return nil, err
	}
	var promotedSize int64
	if err := bt.QueryRow(ctx, `SELECT pg_relation_size('wc_promoted_keyword3')`).Scan(&promotedSize); err != nil {
		return nil, err
	}
	out["promoted_index_bytes"] = promotedSize
	promoted := []Shape{worstPromoted}
	if err := explainShapes(promoted); err != nil {
		return nil, err
	}
	if err := measure("quiet", promoted, nil); err != nil {
		return nil, err
	}
	if err := withWriters("promoted", func() error { return measure(underWrites, promoted, nil) }); err != nil {
		return nil, err
	}
	out["plans"] = plans
	for k, v := range results {
		out[k] = v
	}
	return out, nil
}

type pageRow struct {
	key    SortKey
	status int
}

func scanPage(rows pgx.Rows) ([]pageRow, error) {
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (pageRow, error) {
		var p pageRow
		err := r.Scan(&p.key.RunID, &p.key.Close, &p.key.Start, &p.status)
		return p, err
	})
}

func snapshot(ctx context.Context, conn *pgx.Conn, nsID, flow, kw1 string) (map[string]pageRow, error) {
	rows, err := conn.Query(ctx, snapshotSQL, nsID, flow, kw1)
	if err != nil {
		return nil, err
	}
	list, err := scanPage(rows)
	if err != nil {
		return nil, err
	}
	out := make(map[string]pageRow, len(list))
	for _, p := range list {
		out[p.key.RunID] = p
	}
	return out, nil
}

func sameKey(a, b SortKey) bool {
	return a.Close.Equal(b.Close) && a.Start.Equal(b.Start) && a.RunID == b.RunID
}

// pagingResult classifies every row seen while paging against before/after snapshots.
type pagingResult struct {
	Pages, Seen                   int
	Stable, StableDuplicated      int
	StableSkipped                 int
	OpenToClosed                  int
	OpenToClosedSeen              map[int]int // times seen -> rows
	OtherMoved, Inserted, Deleted int
	InsertedSeen                  int // rows created after the first snapshot that paging reached
	ViolationExamples             []string
}

func evaluatePaging(before, after map[string]pageRow, seen map[string]int) pagingResult {
	r := pagingResult{OpenToClosedSeen: map[int]int{}}
	for id, b := range before {
		a, ok := after[id]
		switch {
		case !ok:
			r.Deleted++
		case sameKey(a.key, b.key):
			r.Stable++
			switch n := seen[id]; {
			case n > 1:
				r.StableDuplicated++
				r.addExample(fmt.Sprintf("duplicated %s x%d", id, n))
			case n == 0:
				r.StableSkipped++
				r.addExample(fmt.Sprintf("skipped %s", id))
			}
		case b.status == statusRunning && a.status != statusRunning:
			r.OpenToClosed++
			r.OpenToClosedSeen[seen[id]]++
		default:
			r.OtherMoved++
			r.addExample(fmt.Sprintf("moved without open->closed %s", id))
		}
	}
	for id := range after {
		if _, ok := before[id]; !ok {
			r.Inserted++
			if seen[id] > 0 {
				r.InsertedSeen++
			}
		}
	}
	return r
}

func (r *pagingResult) addExample(s string) {
	if len(r.ViolationExamples) < 20 {
		r.ViolationExamples = append(r.ViolationExamples, s)
	}
}

type pagingConfig struct {
	Writers   int
	Flow, KW1 string
	PageDelay time.Duration
}

// pagingTest pages the heavy namespace (or namespace 1) with keyset tokens and a residual filter
// while writers close, rewrite, insert into and delete from that same set.
func pagingTest(ctx context.Context, dsn string, d *Dataset, cfg pagingConfig) (map[string]any, error) {
	ns := 1
	if d.HeavyRows > 0 {
		ns = d.HeavyIndex()
	}
	nsID := d.NamespaceIDByIndex(ns)
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		return nil, err
	}
	defer closeConn(conn)

	wctx, cancel := context.WithCancel(ctx)
	defer cancel()
	rec := newRecorder()
	var wg sync.WaitGroup
	lo, hi := d.Rows, d.Total()
	if d.HeavyRows == 0 {
		lo, hi = 0, d.Rows
	}
	match := func(g int64) bool {
		r := d.Row(g)
		return r.NamespaceID == nsID && r.FlowType != nil && *r.FlowType == cfg.Flow && r.Keyword["keyword1"] == cfg.KW1
	}
	for w := 0; w < cfg.Writers; w++ {
		wr, err := newWriter(wctx, dsn, d, w, cfg.Writers, rec)
		if err != nil {
			return nil, err
		}
		wr.lo, wr.hi, wr.match = lo, hi, match
		wr.newFlow, wr.newKW1 = cfg.Flow, cfg.KW1
		wr.chooseNS = func(*rand.Rand) int { return ns }
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer closeConn(wr.conn)
			wr.loop(wctx, pagingWriteMix)
		}()
	}
	time.Sleep(3 * time.Second)

	started := time.Now()
	before, err := snapshot(ctx, conn, nsID, cfg.Flow, cfg.KW1)
	if err != nil {
		return nil, err
	}
	seen := map[string]int{}
	pages := 0
	rows, err := conn.Query(ctx, keysetFirstSQL, nsID, cfg.Flow, cfg.KW1)
	for {
		if err != nil {
			return nil, err
		}
		page, err := scanPage(rows)
		if err != nil {
			return nil, err
		}
		pages++
		for _, p := range page {
			seen[p.key.RunID]++
		}
		if len(page) < 50 {
			break
		}
		last := page[len(page)-1].key
		time.Sleep(cfg.PageDelay)
		rows, err = conn.Query(ctx, keysetSQL, nsID, cfg.Flow, cfg.KW1, last.Close, last.Start, last.RunID)
	}
	after, err := snapshot(ctx, conn, nsID, cfg.Flow, cfg.KW1)
	if err != nil {
		return nil, err
	}
	pagingSeconds := time.Since(started).Seconds()
	cancel()
	wg.Wait()

	r := evaluatePaging(before, after, seen)
	r.Pages, r.Seen = pages, len(seen)
	return map[string]any{
		"namespace": map[bool]string{true: "heavy", false: "regular"}[ns == d.HeavyIndex()], "flow": cfg.Flow, "keyword1": cfg.KW1,
		"set_before": len(before), "set_after": len(after), "paging_seconds": pagingSeconds, "result": r,
		"writes": rec.results(pagingSeconds), "passed": r.StableDuplicated == 0 && r.StableSkipped == 0 && r.OtherMoved == 0,
	}, nil
}
