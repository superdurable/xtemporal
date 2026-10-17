// Command dex runs the Dex-profile visibility benchmark for the json-hybrid ParadeDB design.
// See ../dex-profile-benchmark.md for the specification and ../dex-compare.sh for the driver.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

func parseInts(s string) ([]int, error) {
	var out []int
	for _, f := range strings.Fields(strings.ReplaceAll(s, ",", " ")) {
		n, err := strconv.Atoi(f)
		if err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, nil
}

func main() {
	mode := flag.String("mode", "", "load, sizes, explain, mix, c3, drop-c3, worst, or paging")
	dsn := flag.String("dsn", "postgres://postgres:spike@127.0.0.1:56640/visibility?sslmode=disable", "database URL")
	schema := flag.String("schema", "dex-profile-schema.sql", "schema DDL for -mode load")
	namespaces := flag.Int("namespaces", 10000, "regular namespaces")
	rows := flag.Int64("rows", 10_000_000, "rows across the regular namespaces")
	heavyRows := flag.Int64("heavy-rows", 0, "rows in the single heavy namespace")
	parallel := flag.Int("parallel", 4, "parallel COPY streams")
	writers := flag.Int("writers", 16, "concurrent writers")
	browsers := flag.Int("browsers", 200, "concurrent console browsers")
	flows := flag.Int("flows", 3, "FlowTypes shown per browser")
	pool := flag.Int("pool", 32, "read connection pool size per route")
	duration := flag.Duration("duration", 10*time.Minute, "mix duration")
	poll := flag.Duration("poll", 10*time.Second, "browser poll interval")
	heavyBrowsers := flag.Float64("heavy-browsers", 0.05, "share of browsers on the heavy namespace")
	heavyWrites := flag.Float64("heavy-writes", 0.1, "share of starts in the heavy namespace")
	readyz := flag.Float64("readyz-qps", 0.6, "readiness probe QPS (0.2 per replica, 3 replicas)")
	btreePlan := flag.String("btree-plan-mode", "", "plan_cache_mode for B-tree shapes: empty (auto), force_custom_plan or force_generic_plan")
	reps := flag.Int("reps", 20, "worst-case repetitions per variant")
	bounds := flag.String("bounds", "1000 10000", "bounded B-tree scan sizes for the worst case")
	pageDelay := flag.Duration("page-delay", 10*time.Millisecond, "pause between pages in the paging test")
	flag.Parse()

	ctx := context.Background()
	result, err := run(ctx, *mode, *dsn, func(conn *pgx.Conn) (*Dataset, error) {
		if *mode == "load" {
			return &Dataset{Namespaces: *namespaces, Rows: *rows, HeavyRows: *heavyRows, Base: time.Now().UTC().Truncate(time.Second)}, nil
		}
		return readMeta(ctx, conn)
	}, func(d *Dataset, conn *pgx.Conn) (any, error) {
		switch *mode {
		case "load":
			return load(ctx, *dsn, *schema, d, *parallel)
		case "sizes":
			return sizes(ctx, conn)
		case "explain":
			return explainAll(ctx, *dsn, d)
		case "mix":
			return runMix(ctx, *dsn, d, mixConfig{Writers: *writers, Browsers: *browsers, FlowsPerBrowser: *flows, Pool: *pool,
				Duration: *duration, PollInterval: *poll, HeavyBrowsers: *heavyBrowsers, HeavyWrites: *heavyWrites, ReadyzQPS: *readyz, BtreePlanMode: *btreePlan})
		case "c3":
			t := time.Now()
			if _, err := conn.Exec(ctx, c3IndexDDL); err != nil {
				return nil, err
			}
			build := time.Since(t).Seconds()
			if _, err := conn.Exec(ctx, `ANALYZE executions_visibility`); err != nil {
				return nil, err
			}
			var size int64
			err := conn.QueryRow(ctx, `SELECT pg_relation_size('by_flow_type_status')`).Scan(&size)
			return map[string]any{"build_seconds": build, "bytes": size}, err
		case "drop-c3":
			_, err := conn.Exec(ctx, `DROP INDEX IF EXISTS by_flow_type_status`)
			return map[string]any{"dropped": err == nil}, err
		case "worst":
			b, err := parseInts(*bounds)
			if err != nil {
				return nil, err
			}
			return worstCase(ctx, *dsn, d, worstConfig{Reps: *reps, Writers: *writers, Bounds: b, Flow: "approval"})
		case "paging":
			return pagingTest(ctx, *dsn, d, pagingConfig{Writers: *writers, Flow: "approval", KW1: "team-3", PageDelay: *pageDelay})
		}
		return nil, fmt.Errorf("unknown mode %q", *mode)
	})
	if result != nil {
		if encErr := json.NewEncoder(os.Stdout).Encode(result); encErr != nil {
			fmt.Fprintln(os.Stderr, encErr)
			os.Exit(1)
		}
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, mode, dsn string, dataset func(*pgx.Conn) (*Dataset, error), body func(*Dataset, *pgx.Conn) (any, error)) (any, error) {
	if mode == "" {
		return nil, errors.New("-mode is required")
	}
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		return nil, err
	}
	defer closeConn(conn)
	d, err := dataset(conn)
	if err != nil {
		return nil, err
	}
	return body(d, conn)
}
