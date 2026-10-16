package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5"
)

type fenceStats struct {
	Target                                                                                                                                    string
	Started, Ended                                                                                                                            time.Time
	Commits, Takeovers, OwnershipLost, Retryable, HardFailures, UpdateShard, KillAttempts, Killed, CancelAttempts, Reconnects, RedialFailures int64
	SQLStates                                                                                                                                 map[string]int
	Errors                                                                                                                                    []string
}
type counters struct {
	commits, takeovers, lost, retry, hard, updates, kills, killed, cancels, reconnects, redialFailures atomic.Int64
	mu                                                                                                 sync.Mutex
	states                                                                                             map[string]int
	errs                                                                                               []string
	pids                                                                                               map[int]map[uint32]struct{}
}

func rollback(tx pgx.Tx) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err := tx.Rollback(ctx)
	if errors.Is(err, pgx.ErrTxClosed) {
		return nil
	}
	return err
}

func (v *counters) writerPID(sid int, pid uint32, add bool) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.pids[sid] == nil {
		v.pids[sid] = map[uint32]struct{}{}
	}
	if add {
		v.pids[sid][pid] = struct{}{}
	} else {
		delete(v.pids[sid], pid)
	}
}

func (v *counters) randomWriterPID(sid int) uint32 {
	v.mu.Lock()
	defer v.mu.Unlock()
	for pid := range v.pids[sid] {
		return pid
	}
	return 0
}

func (v *counters) record(e error) {
	if e == nil {
		return
	}
	s := state(e)
	v.mu.Lock()
	v.states[s]++
	if len(v.errs) < 30 {
		v.errs = append(v.errs, e.Error())
	}
	v.mu.Unlock()
	if s == "23505" {
		v.hard.Add(1)
	} else if s == "40001" || s == "40P01" || s == "connection" || s == "57014" || s == "57P01" || s == "57P03" || s == "53300" || strings.HasPrefix(s, "08") {
		v.retry.Add(1)
	} else {
		v.hard.Add(1)
	}
}
func fence(ctx context.Context, c *connector, out string, shards, writers int, duration, minWait, maxWait time.Duration, kill bool, cancelRate float64) error {
	if shards < 1 || writers < 1 || minWait <= 0 || maxWait < minWait || cancelRate < 0 || cancelRate > 1 {
		return errors.New("invalid fence flags")
	}
	if e := setup(ctx, c, shards); e != nil {
		return e
	}
	start := time.Now()
	runCtx, stop := context.WithTimeout(ctx, duration)
	defer stop()
	v := &counters{states: map[string]int{}, pids: map[int]map[uint32]struct{}{}}
	var all sync.WaitGroup
	for sid := 1; sid <= shards; sid++ {
		sid := sid
		all.Add(1)
		go func() { defer all.Done(); shardLoop(runCtx, c, sid, writers, minWait, maxWait, kill, cancelRate, v) }()
	}
	all.Wait()
	stats := fenceStats{Target: c.target, Started: start, Ended: time.Now(), Commits: v.commits.Load(), Takeovers: v.takeovers.Load(), OwnershipLost: v.lost.Load(), Retryable: v.retry.Load(), HardFailures: v.hard.Load(), UpdateShard: v.updates.Load(), KillAttempts: v.kills.Load(), Killed: v.killed.Load(), CancelAttempts: v.cancels.Load(), Reconnects: v.reconnects.Load(), RedialFailures: v.redialFailures.Load(), SQLStates: v.states, Errors: v.errs}
	file := filepath.Join(out, fmt.Sprintf("%s-%s-fence.json", c.target, start.Format("20060102T150405")))
	b, _ := json.MarshalIndent(stats, "", "  ")
	if e := os.WriteFile(file, b, 0644); e != nil {
		return e
	}
	fmt.Printf("fence results: %s commits=%d takeovers=%d retryable=%d hard=%d\n", file, stats.Commits, stats.Takeovers, stats.Retryable, stats.HardFailures)
	return nil
}
func shardLoop(ctx context.Context, c *connector, sid, writers int, minWait, maxWait time.Duration, kill bool, cancelRate float64, v *counters) {
	var wg sync.WaitGroup
	spawn := func(rng int64) {
		for j := 0; j < writers; j++ {
			wg.Add(1)
			go func(j int) { defer wg.Done(); writerLoop(ctx, c, sid, rng, int(rng)*writers+j, cancelRate, v) }(j)
		}
	}
	spawn(5)
	conn, ok := redial(ctx, c.DialAny, v)
	if !ok {
		wg.Wait()
		return
	}
	defer func() {
		if conn != nil {
			conn.Close(context.Background())
		}
	}()
	rng := int64(5)

	for ctx.Err() == nil {
		delta := minWait
		if maxWait > minWait {
			delta += time.Duration(rand.Int63n(int64(maxWait - minWait)))
		}
		timer := time.NewTimer(delta)
		select {
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
			wg.Wait()
			return
		}
		opCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		tx, e := conn.Begin(opCtx)
		if e != nil {
			cancel()
			v.record(e)
			continue
		}
		var old, count int64
		lockStart := time.Now()
		e = tx.QueryRow(opCtx, lockShardQry, sid).Scan(&old)
		waitMS := float64(time.Since(lockStart).Microseconds()) / 1000
		if e == nil {
			e = tx.QueryRow(opCtx, "SELECT count(*) FROM fence_log WHERE shard_id=$1 AND range_id=$2", sid, old).Scan(&count)
		}
		if e == nil {
			_, e = tx.Exec(opCtx, updateShardQry, old+1, []byte(nil), "", sid)
		}
		if e == nil {
			_, e = tx.Exec(opCtx, "INSERT INTO takeover_log(shard_id,old_range,observed_count,wait_ms) VALUES($1,$2,$3,$4)", sid, old, count, waitMS)
		}
		if e == nil {
			_, e = tx.Exec(opCtx, "INSERT INTO range_history(shard_id,seq,range_id) SELECT $1,COALESCE(max(seq),0)+1,$2 FROM range_history WHERE shard_id=$1", sid, old+1)
		}
		if e == nil {
			e = tx.Commit(opCtx)
		} else {
			if re := rollback(tx); re != nil {
				v.record(re)
			}
		}
		cancel()
		if e != nil {
			v.record(e)
			if conn.IsClosed() || connBroken(e) {
				conn.Close(context.Background())
				var ok bool
				if conn, ok = redial(ctx, c.DialAny, v); !ok {
					break
				}
				v.reconnects.Add(1)
			}
			continue
		}
		v.takeovers.Add(1)
		rng = old + 1
		spawn(rng)
		if kill && rand.Intn(100) == 0 {
			pid := v.randomWriterPID(sid)
			if pid == 0 {
				continue
			}
			v.kills.Add(1)
			// Backend PIDs are local to a Limitless router, so try every router.
			for ri := 0; ri < 2; ri++ {
				killer, ke := c.DialRouter(ctx, ri)
				if ke == nil {
					var ok bool
					ke = killer.QueryRow(ctx, "SELECT pg_terminate_backend($1)", pid).Scan(&ok)
					killer.Close(ctx)
					if ke == nil && ok {
						v.killed.Add(1)
					}
				}
				if ke != nil {
					fmt.Printf("pg_terminate_backend unsupported or failed: %v\n", ke)
				}
			}
		}
	}
	wg.Wait()
}

// connBroken reports errors after which the connection should not be reused.
func connBroken(e error) bool {
	s := state(e)
	return s == "connection" || s == "57P01" || s == "57P03" || strings.HasPrefix(s, "08")
}

// redial keeps retrying with backoff so a shard group reboot does not end the workload.
func redial(ctx context.Context, dial func(context.Context) (*pgx.Conn, error), v *counters) (*pgx.Conn, bool) {
	backoff := 200 * time.Millisecond
	for ctx.Err() == nil {
		conn, e := dial(ctx)
		if e == nil {
			return conn, true
		}
		v.record(e)
		v.redialFailures.Add(1)
		timer := time.NewTimer(backoff)
		select {
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
			return nil, false
		}
		backoff = min(2*backoff, 5*time.Second)
	}
	return nil, false
}

func writerLoop(ctx context.Context, c *connector, sid int, owned int64, writerID int, cancelRate float64, v *counters) {
	dial := func(ctx context.Context) (*pgx.Conn, error) { return c.DialRouter(ctx, writerID%2) }
	conn, ok := redial(ctx, dial, v)
	if !ok {
		return
	}
	defer func() {
		if conn != nil {
			conn.Close(context.Background())
		}
	}()
	pid := conn.PgConn().PID()
	v.writerPID(sid, pid, true)
	defer func() { v.writerPID(sid, pid, false) }()
	reconnect := func() bool {
		v.writerPID(sid, pid, false)
		conn.Close(context.Background())
		var ok bool
		if conn, ok = redial(ctx, dial, v); !ok {
			return false
		}
		v.reconnects.Add(1)
		pid = conn.PgConn().PID()
		v.writerPID(sid, pid, true)
		return true
	}
	var seq int64
	var expected int64
	for ctx.Err() == nil {
		opCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
		injectCancel := cancelRate > 0 && rand.Float64() < cancelRate
		tx, e := conn.Begin(opCtx)
		if e != nil {
			cancel()
			v.record(e)
			if (conn.IsClosed() || connBroken(e)) && !reconnect() {
				return
			}
			continue
		}
		var got, version int64
		e = tx.QueryRow(opCtx, readLockShardQry, sid).Scan(&got)
		if e == nil && got != owned {
			if re := rollback(tx); re != nil {
				v.record(re)
			}
			cancel()
			v.lost.Add(1)
			return
		}
		if e == nil {
			e = tx.QueryRow(opCtx, "SELECT db_record_version FROM execs WHERE shard_id=$1 AND wf_id='main' FOR UPDATE", sid).Scan(&version)
		}
		if e == nil && injectCancel {
			v.cancels.Add(1)
			cancel()
		}
		if e == nil && expected != version {
			expected = version
			if re := rollback(tx); re != nil {
				v.record(re)
			}
			cancel()
			continue
		}
		if e == nil {
			_, e = tx.Exec(opCtx, "UPDATE execs SET db_record_version=$1 WHERE shard_id=$2 AND wf_id='main'", version+1, sid)
		}
		if e == nil {
			_, e = tx.Exec(opCtx, "INSERT INTO fence_log(shard_id,range_id,writer_id,seq,wf_id,written_version) VALUES($1,$2,$3,$4,'main',$5)", sid, owned, writerID, seq+1, version+1)
		}
		if e == nil {
			e = tx.Commit(opCtx)
		} else {
			if re := rollback(tx); re != nil {
				v.record(re)
			}
		}
		cancel()
		if e != nil {
			v.record(e)
			if (conn.IsClosed() || connBroken(e)) && !reconnect() {
				return
			}
			continue
		}
		seq++
		expected = version + 1
		v.commits.Add(1)
		if seq%20 == 0 {
			updateShardVariant(ctx, conn, sid, owned, v)
		}
	}
}
func updateShardVariant(ctx context.Context, conn *pgx.Conn, sid int, owned int64, v *counters) {
	opCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	tx, e := conn.Begin(opCtx)
	if e != nil {
		v.record(e)
		return
	}
	defer func() {
		if re := rollback(tx); re != nil {
			v.record(re)
		}
	}()
	var current int64
	e = tx.QueryRow(opCtx, lockShardQry, sid).Scan(&current)
	if e != nil {
		v.record(e)
		return
	}
	if current != owned {
		v.lost.Add(1)
		return
	}
	if _, e = tx.Exec(opCtx, updateShardQry, owned, []byte(nil), "", sid); e != nil {
		v.record(e)
		return
	}
	if _, e = tx.Exec(opCtx, "INSERT INTO range_history(shard_id,seq,range_id) SELECT $1,COALESCE(max(seq),0)+1,$2 FROM range_history WHERE shard_id=$1 HAVING $2 > max(range_id)", sid, owned); e != nil {
		v.record(e)
		return
	}
	if e = tx.Commit(opCtx); e != nil {
		v.record(e)
		return
	}
	v.updates.Add(1)
}
