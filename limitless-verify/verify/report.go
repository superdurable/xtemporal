package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func report(target, out string) error {
	lp, e := latest(out, target, "-locktest.json")
	if e != nil {
		return e
	}
	fp, e := latest(out, target, "-fence.json")
	if e != nil {
		return e
	}
	cp, e := latest(out, target, "-check.json")
	if e != nil {
		return e
	}
	var l lockResults
	var f fenceStats
	var c checkResults
	for _, x := range []struct {
		path string
		v    any
	}{{lp, &l}, {fp, &f}, {cp, &c}} {
		b, e := os.ReadFile(x.path)
		if e != nil {
			return e
		}
		if e = json.Unmarshal(b, x.v); e != nil {
			return e
		}
	}
	counts := map[string][3]int{}
	for _, r := range l.Cases {
		v := counts[r.Case]
		switch r.Status {
		case "PASS":
			v[0]++
		case "FAIL":
			v[1]++
		case "SKIP":
			v[2]++
		}
		counts[r.Case] = v
	}
	keys := make([]string, 0, len(counts))
	for k := range counts {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var s strings.Builder
	fmt.Fprintf(&s, "# %s verification — %s\n\n## Phase A\n\n| Case | PASS | FAIL | SKIP |\n|---|---:|---:|---:|\n", target, time.Now().Format(time.RFC3339))
	for _, k := range keys {
		v := counts[k]
		fmt.Fprintf(&s, "| %s | %d | %d | %d |\n", k, v[0], v[1], v[2])
	}
	var a9Checks int64
	for _, result := range l.Cases {
		if result.Case == "A9" {
			a9Checks += result.Observed
		}
	}
	fmt.Fprintf(&s, "\nA9 read-after-write checks completed: %d.\n", a9Checks)
	fmt.Fprint(&s, "\n## SQLSTATE histogram\n\n")
	for k, v := range l.SQLStates {
		fmt.Fprintf(&s, "- Phase A `%s`: %d\n", k, v)
	}
	for k, v := range f.SQLStates {
		fmt.Fprintf(&s, "- Phase B `%s`: %d\n", k, v)
	}
	fmt.Fprintf(&s, "\n## Phase B\n\n| Invariant | Result |\n|---|---|\n| Fencing | %t |\n| Monotonic range | %t |\n| CAS versions | %t |\n| Takeover liveness | %t |\n\nCommits: %d; takeovers: %d; retryable errors: %d; hard errors: %d.\n\nThroughput: %.1f commits/s; takeover lock wait p50/p99: %.2f/%.2f ms.\n", c.Fencing, c.Monotonic, c.CAS, c.Starvation, f.Commits, f.Takeovers, f.Retryable, f.HardFailures, float64(f.Commits)/f.Ended.Sub(f.Started).Seconds(), c.WaitP50MS, c.WaitP99MS)
	for _, v := range c.Violations {
		fmt.Fprintf(&s, "- %s\n", v)
	}
	name := fmt.Sprintf("%s-%s", target, time.Now().Format("20060102T150405"))
	raw := struct {
		Lock  lockResults
		Fence fenceStats
		Check checkResults
	}{l, f, c}
	b, _ := json.MarshalIndent(raw, "", "  ")
	if e = os.WriteFile(filepath.Join(out, name+".json"), b, 0644); e != nil {
		return e
	}
	if e = os.WriteFile(filepath.Join(out, name+".md"), []byte(s.String()), 0644); e != nil {
		return e
	}
	fmt.Println("report:", filepath.Join(out, name+".md"))
	return nil
}
