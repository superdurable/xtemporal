package main

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

// Dataset describes a deterministic Dex-like visibility population. Row g is a pure function of the
// dataset parameters and g, so the preload and the writers (which must re-close preloaded open rows)
// share one source of truth.
type Dataset struct {
	Namespaces int
	Rows       int64 // rows spread round-robin across the regular namespaces
	HeavyRows  int64 // rows of the single heavy namespace, ids [Rows, Rows+HeavyRows)
	Base       time.Time
}

const (
	statusRunning        = 1
	statusCompleted      = 2
	statusFailed         = 3
	statusCanceled       = 4
	statusTerminated     = 5
	statusContinuedAsNew = 6
	statusTimedOut       = 7

	engineType      = "Engine"
	rareKeyword3    = "k3-rare"
	absentKeyword3  = "k3-absent"
	rareTextWord    = "zephyr"
	retention       = 30 * 24 * time.Hour
	openStartWindow = 48 * time.Hour
)

// Flow types with a skewed distribution; weights sum to 100.
var flowTypes = []struct {
	name   string
	weight uint64
}{
	{"approval", 30}, {"onboarding", 20}, {"invoice", 15}, {"review", 10}, {"escalation", 8},
	{"intake", 6}, {"audit", 4}, {"migration", 3}, {"report", 2}, {"cleanup", 2},
}

var permissions = []string{"perm-admin", "perm-ops", "perm-finance", "perm-legal", "perm-support", "perm-sales", "perm-eng", "perm-hr"}

var words = []string{
	"invoice", "approval", "customer", "contract", "renewal", "shipment", "refund", "escalation",
	"onboarding", "payroll", "audit", "vendor", "compliance", "review", "migration", "report",
	"urgent", "quarterly", "europe", "asia", "acme", "globex", "initech", "umbrella",
	"policy", "claim", "ticket", "deploy", "budget", "forecast", "hiring", "training",
	"security", "privacy", "billing", "pricing", "support", "outage", "backlog", "release",
}

// Columns written by the store, in insert order; sort_close_time is generated.
var writeColumns = []string{
	"namespace_id", "run_id", "workflow_id", "workflow_type_name", "status", "start_time", "close_time",
	"namespace_division", "flow_type", "task_queue", "execution_time", "parent_workflow_id", "parent_run_id",
	"root_workflow_id", "root_run_id", "history_length", "history_size_bytes", "execution_duration",
	"state_transition_count", "memo", "encoding", "_version",
	"sa_keyword", "sa_text", "sa_number", "sa_datetime", "sa_bool",
}

var writeColumnTypes = []string{
	"text", "text", "text", "text", "integer", "timestamp", "timestamp",
	"text", "text", "text", "timestamp", "text", "text",
	"text", "text", "bigint", "bigint", "bigint",
	"bigint", "bytea", "text", "bigint",
	"jsonb", "jsonb", "jsonb", "jsonb", "jsonb",
}

type Row struct {
	NamespaceID   string
	RunID         string
	WorkflowID    string
	WorkflowType  string
	Status        int
	Start         time.Time
	Close         *time.Time
	Division      *string
	FlowType      *string
	TaskQueue     string
	ParentWF      *string
	ParentRun     *string
	RootWF        string
	RootRun       string
	HistoryLength int64
	HistorySize   int64
	Duration      int64
	Transitions   int64
	Memo          []byte
	Version       int64
	Keyword       map[string]any
	Text          map[string]any
	Number        map[string]any
	Datetime      map[string]any
	Bool          map[string]any
}

func (r *Row) Values() []any {
	var closeTime any
	if r.Close != nil {
		closeTime = *r.Close
	}
	return []any{
		r.NamespaceID, r.RunID, r.WorkflowID, r.WorkflowType, r.Status, r.Start, closeTime,
		r.Division, r.FlowType, r.TaskQueue, r.Start, r.ParentWF, r.ParentRun,
		r.RootWF, r.RootRun, r.HistoryLength, r.HistorySize, r.Duration,
		r.Transitions, r.Memo, "json/plain", r.Version,
		r.Keyword, r.Text, r.Number, r.Datetime, r.Bool,
	}
}

// SortKey is the keyset position of a row in the default order.
type SortKey struct {
	Close time.Time
	Start time.Time
	RunID string
}

func (r *Row) SortKey() SortKey {
	k := SortKey{Close: maxSortTime, Start: r.Start, RunID: r.RunID}
	if r.Close != nil {
		k.Close = *r.Close
	}
	return k
}

var maxSortTime = time.Date(9999, 12, 31, 23, 59, 59, 0, time.UTC)

func mix(x uint64) uint64 {
	x += 0x9e3779b97f4a7c15
	x = (x ^ (x >> 30)) * 0xbf58476d1ce4e5b9
	x = (x ^ (x >> 27)) * 0x94d049bb133111eb
	return x ^ (x >> 31)
}

func hash(g int64, salt uint64) uint64 { return mix(uint64(g)*0x100000001b3 ^ mix(salt)) }

func md5hex(s string) string {
	sum := md5.Sum([]byte(s))
	return hex.EncodeToString(sum[:])
}

func uuidFrom(s string) string {
	h := md5hex(s)
	return h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:32]
}

// HeavyIndex is the namespace index of the heavy namespace.
func (d *Dataset) HeavyIndex() int { return d.Namespaces }

func (d *Dataset) Total() int64 { return d.Rows + d.HeavyRows }

func NamespaceID(i int, heavy bool) string {
	if heavy {
		return md5hex("dex-ns-heavy")
	}
	return md5hex(fmt.Sprintf("dex-ns-%d", i))
}

func (d *Dataset) NamespaceOf(g int64) (int, string) {
	if g >= d.Rows {
		return d.HeavyIndex(), NamespaceID(0, true)
	}
	i := int(g % int64(d.Namespaces))
	return i, NamespaceID(i, false)
}

// NamespaceIDByIndex maps an index from NamespaceOf back to its ID.
func (d *Dataset) NamespaceIDByIndex(i int) string {
	return NamespaceID(i, i == d.HeavyIndex())
}

// RowsIn returns the number of preloaded rows in namespace i.
func (d *Dataset) RowsIn(i int) int64 {
	if i == d.HeavyIndex() {
		return d.HeavyRows
	}
	n := d.Rows / int64(d.Namespaces)
	if int64(i) < d.Rows%int64(d.Namespaces) {
		n++
	}
	return n
}

// RowID returns the id of the k-th preloaded row of namespace i.
func (d *Dataset) RowID(i int, k int64) int64 {
	if i == d.HeavyIndex() {
		return d.Rows + k
	}
	return int64(i) + k*int64(d.Namespaces)
}

func pickFlow(h uint64) string {
	h %= 100
	for _, f := range flowTypes {
		if h < f.weight {
			return f.name
		}
		h -= f.weight
	}
	return flowTypes[len(flowTypes)-1].name
}

func pickStatus(h uint64) int {
	switch h %= 1000; {
	case h < 3:
		return statusRunning
	case h < 53:
		return statusContinuedAsNew
	case h < 83:
		return statusFailed
	case h < 93:
		return statusTerminated
	case h < 98:
		return statusCanceled
	case h < 100:
		return statusTimedOut
	default:
		return statusCompleted
	}
}

func isSystem(g int64) bool { return hash(g, 1)%1000 == 0 }

// Status is cheap enough for writers to scan for preloaded open rows.
func (d *Dataset) Status(g int64) int { return pickStatus(hash(g, 3)) }

func (d *Dataset) IsUserRow(g int64) bool { return !isSystem(g) }

func strptr(s string) *string { return &s }

func (d *Dataset) Row(g int64) Row {
	nsIdx, ns := d.NamespaceOf(g)
	r := Row{
		NamespaceID: ns,
		RunID:       uuidFrom(fmt.Sprintf("run-%d", g)),
		WorkflowID:  fmt.Sprintf("wf-%d", g),
		Status:      d.Status(g),
		Version:     1,
		Keyword:     map[string]any{},
		Text:        map[string]any{},
		Number:      map[string]any{},
		Datetime:    map[string]any{},
		Bool:        map[string]any{},
	}
	age := time.Duration(hash(g, 4)%uint64(retention/time.Second)) * time.Second
	if r.Status == statusRunning {
		age %= openStartWindow
	}
	r.Start = d.Base.Add(-age).Truncate(time.Microsecond)
	if r.Status != statusRunning {
		c := r.Start.Add(30*time.Second + time.Duration(hash(g, 5)%7200)*time.Second)
		if c.After(d.Base) {
			c = d.Base
		}
		r.Close = &c
	}
	r.RootWF, r.RootRun = r.WorkflowID, r.RunID
	r.HistoryLength = int64(10 + hash(g, 16)%2000)
	r.HistorySize = r.HistoryLength * 350
	r.Transitions = r.HistoryLength * 2
	if r.Close != nil {
		r.Duration = r.Close.Sub(r.Start).Nanoseconds()
	}
	r.Memo = memo(g)

	if isSystem(g) {
		div := "TemporalScheduler"
		r.WorkflowType = "temporal-sys-scheduler-workflow"
		if hash(g, 2)%10 == 0 {
			div = "TemporalBatcher"
			r.WorkflowType = "temporal-sys-batch-workflow"
		}
		r.Division = &div
		r.TaskQueue = "temporal-sys-per-ns-tq"
		return r
	}

	flow := pickFlow(hash(g, 2))
	r.FlowType = &flow
	r.WorkflowType = engineType
	r.TaskQueue = "dex-engine"
	n := 1 + hash(g, 6)%3
	start := hash(g, 7) % uint64(len(permissions))
	perms := make([]string, 0, n)
	for j := uint64(0); j < n; j++ {
		perms = append(perms, permissions[(start+j)%uint64(len(permissions))])
	}
	r.Keyword["DexWorkQueuePermissions"] = perms
	r.Keyword["keyword1"] = fmt.Sprintf("team-%d", hash(g, 8)%20)
	r.Keyword["keyword2"] = fmt.Sprintf("proj-%d", nsIdx)
	if hash(g, 9)%10000 < 3 {
		r.Keyword["keyword3"] = rareKeyword3
	} else {
		r.Keyword["keyword3"] = fmt.Sprintf("k3-%d", hash(g, 9)%50)
	}
	if hash(g, 10)%5 == 0 {
		parent := g - int64(d.Namespaces)
		if g >= d.Rows {
			parent = g - 1
		}
		if parent >= 0 {
			r.ParentWF = strptr(fmt.Sprintf("wf-%d", parent))
			r.ParentRun = strptr(uuidFrom(fmt.Sprintf("run-%d", parent)))
			r.RootWF, r.RootRun = *r.ParentWF, *r.ParentRun
			r.Keyword["DexParentFlowID"] = *r.ParentWF
		}
	}
	r.Number["int1"] = hash(g, 11) % 1000
	r.Number["int2"] = hash(g, 12) % 100000
	r.Datetime["datetime1"] = r.Start.UnixMicro() + int64(hash(g, 13)%86400)*1_000_000
	r.Bool["bool1"] = hash(g, 14)%2 == 0
	r.Text["text1"] = text(g)
	return r
}

func text(g int64) string {
	parts := make([]string, 0, 5)
	for j := uint64(0); j < 4; j++ {
		parts = append(parts, words[hash(g, 20+j)%uint64(len(words))])
	}
	if hash(g, 15)%1000 == 0 {
		parts = append(parts, rareTextWord)
	}
	return strings.Join(parts, " ")
}

func memo(g int64) []byte {
	size := 80 + hash(g, 17)%160
	seed := md5.Sum([]byte(fmt.Sprintf("memo-%d", g)))
	out := make([]byte, size)
	for i := range out {
		out[i] = seed[i%len(seed)] ^ byte(i)
	}
	return out
}

// NewRun builds a freshly started run for the write workload.
func (d *Dataset) NewRun(nsIdx int, flow, keyword1, workflowID, runID string, now time.Time, seed int64) Row {
	r := d.Row(seed % d.Rows)
	for isSystem(seed % d.Rows) {
		seed++
		r = d.Row(seed % d.Rows)
	}
	r.NamespaceID = d.NamespaceIDByIndex(nsIdx)
	r.RunID = runID
	r.WorkflowID = workflowID
	r.RootWF, r.RootRun = workflowID, runID
	r.ParentWF, r.ParentRun = nil, nil
	delete(r.Keyword, "DexParentFlowID")
	r.Keyword["keyword2"] = fmt.Sprintf("proj-%d", nsIdx)
	if keyword1 != "" {
		r.Keyword["keyword1"] = keyword1
	}
	r.FlowType = &flow
	r.Status = statusRunning
	r.Start = now.UTC().Truncate(time.Microsecond)
	r.Close = nil
	r.Duration = 0
	r.Datetime["datetime1"] = r.Start.UnixMicro()
	r.Version = now.UnixNano()
	return r
}
