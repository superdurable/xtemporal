package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

type checkResults struct {
	Target                              string
	At                                  time.Time
	Fencing, Monotonic, CAS, Starvation bool
	Violations                          []string
	Commits, Takeovers, HardFailures    int64
	WaitP50MS, WaitP99MS                float64
}

func check(ctx context.Context, c *connector, out string, shards, minTakeovers int) error {
	conn, e := c.DialAny(ctx)
	if e != nil {
		return e
	}
	defer conn.Close(ctx)
	r := checkResults{Target: c.target, At: time.Now(), Fencing: true, Monotonic: true, CAS: true, Starvation: true}
	if p, err := latest(out, c.target, "-fence.json"); err == nil {
		var stats fenceStats
		if b, err := os.ReadFile(p); err == nil && json.Unmarshal(b, &stats) == nil {
			r.HardFailures = stats.HardFailures
			if stats.HardFailures > 0 {
				r.Violations = append(r.Violations, fmt.Sprintf("%d hard SQL failures", stats.HardFailures))
			}
		}
	}
	rows, e := conn.Query(ctx, "SELECT t.shard_id,t.old_range,t.observed_count,count(f.*) FROM takeover_log t LEFT JOIN fence_log f ON f.shard_id=t.shard_id AND f.range_id=t.old_range GROUP BY t.shard_id,t.old_range,t.observed_count")
	if e != nil {
		return e
	}
	for rows.Next() {
		var sid int
		var rng, want, got int64
		if e = rows.Scan(&sid, &rng, &want, &got); e != nil {
			rows.Close()
			return e
		}
		if got != want {
			r.Fencing = false
			r.Violations = append(r.Violations, fmt.Sprintf("shard %d range %d fence count %d != %d", sid, rng, got, want))
		}
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	for sid := 1; sid <= shards; sid++ {
		var current, takeovers int64
		e = conn.QueryRow(ctx, "SELECT range_id FROM shards WHERE shard_id=$1", sid).Scan(&current)
		if e != nil {
			return e
		}
		e = conn.QueryRow(ctx, "SELECT count(*) FROM takeover_log WHERE shard_id=$1", sid).Scan(&takeovers)
		if e != nil {
			return e
		}
		r.Takeovers += takeovers
		if takeovers < int64(minTakeovers) {
			r.Starvation = false
			r.Violations = append(r.Violations, fmt.Sprintf("shard %d has %d takeovers", sid, takeovers))
		}
		if current != 5+takeovers {
			r.Monotonic = false
			r.Violations = append(r.Violations, fmt.Sprintf("shard %d range %d != 5+%d", sid, current, takeovers))
		}
		history, e := conn.Query(ctx, "SELECT range_id FROM range_history WHERE shard_id=$1 ORDER BY seq", sid)
		if e != nil {
			return e
		}
		last := int64(4)
		for history.Next() {
			var val int64
			if e = history.Scan(&val); e != nil {
				history.Close()
				return e
			}
			if val <= last || val > last+1 {
				r.Monotonic = false
				r.Violations = append(r.Violations, fmt.Sprintf("shard %d history %d after %d", sid, val, last))
			}
			last = val
		}
		history.Close()
		if last != current {
			r.Monotonic = false
			r.Violations = append(r.Violations, fmt.Sprintf("shard %d history ends at %d vs %d", sid, last, current))
		}
		var version int64
		e = conn.QueryRow(ctx, "SELECT db_record_version FROM execs WHERE shard_id=$1 AND wf_id='main'", sid).Scan(&version)
		if e != nil {
			return e
		}
		var n, minv, maxv, distinct int64
		e = conn.QueryRow(ctx, "SELECT count(*),COALESCE(min(written_version),0),COALESCE(max(written_version),0),count(DISTINCT written_version) FROM fence_log WHERE shard_id=$1 AND wf_id='main'", sid).Scan(&n, &minv, &maxv, &distinct)
		if e != nil {
			return e
		}
		r.Commits += n
		if n != version || distinct != n || (n > 0 && (minv != 1 || maxv != n)) {
			r.CAS = false
			r.Violations = append(r.Violations, fmt.Sprintf("shard %d versions count=%d distinct=%d min=%d max=%d db=%d", sid, n, distinct, minv, maxv, version))
		}
	}
	waits := []float64{}
	rows, e = conn.Query(ctx, "SELECT wait_ms FROM takeover_log")
	if e != nil {
		return e
	}
	for rows.Next() {
		var ms float64
		if e = rows.Scan(&ms); e != nil {
			rows.Close()
			return e
		}
		waits = append(waits, ms)
	}
	rows.Close()
	sort.Float64s(waits)
	if len(waits) > 0 {
		r.WaitP50MS = waits[len(waits)/2]
		r.WaitP99MS = waits[min(len(waits)-1, len(waits)*99/100)]
	}
	file := filepath.Join(out, fmt.Sprintf("%s-%s-check.json", c.target, r.At.Format("20060102T150405")))
	b, _ := json.MarshalIndent(r, "", "  ")
	if e = os.WriteFile(file, b, 0644); e != nil {
		return e
	}
	fmt.Printf("check: fencing=%t monotonic=%t CAS=%t starvation=%t commits=%d takeovers=%d wait p50/p99=%.2f/%.2fms\n", r.Fencing, r.Monotonic, r.CAS, r.Starvation, r.Commits, r.Takeovers, r.WaitP50MS, r.WaitP99MS)
	if len(r.Violations) > 0 {
		return fmt.Errorf("%d invariant violations: %v", len(r.Violations), r.Violations[:min(5, len(r.Violations))])
	}
	return nil
}
