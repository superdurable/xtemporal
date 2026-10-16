package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

const lockShardQry = `SELECT range_id FROM shards WHERE shard_id = $1 FOR UPDATE`
const readLockShardQry = `SELECT range_id FROM shards WHERE shard_id = $1 FOR SHARE`
const updateShardQry = `UPDATE shards 
 SET range_id = $1, data = $2, data_encoding = $3 
 WHERE shard_id = $4`

type caseResult struct {
	Case, Topology, Isolation, Status, Reason, SQLState string
	Iteration                                           int
	Observed                                            int64
	Blocked                                             bool
	DurationMS                                          int64
}
type lockResults struct {
	Target    string
	At        time.Time
	Cases     []caseResult
	SQLStates map[string]int
}

func state(e error) string {
	var pe *pgconn.PgError
	if errors.As(e, &pe) {
		return pe.Code
	}
	if e != nil {
		return "connection"
	}
	return ""
}
func caseRun(ctx context.Context, c *connector, id int, kind, iso, top string, iteration int) (r caseResult) {
	r = caseResult{Case: kind, Topology: top, Isolation: iso, Iteration: iteration, Status: "FAIL"}
	start := time.Now()
	defer func() { r.DurationMS = time.Since(start).Milliseconds() }()
	if kind == "A8" && c.target == "pg16" {
		r.Status = "SKIP"
		r.Reason = "Limitless plan required"
		return
	}
	a, e := c.DialRouter(ctx, 0)
	if e != nil {
		r.Reason = e.Error()
		return
	}
	defer a.Close(ctx)
	bi := 0
	if top == "cross-router" {
		bi = 1
	}
	b, e := c.DialRouter(ctx, bi)
	if e != nil {
		r.Reason = e.Error()
		return
	}
	defer b.Close(ctx)
	if e = resetRow(ctx, a, id); e != nil {
		r.Reason = e.Error()
		return
	}
	if kind == "A10" {
		return deadlockCase(ctx, c, a, b, id, iteration, top)
	}
	if kind == "A8" {
		return a8Case(ctx, a, id, top, iteration)
	}
	if kind == "A4-nowait" || kind == "A4-wait" {
		return a4Case(ctx, a, b, id, kind, top, iteration)
	}
	if kind == "A6-share-share" || kind == "A6-nowait" || kind == "A6-timeout" || kind == "A6-share-update" || kind == "A6-update-share" {
		return a6Case(ctx, a, b, id, kind, top, iteration)
	}
	bt, e := b.Begin(ctx)
	if e != nil {
		r.Reason = e.Error()
		return
	}
	defer bt.Rollback(ctx)
	if e = bt.QueryRow(ctx, lockShardQry, id).Scan(new(int64)); e != nil {
		r.Reason = e.Error()
		return
	}
	if _, e = bt.Exec(ctx, "UPDATE shards SET range_id=6 WHERE shard_id=$1", id); e != nil {
		r.Reason = e.Error()
		return
	}
	opts := pgx.TxOptions{}
	if iso == "RR" {
		opts.IsoLevel = pgx.RepeatableRead
	}
	at, e := a.BeginTx(ctx, opts)
	if e != nil {
		r.Reason = e.Error()
		return
	}
	defer at.Rollback(ctx)
	if kind == "A3" {
		var v int
		if e = at.QueryRow(ctx, "SELECT 1 FROM execs WHERE shard_id=$1", id).Scan(&v); e != nil {
			r.Reason = e.Error()
			return
		}
	}
	if iso == "RR" {
		var v int64
		if e = at.QueryRow(ctx, "SELECT range_id FROM shards WHERE shard_id=$1", id).Scan(&v); e != nil {
			r.Reason = e.Error()
			return
		}
	}
	type outcome struct {
		value int64
		rows  int64
		err   error
	}
	done := make(chan outcome, 1)
	go func() {
		var o outcome
		switch kind {
		case "A1", "A3":
			o.err = at.QueryRow(ctx, readLockShardQry, id).Scan(&o.value)
		case "A2", "A7", "A7b":
			o.err = at.QueryRow(ctx, lockShardQry, id).Scan(&o.value)
		case "A5":
			var tag pgconn.CommandTag
			tag, o.err = at.Exec(ctx, "UPDATE shards SET range_id=range_id WHERE shard_id=$1 AND range_id=5", id)
			o.rows = tag.RowsAffected()
		}
		done <- o
	}()
	select {
	case o := <-done:
		r.Reason = fmt.Sprintf("statement returned before takeover commit: %v", o.err)
		return
	case <-time.After(2 * time.Second):
		r.Blocked = true
	case <-ctx.Done():
		r.Reason = ctx.Err().Error()
		return
	}
	if e = bt.Commit(ctx); e != nil {
		r.Reason = e.Error()
		return
	}
	var o outcome
	select {
	case o = <-done:
	case <-time.After(10 * time.Second):
		r.Reason = "statement did not return after commit"
		return
	case <-ctx.Done():
		r.Reason = ctx.Err().Error()
		return
	}
	r.Observed = o.value
	r.SQLState = state(o.err)
	switch kind {
	case "A1", "A2", "A3":
		if iso == "RR" {
			r.Status = pass(o.err != nil && r.SQLState == "40001")
		} else {
			r.Status = pass(o.err == nil && o.value == 6)
		}
	case "A5":
		if iso == "RR" {
			r.Status = pass(r.SQLState == "40001")
		} else {
			r.Observed = o.rows
			r.Status = pass(o.err == nil && o.rows == 0)
		}
	case "A7":
		if o.err == nil && o.value != 5 {
			r.Reason = "ownership lost before UPDATE"
			r.Status = "PASS"
		} else {
			r.Reason = "lock did not fence old owner"
		}
	case "A7b":
		if o.err == nil {
			tag, err := at.Exec(ctx, "UPDATE shards SET range_id=7 WHERE shard_id=$1 AND range_id=5", id)
			r.Observed = tag.RowsAffected()
			r.SQLState = state(err)
			r.Status = pass(err == nil && tag.RowsAffected() == 0)
		}
	}
	return
}
func pass(ok bool) string {
	if ok {
		return "PASS"
	}
	return "FAIL"
}
func a4Case(ctx context.Context, a, b *pgx.Conn, id int, kind, top string, it int) (r caseResult) {
	r = caseResult{Case: kind, Topology: top, Isolation: "RR", Iteration: it, Status: "FAIL"}
	at, e := a.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if e != nil {
		r.Reason = e.Error()
		return
	}
	defer at.Rollback(ctx)
	var x int64
	if e = at.QueryRow(ctx, "SELECT range_id FROM shards WHERE shard_id=$1", id).Scan(&x); e != nil {
		r.Reason = e.Error()
		return
	}
	bt, e := b.Begin(ctx)
	if e != nil {
		r.Reason = e.Error()
		return
	}
	defer bt.Rollback(ctx)
	if _, e = bt.Exec(ctx, "UPDATE shards SET range_id=6 WHERE shard_id=$1", id); e != nil {
		r.Reason = e.Error()
		return
	}
	if kind == "A4-nowait" {
		if e = bt.Commit(ctx); e != nil {
			r.Reason = e.Error()
			return
		}
	}
	done := make(chan error, 1)
	go func() { var v int64; done <- at.QueryRow(ctx, readLockShardQry, id).Scan(&v) }()
	if kind == "A4-wait" {
		select {
		case e = <-done:
			r.Reason = "returned before commit"
			return
		case <-time.After(2 * time.Second):
			r.Blocked = true
		}
		if e = bt.Commit(ctx); e != nil {
			r.Reason = e.Error()
			return
		}
	}
	select {
	case e = <-done:
		r.SQLState = state(e)
		r.Status = pass(r.SQLState == "40001")
	case <-time.After(10 * time.Second):
		r.Reason = "lock did not return"
	}
	return
}
func a6Case(ctx context.Context, a, b *pgx.Conn, id int, kind, top string, it int) (r caseResult) {
	r = caseResult{Case: kind, Topology: top, Isolation: "RC", Iteration: it, Status: "FAIL"}
	bt, e := b.Begin(ctx)
	if e != nil {
		r.Reason = e.Error()
		return
	}
	defer bt.Rollback(ctx)
	first := lockShardQry
	if kind == "A6-share-share" || kind == "A6-share-update" {
		first = readLockShardQry
	}
	var v int64
	if e = bt.QueryRow(ctx, first, id).Scan(&v); e != nil {
		r.Reason = e.Error()
		return
	}
	at, e := a.Begin(ctx)
	if e != nil {
		r.Reason = e.Error()
		return
	}
	defer at.Rollback(ctx)
	if kind == "A6-timeout" {
		if _, e = at.Exec(ctx, "SET LOCAL lock_timeout='500ms'"); e != nil {
			r.Reason = e.Error()
			return
		}
	}
	q := readLockShardQry
	if kind == "A6-nowait" {
		q = lockShardQry + " NOWAIT"
	}
	if kind == "A6-share-update" {
		q = lockShardQry
	}
	done := make(chan error, 1)
	go func() { var x int64; done <- at.QueryRow(ctx, q, id).Scan(&x) }()
	if kind == "A6-share-share" {
		select {
		case e = <-done:
			r.Status = pass(e == nil)
			r.SQLState = state(e)
		case <-time.After(2 * time.Second):
			r.Reason = "compatible share lock blocked"
		}
		return
	}
	if kind == "A6-nowait" || kind == "A6-timeout" {
		select {
		case e = <-done:
			r.SQLState = state(e)
			r.Status = pass(r.SQLState == "55P03")
		case <-time.After(2 * time.Second):
			r.Reason = "timeout did not fire"
		}
		return
	}
	select {
	case e = <-done:
		r.Reason = "conflicting lock did not block"
		return
	case <-time.After(2 * time.Second):
		r.Blocked = true
	}
	if e = bt.Commit(ctx); e != nil {
		r.Reason = e.Error()
		return
	}
	select {
	case e = <-done:
		r.SQLState = state(e)
		r.Status = pass(e == nil)
	case <-time.After(10 * time.Second):
		r.Reason = "lock did not return"
	}
	return
}
func deadlockCase(ctx context.Context, c *connector, a, b *pgx.Conn, id, it int, top string) (r caseResult) {
	r = caseResult{Case: "A10", Topology: top, Isolation: "RC", Iteration: it, Status: "FAIL"}
	second := id + 100000
	if c.target == "limitless" {
		// Shard mapping uses EXPLAIN with shard_plans, which silently drops a later
		// write to the explained key on the same session, so keep it off a and b.
		mapper, e := c.DialAny(ctx)
		if e != nil {
			r.Reason = e.Error()
			return
		}
		defer mapper.Close(ctx)
		first, e := mappedShard(ctx, mapper, id)
		if e != nil {
			r.Reason = e.Error()
			return
		}
		found := false
		for trial := second; trial < second+32; trial++ {
			other, err := mappedShard(ctx, mapper, trial)
			if err == nil && other != first {
				second = trial
				found = true
				break
			}
		}
		if !found {
			r.Status = "SKIP"
			r.Reason = "no second physical shard mapped"
			return
		}
	}
	if _, e := b.Exec(ctx, "INSERT INTO shards(shard_id,range_id) VALUES($1,5) ON CONFLICT DO NOTHING", second); e != nil {
		r.Reason = e.Error()
		return
	}
	at, e := a.Begin(ctx)
	if e != nil {
		r.Reason = e.Error()
		return
	}
	defer at.Rollback(ctx)
	bt, e := b.Begin(ctx)
	if e != nil {
		r.Reason = e.Error()
		return
	}
	defer bt.Rollback(ctx)
	var v int64
	if e = at.QueryRow(ctx, lockShardQry, id).Scan(&v); e != nil {
		r.Reason = e.Error()
		return
	}
	if e = bt.QueryRow(ctx, lockShardQry, second).Scan(&v); e != nil {
		r.Reason = e.Error()
		return
	}
	ad := make(chan error, 1)
	bd := make(chan error, 1)
	go func() { var x int64; ad <- at.QueryRow(ctx, lockShardQry, second).Scan(&x) }()
	go func() { var x int64; bd <- bt.QueryRow(ctx, lockShardQry, id).Scan(&x) }()
	var ae, be error
	select {
	case ae = <-ad:
	case <-time.After(10 * time.Second):
		r.Reason = "deadlock not detected"
		return
	}
	select {
	case be = <-bd:
	case <-time.After(10 * time.Second):
		r.Reason = "second participant stuck"
		return
	}
	r.SQLState = state(ae)
	if r.SQLState == "" {
		r.SQLState = state(be)
	}
	r.Status = pass(state(ae) == "40P01" || state(be) == "40P01")
	return
}
func locktest(ctx context.Context, c *connector, out string, iterations, a9checks, shards int, only string) error {
	selected := func(name string) bool { return only == "" || slices.Contains(strings.Split(only, ","), name) }
	cases := []struct {
		name string
		isos []string
	}{{"A1", []string{"RC", "RR"}}, {"A2", []string{"RC", "RR"}}, {"A3", []string{"RC", "RR"}}, {"A4-nowait", []string{"RR"}}, {"A4-wait", []string{"RR"}}, {"A5", []string{"RC", "RR"}}, {"A6-share-share", []string{"RC"}}, {"A6-share-update", []string{"RC"}}, {"A6-update-share", []string{"RC"}}, {"A6-nowait", []string{"RC"}}, {"A6-timeout", []string{"RC"}}, {"A7", []string{"RC"}}, {"A7b", []string{"RC"}}, {"A8", []string{"RC"}}, {"A10", []string{"RC"}}}
	res := lockResults{Target: c.target, At: time.Now(), SQLStates: map[string]int{}}
	for _, spec := range cases {
		if !selected(spec.name) {
			continue
		}
		for _, iso := range spec.isos {
			for _, top := range []string{"same-router", "cross-router"} {
				var wg sync.WaitGroup
				ch := make(chan caseResult, iterations)
				sem := make(chan struct{}, max(1, min(shards, 64)))
				for i := 0; i < iterations; i++ {
					wg.Add(1)
					sem <- struct{}{}
					go func(i int) {
						defer wg.Done()
						defer func() { <-sem }()
						id := 1 + i%max(1, shards)
						tctx, cancel := context.WithTimeout(ctx, 30*time.Second)
						defer cancel()
						ch <- caseRun(tctx, c, id, spec.name, iso, top, i)
					}(i)
				}
				wg.Wait()
				close(ch)
				for r := range ch {
					res.Cases = append(res.Cases, r)
					if r.SQLState != "" {
						res.SQLStates[r.SQLState]++
					}
					if r.Status == "FAIL" {
						fmt.Printf("%s %s %s iteration %d FAIL: %s state=%s observed=%d\n", r.Case, r.Topology, r.Isolation, r.Iteration, r.Reason, r.SQLState, r.Observed)
					}
				}
				fmt.Printf("%s %s %s complete\n", spec.name, top, iso)
			}
		}
	}
	if selected("A9") {
		res.Cases = append(res.Cases, a9Case(ctx, c, a9checks, shards)...)
	}
	file := filepath.Join(out, fmt.Sprintf("%s-%s-locktest.json", c.target, res.At.Format("20060102T150405")))
	b, _ := json.MarshalIndent(res, "", "  ")
	if e := os.WriteFile(file, b, 0644); e != nil {
		return e
	}
	fmt.Println("locktest results:", file)
	return nil
}
func a9Case(ctx context.Context, c *connector, checks, shards int) []caseResult {
	results := make([]caseResult, shards)
	var wg sync.WaitGroup
	for sid := 1; sid <= shards; sid++ {
		wg.Add(1)
		go func(sid int) {
			defer wg.Done()
			r := caseResult{Case: "A9", Topology: "cross-router", Isolation: "RC", Iteration: sid, Status: "PASS"}
			w, e := c.DialRouter(ctx, 0)
			if e != nil {
				r.Status = "FAIL"
				r.Reason = e.Error()
				results[sid-1] = r
				return
			}
			defer w.Close(ctx)
			reader, e := c.DialRouter(ctx, 1)
			if e != nil {
				r.Status = "FAIL"
				r.Reason = e.Error()
				results[sid-1] = r
				return
			}
			defer reader.Close(ctx)
			limit := checks / shards
			if sid <= checks%shards {
				limit++
			}
			notify := make(chan int64)
			ack := make(chan bool)
			writeErr := make(chan error, 1)
			go func() {
				for i := 0; i < limit; i++ {
					tag, err := w.Exec(ctx, "UPDATE shards SET range_id=$1 WHERE shard_id=$2", int64(1000+i), sid)
					if err == nil && tag.RowsAffected() != 1 {
						err = fmt.Errorf("write affected %d rows", tag.RowsAffected())
					}
					if err != nil {
						writeErr <- err
						close(notify)
						return
					}
					select {
					case notify <- int64(1000 + i):
					case <-ctx.Done():
						close(notify)
						writeErr <- ctx.Err()
						return
					}
					if !<-ack {
						close(notify)
						writeErr <- nil
						return
					}
				}
				close(notify)
				writeErr <- nil
			}()
			for expected := range notify {
				var val int64
				if e = reader.QueryRow(ctx, "SELECT range_id FROM shards WHERE shard_id=$1", sid).Scan(&val); e != nil || val != expected {
					r.Status = "FAIL"
					r.Reason = fmt.Sprintf("read %d expected %d: %v", val, expected, e)
					ack <- false
					break
				}
				r.Observed++
				ack <- true
			}
			if e = <-writeErr; e != nil {
				r.Status = "FAIL"
				r.Reason = e.Error()
			}
			results[sid-1] = r
		}(sid)
	}
	wg.Wait()
	return results
}

func mappedShard(ctx context.Context, conn *pgx.Conn, id int) (string, error) {
	if _, e := conn.Exec(ctx, "SET rds_aurora.limitless_explain_options = 'shard_plans, single_shard_optimization'"); e != nil {
		return "", e
	}
	rows, e := conn.Query(ctx, fmt.Sprintf("EXPLAIN SELECT * FROM shards WHERE shard_id = %d", id))
	if e != nil {
		return "", e
	}
	defer rows.Close()
	var plan string
	for rows.Next() {
		var line string
		if e = rows.Scan(&line); e != nil {
			return "", e
		}
		plan += line + "\n"
	}
	if e = rows.Err(); e != nil {
		return "", e
	}
	m := shardPattern.FindStringSubmatch(plan)
	if len(m) < 2 {
		return "", fmt.Errorf("shard mapping absent for %d: %s", id, plan)
	}
	return m[1], nil
}
func a8Case(ctx context.Context, conn *pgx.Conn, id int, top string, it int) (r caseResult) {
	r = caseResult{Case: "A8", Topology: top, Isolation: "RC", Iteration: it, Status: "FAIL"}
	if _, e := conn.Exec(ctx, "SET rds_aurora.limitless_explain_options = 'shard_plans, single_shard_optimization'"); e != nil {
		r.Reason = e.Error()
		return
	}
	tx, e := conn.Begin(ctx)
	if e != nil {
		r.Reason = e.Error()
		return
	}
	defer tx.Rollback(ctx)
	queries := []string{fmt.Sprintf("EXPLAIN SELECT range_id FROM shards WHERE shard_id=%d FOR UPDATE", id), fmt.Sprintf("EXPLAIN UPDATE execs SET db_record_version=db_record_version WHERE shard_id=%d AND wf_id='main'", id), fmt.Sprintf("EXPLAIN INSERT INTO fence_log(shard_id,range_id,writer_id,seq,wf_id,written_version) VALUES(%d,5,-1,-1,'plan',0)", id)}
	for _, q := range queries {
		rows, e := tx.Query(ctx, q)
		if e != nil {
			r.Reason = e.Error()
			return
		}
		var plan string
		for rows.Next() {
			var line string
			if e = rows.Scan(&line); e != nil {
				rows.Close()
				r.Reason = e.Error()
				return
			}
			plan += line + "\n"
		}
		rows.Close()
		if !strings.Contains(plan, "Single Shard Optimized") {
			r.Reason = plan
			return
		}
	}
	var rangeID int64
	if e = tx.QueryRow(ctx, lockShardQry, id).Scan(&rangeID); e != nil {
		r.Reason = e.Error()
		return
	}
	if _, e = tx.Exec(ctx, "UPDATE execs SET db_record_version=db_record_version WHERE shard_id=$1 AND wf_id='main'", id); e != nil {
		r.Reason = e.Error()
		return
	}
	if _, e = tx.Exec(ctx, "INSERT INTO fence_log(shard_id,range_id,writer_id,seq,wf_id,written_version) VALUES($1,$2,-1,$3,'plan',0)", id, rangeID, it); e != nil {
		r.Reason = e.Error()
		return
	}
	r.Status = "PASS"
	return
}
