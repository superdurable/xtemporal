package main

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type sample struct {
	mu       sync.Mutex
	latency  []float64
	errors   int64
	firstErr string
}

func (s *sample) add(begun time.Time, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err != nil {
		s.errors++
		if s.firstErr == "" {
			s.firstErr = err.Error()
		}
		return
	}
	s.latency = append(s.latency, float64(time.Since(begun).Microseconds())/1000)
}

func (s *sample) result(seconds float64) map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	sort.Float64s(s.latency)
	out := map[string]any{"count": len(s.latency), "errors": s.errors, "per_second": float64(len(s.latency)) / seconds}
	if s.firstErr != "" {
		out["first_error"] = s.firstErr
	}
	if n := len(s.latency); n > 0 {
		for _, p := range []int{50, 95, 99} {
			out[fmt.Sprintf("p%d_ms", p)] = s.latency[(n-1)*p/100]
		}
		out["max_ms"] = s.latency[n-1]
	}
	return out
}

type recorder struct {
	mu sync.Mutex
	m  map[string]*sample
}

func newRecorder() *recorder { return &recorder{m: map[string]*sample{}} }

func (r *recorder) get(name string) *sample {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.m[name]
	if !ok {
		s = &sample{}
		r.m[name] = s
	}
	return s
}

func (r *recorder) results(seconds float64) map[string]any {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := map[string]any{}
	for k, s := range r.m {
		out[k] = s.result(seconds)
	}
	return out
}

// Pools: B-tree shapes run with the default plan cache; BM25 shapes need custom plans
// (a generic plan with JSON keys loses the ParadeDB custom scan on 0.26.1).
type pools struct{ btree, bm25 *pgxpool.Pool }

// btreeMode optionally sets plan_cache_mode on the B-tree pool; the BM25 pool always uses custom plans.
func newPools(ctx context.Context, dsn string, size int, btreeMode string) (*pools, error) {
	mk := func(mode string) (*pgxpool.Pool, error) {
		cfg, err := pgxpool.ParseConfig(dsn)
		if err != nil {
			return nil, err
		}
		cfg.MaxConns = int32(size)
		if mode != "" {
			cfg.AfterConnect = func(ctx context.Context, c *pgx.Conn) error {
				_, err := c.Exec(ctx, "SET plan_cache_mode = "+mode)
				return err
			}
		}
		return pgxpool.NewWithConfig(ctx, cfg)
	}
	b, err := mk(btreeMode)
	if err != nil {
		return nil, err
	}
	m, err := mk("force_custom_plan")
	if err != nil {
		b.Close()
		return nil, err
	}
	return &pools{b, m}, nil
}

func (p *pools) close() { p.btree.Close(); p.bm25.Close() }

func (p *pools) forShape(s Shape) *pgxpool.Pool {
	if s.Route == routeBM25 {
		return p.bm25
	}
	return p.btree
}

func drain(rows pgx.Rows) (int, error) {
	n := 0
	for rows.Next() {
		n++
	}
	rows.Close()
	return n, rows.Err()
}

func runQuery(ctx context.Context, pool *pgxpool.Pool, sql string, args ...any) (int, error) {
	rows, err := pool.Query(ctx, sql, args...)
	if err != nil {
		return 0, err
	}
	return drain(rows)
}

// browser is one visible console tab polling a namespace.
type browser struct {
	ns     int
	nsID   string
	flows  []string
	perms  []string
	rng    *rand.Rand
	rowsIn int64
}

func pickFlows(rng *rand.Rand, n int) []string {
	seen := map[string]bool{}
	var out []string
	for len(out) < n {
		f := pickFlow(rng.Uint64())
		if !seen[f] {
			seen[f] = true
			out = append(out, f)
		}
	}
	return out
}

func (b *browser) args(s Shape, flow string) []any {
	switch s.Name {
	case "main_list", "running":
		return []any{b.nsID, flow}
	case "need_person":
		return []any{b.nsID, flow, b.perms}
	case "done_today":
		return []any{b.nsID, flow, today()}
	case "custom_eq":
		return []any{b.nsID, flow, fmt.Sprintf("team-%d", b.rng.Intn(20))}
	case "custom_range":
		lo := b.rng.Intn(950)
		return []any{b.nsID, flow, lo, lo + 50}
	case "text_bm25":
		return []any{b.nsID, flow, words[b.rng.Intn(len(words))]}
	case "custom_only_bm25":
		return []any{b.nsID, fmt.Sprintf("k3-%d", b.rng.Intn(50))}
	case "schedules", "readyz":
		return []any{b.nsID}
	}
	return nil
}

func (b *browser) workflowID(d *Dataset) string {
	return fmt.Sprintf("wf-%d", d.RowID(b.ns, b.rng.Int63n(b.rowsIn)))
}

type mixConfig struct {
	Writers         int
	Browsers        int
	FlowsPerBrowser int
	Pool            int
	Duration        time.Duration
	PollInterval    time.Duration
	HeavyBrowsers   float64 // share of browsers polling the heavy namespace
	HeavyWrites     float64 // share of starts in the heavy namespace
	ReadyzQPS       float64
	BtreePlanMode   string // auto (empty), force_custom_plan or force_generic_plan for B-tree shapes
}

// runMix drives concurrent console browsers and store writers for cfg.Duration.
func runMix(ctx context.Context, dsn string, d *Dataset, cfg mixConfig) (map[string]any, error) {
	p, err := newPools(ctx, dsn, cfg.Pool, cfg.BtreePlanMode)
	if err != nil {
		return nil, err
	}
	defer p.close()
	reads := newRecorder()
	runCtx, cancel := context.WithTimeout(ctx, cfg.Duration)
	defer cancel()
	var polls, latePolls atomic.Int64
	var wg sync.WaitGroup
	started := time.Now()

	for i := 0; i < cfg.Browsers; i++ {
		rng := rand.New(rand.NewSource(int64(i) + 7))
		ns := rng.Intn(d.Namespaces)
		if d.HeavyRows > 0 && rng.Float64() < cfg.HeavyBrowsers {
			ns = d.HeavyIndex()
		}
		perm := rng.Perm(len(permissions))
		b := &browser{ns: ns, nsID: d.NamespaceIDByIndex(ns), flows: pickFlows(rng, cfg.FlowsPerBrowser),
			perms: []string{permissions[perm[0]], permissions[perm[1]]}, rng: rng, rowsIn: d.RowsIn(ns)}
		wg.Add(1)
		go func(b *browser, offset time.Duration) {
			defer wg.Done()
			select {
			case <-runCtx.Done():
				return
			case <-time.After(offset):
			}
			extras := []Shape{textShape, customOnlyShape, scheduleShape}
			for n := 0; runCtx.Err() == nil; n++ {
				begun := time.Now()
				type job struct {
					shape Shape
					args  []any
				}
				var jobs []job
				for _, f := range b.flows {
					for _, s := range flowShapes {
						jobs = append(jobs, job{s, b.args(s, f)})
					}
					jobs = append(jobs, job{workflowIDShape, []any{b.nsID, b.workflowID(d)}})
				}
				extra := extras[n%len(extras)]
				jobs = append(jobs, job{extra, b.args(extra, b.flows[0])})
				var pw sync.WaitGroup
				for _, j := range jobs {
					pw.Add(1)
					go func(j job) {
						defer pw.Done()
						t := time.Now()
						_, err := runQuery(runCtx, p.forShape(j.shape), j.shape.SQL, j.args...)
						if runCtx.Err() == nil {
							reads.get(j.shape.Name).add(t, err)
						}
					}(j)
				}
				pw.Wait()
				polls.Add(1)
				elapsed := time.Since(begun)
				if elapsed > cfg.PollInterval {
					latePolls.Add(1)
					continue
				}
				select {
				case <-runCtx.Done():
				case <-time.After(cfg.PollInterval - elapsed):
				}
			}
		}(b, time.Duration(rng.Int63n(int64(cfg.PollInterval))))
	}

	if cfg.ReadyzQPS > 0 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rng := rand.New(rand.NewSource(99))
			ticker := time.NewTicker(time.Duration(float64(time.Second) / cfg.ReadyzQPS))
			defer ticker.Stop()
			for {
				select {
				case <-runCtx.Done():
					return
				case <-ticker.C:
				}
				t := time.Now()
				_, err := runQuery(runCtx, p.btree, readyzShape.SQL, d.NamespaceIDByIndex(rng.Intn(d.Namespaces)))
				if runCtx.Err() == nil {
					reads.get(readyzShape.Name).add(t, err)
				}
			}
		}()
	}

	writes := newRecorder()
	for w := 0; w < cfg.Writers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			wr, err := newWriter(runCtx, dsn, d, w, cfg.Writers, writes)
			if err != nil {
				writes.get("connect").add(time.Now(), err)
				return
			}
			defer closeConn(wr.conn)
			wr.chooseNS = func(rng *rand.Rand) int {
				if d.HeavyRows > 0 && rng.Float64() < cfg.HeavyWrites {
					return d.HeavyIndex()
				}
				return rng.Intn(d.Namespaces)
			}
			wr.loop(runCtx, defaultWriteMix)
		}(w)
	}
	wg.Wait()
	seconds := time.Since(started).Seconds()
	writeResults := writes.results(seconds)
	var total float64
	var errs int64
	for _, v := range writeResults {
		total += v.(map[string]any)["per_second"].(float64)
		errs += v.(map[string]any)["errors"].(int64)
	}
	result := map[string]any{
		"seconds": seconds, "writers": cfg.Writers, "browsers": cfg.Browsers, "flows_per_browser": cfg.FlowsPerBrowser,
		"pool": cfg.Pool, "btree_plan_mode": cfg.BtreePlanMode, "polls": polls.Load(), "late_polls": latePolls.Load(),
		"reads": reads.results(seconds), "writes": writeResults, "write_statements_per_second": total,
	}
	if errs > 0 {
		return result, fmt.Errorf("write workload had %d errors", errs)
	}
	for name, v := range result["reads"].(map[string]any) {
		if n := v.(map[string]any)["errors"].(int64); n > 0 {
			return result, fmt.Errorf("%s had %d read errors: %v", name, n, v.(map[string]any)["first_error"])
		}
	}
	return result, nil
}

// writeMix weights the store operations; each op is one or two version-guarded statements. Starts
// and closes of the writer's own runs balance, so the open-row share stays near the preloaded 0.3%.
type writeMix struct{ start, closeOwn, closePreloaded, upsert, continueAsNew, delete, rewrite int }

var (
	defaultWriteMix = writeMix{start: 25, closeOwn: 25, closePreloaded: 5, upsert: 20, continueAsNew: 15, delete: 10}
	pagingWriteMix  = writeMix{start: 20, closeOwn: 20, closePreloaded: 10, rewrite: 40, delete: 10}
)

type writer struct {
	conn     *pgx.Conn
	d        *Dataset
	rng      *rand.Rand
	rec      *recorder
	open     []Row
	cursor   int64 // next preloaded id this writer scans for open rows
	stride   int64
	lo, hi   int64 // preloaded id range the writer closes, rewrites and deletes in
	chooseNS func(*rand.Rand) int
	match    func(g int64) bool // restricts rewrites (paging test)
	newFlow  string
	newKW1   string
}

func newWriter(ctx context.Context, dsn string, d *Dataset, w, writers int, rec *recorder) (*writer, error) {
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		return nil, err
	}
	return &writer{conn: conn, d: d, rng: rand.New(rand.NewSource(int64(w)*7919 + time.Now().UnixNano())), rec: rec,
		cursor: int64(w), stride: int64(writers), lo: 0, hi: d.Total()}, nil
}

func (wr *writer) exec(ctx context.Context, op, sql string, args ...any) error {
	t := time.Now()
	_, err := wr.conn.Exec(ctx, sql, args...)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	wr.rec.get(op).add(t, err)
	return err
}

func (wr *writer) upsert(ctx context.Context, op string, r *Row) error {
	return wr.exec(ctx, op, upsertSQL, r.Values()...)
}

func (wr *writer) start(ctx context.Context) error {
	ns := wr.chooseNS(wr.rng)
	flow := wr.newFlow
	if flow == "" {
		flow = pickFlow(wr.rng.Uint64())
	}
	id := uuidFrom(fmt.Sprintf("new-%d-%d", wr.rng.Int63(), time.Now().UnixNano()))
	r := wr.d.NewRun(ns, flow, wr.newKW1, "wf-new-"+id[:13], id, time.Now(), wr.rng.Int63n(wr.d.Rows))
	if err := wr.upsert(ctx, "start", &r); err != nil {
		return err
	}
	wr.open = append(wr.open, r)
	return nil
}

// nextPreloadedOpen returns a preloaded open user row in this writer's partition of [lo, hi).
func (wr *writer) nextPreloadedOpen() (int64, bool) {
	for i := 0; i < 1_000_000; i++ {
		g := wr.lo + wr.cursor
		wr.cursor += wr.stride
		if g >= wr.hi {
			return 0, false
		}
		if wr.d.IsUserRow(g) && wr.d.Status(g) == statusRunning && (wr.match == nil || wr.match(g)) {
			return g, true
		}
	}
	return 0, false
}

func (wr *writer) closeRow(ctx context.Context, op string, r *Row, status int, version int64) error {
	now := time.Now().UTC().Truncate(time.Microsecond)
	r.Status = status
	r.Close = &now
	r.Duration = now.Sub(r.Start).Nanoseconds()
	r.Version = version
	return wr.upsert(ctx, op, r)
}

func (wr *writer) popOpen() (Row, bool) {
	if len(wr.open) == 0 {
		return Row{}, false
	}
	i := wr.rng.Intn(len(wr.open))
	r := wr.open[i]
	wr.open[i] = wr.open[len(wr.open)-1]
	wr.open = wr.open[:len(wr.open)-1]
	return r, true
}

func (wr *writer) randomClosedPreloaded() (int64, bool) {
	for i := 0; i < 10_000; i++ {
		g := wr.lo + wr.rng.Int63n(wr.hi-wr.lo)
		if wr.d.IsUserRow(g) && wr.d.Status(g) != statusRunning && (wr.match == nil || wr.match(g)) {
			return g, true
		}
	}
	return 0, false
}

func (wr *writer) loop(ctx context.Context, mix writeMix) {
	total := mix.start + mix.closeOwn + mix.closePreloaded + mix.upsert + mix.continueAsNew + mix.delete + mix.rewrite
	for ctx.Err() == nil {
		op := wr.rng.Intn(total)
		var err error
		switch {
		case op < mix.start:
			err = wr.start(ctx)
		case op < mix.start+mix.closeOwn:
			if r, ok := wr.popOpen(); ok {
				err = wr.closeRow(ctx, "close", &r, statusCompleted, time.Now().UnixNano())
			} else {
				err = wr.start(ctx)
			}
		case op < mix.start+mix.closeOwn+mix.closePreloaded:
			// Preloaded open rows close with a fixed version, so a repeat close in a later phase is a
			// no-op instead of moving an already closed row.
			if g, ok := wr.nextPreloadedOpen(); ok {
				r := wr.d.Row(g)
				err = wr.closeRow(ctx, "close", &r, statusCompleted, 2)
			}
		case op < mix.start+mix.closeOwn+mix.closePreloaded+mix.upsert:
			if len(wr.open) == 0 {
				err = wr.start(ctx)
				break
			}
			r := &wr.open[wr.rng.Intn(len(wr.open))]
			r.Number["int1"] = wr.rng.Intn(1000)
			r.Keyword["DexWorkQueuePermissions"] = []string{permissions[wr.rng.Intn(len(permissions))]}
			r.Version = time.Now().UnixNano()
			err = wr.upsert(ctx, "upsert", r)
		case op < mix.start+mix.closeOwn+mix.closePreloaded+mix.upsert+mix.continueAsNew:
			r, ok := wr.popOpen()
			if !ok {
				err = wr.start(ctx)
				break
			}
			if err = wr.closeRow(ctx, "continue_as_new_close", &r, statusContinuedAsNew, time.Now().UnixNano()); err != nil {
				break
			}
			id := uuidFrom(fmt.Sprintf("can-%d-%d", wr.rng.Int63(), time.Now().UnixNano()))
			next := wr.d.NewRun(0, *r.FlowType, wr.newKW1, r.WorkflowID, id, time.Now(), wr.rng.Int63n(wr.d.Rows))
			next.NamespaceID = r.NamespaceID
			next.Keyword["keyword2"] = r.Keyword["keyword2"]
			if err = wr.upsert(ctx, "continue_as_new_start", &next); err == nil {
				wr.open = append(wr.open, next)
			}
		case op < mix.start+mix.closeOwn+mix.closePreloaded+mix.upsert+mix.continueAsNew+mix.delete:
			g, ok := wr.randomClosedPreloaded()
			if !ok {
				continue
			}
			r := wr.d.Row(g)
			err = wr.exec(ctx, "delete", deleteSQL, r.NamespaceID, r.RunID, time.Now().UnixNano())
		default:
			// Re-delivered visibility task: same row, higher version, unchanged sort key. Microsecond
			// versions stay below the nanosecond delete versions, so a rewrite never outranks a tombstone.
			g, ok := wr.randomClosedPreloaded()
			if !ok {
				continue
			}
			r := wr.d.Row(g)
			r.Version = time.Now().UnixMicro()
			err = wr.upsert(ctx, "rewrite", &r)
		}
		if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) && wr.conn.IsClosed() {
			return
		}
	}
}
