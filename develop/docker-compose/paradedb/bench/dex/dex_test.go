package main

import (
	"os"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"
)

func testDataset() *Dataset {
	return &Dataset{Namespaces: 100, Rows: 200_000, HeavyRows: 50_000, Base: time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)}
}

func TestRowsAreDeterministic(t *testing.T) {
	d := testDataset()
	for _, g := range []int64{0, 1, 99_999, 200_000, 249_999} {
		a, b := d.Row(g), d.Row(g)
		if !reflect.DeepEqual(a, b) {
			t.Fatalf("row %d differs between calls", g)
		}
	}
}

func TestDistribution(t *testing.T) {
	d := testDataset()
	var open, can, system, rare, total int
	flows := map[string]int{}
	for g := int64(0); g < d.Total(); g++ {
		r := d.Row(g)
		total++
		if r.Division != nil {
			system++
			if r.FlowType != nil || r.WorkflowType == engineType {
				t.Fatalf("system row %d carries user fields", g)
			}
			continue
		}
		if r.WorkflowType != engineType || r.FlowType == nil {
			t.Fatalf("user row %d is not an Engine flow", g)
		}
		flows[*r.FlowType]++
		switch r.Status {
		case statusRunning:
			open++
			if r.Close != nil {
				t.Fatalf("open row %d has a close time", g)
			}
		case statusContinuedAsNew:
			can++
		}
		if r.Close != nil && r.Close.After(d.Base) {
			t.Fatalf("row %d closes in the future", g)
		}
		if r.Keyword["keyword3"] == rareKeyword3 {
			rare++
		}
		if _, ok := r.Keyword["FlowType"]; ok {
			t.Fatal("FlowType must live in flow_type, not sa_keyword")
		}
		perms := r.Keyword["DexWorkQueuePermissions"].([]string)
		if len(perms) < 1 || len(perms) > 3 {
			t.Fatalf("row %d has %d permissions", g, len(perms))
		}
	}
	within := func(name string, got int, want float64) {
		t.Helper()
		ratio := float64(got) / float64(total)
		if ratio < want*0.7 || ratio > want*1.3 {
			t.Fatalf("%s ratio %.5f, want about %.5f", name, ratio, want)
		}
	}
	within("open", open, 0.003)
	within("continued-as-new", can, 0.05)
	within("system", system, 0.001)
	within("rare keyword3", rare, 0.0003)
	if flows["approval"] < flows["onboarding"] || flows["onboarding"] < flows["cleanup"] || len(flows) != len(flowTypes) {
		t.Fatalf("flow distribution not skewed as configured: %v", flows)
	}
}

func TestNilValuesAreOmitted(t *testing.T) {
	d := testDataset()
	for g := int64(0); g < 5000; g++ {
		r := d.Row(g)
		for _, m := range []map[string]any{r.Keyword, r.Text, r.Number, r.Datetime, r.Bool} {
			for k, v := range m {
				if v == nil {
					t.Fatalf("row %d stores null for %s", g, k)
				}
				if l, ok := v.([]string); ok && len(l) == 0 {
					t.Fatalf("row %d stores an empty list for %s", g, k)
				}
			}
		}
		if len(r.Values()) != len(writeColumns) || len(writeColumnTypes) != len(writeColumns) {
			t.Fatal("row values do not match the column list")
		}
	}
}

func TestNamespaceLayout(t *testing.T) {
	d := testDataset()
	var sum int64
	for i := 0; i < d.Namespaces; i++ {
		sum += d.RowsIn(i)
		for _, k := range []int64{0, d.RowsIn(i) - 1} {
			if idx, _ := d.NamespaceOf(d.RowID(i, k)); idx != i {
				t.Fatalf("RowID(%d,%d) maps to namespace %d", i, k, idx)
			}
		}
	}
	if sum != d.Rows || d.RowsIn(d.HeavyIndex()) != d.HeavyRows {
		t.Fatalf("namespace row counts do not add up: %d", sum)
	}
	if idx, id := d.NamespaceOf(d.Rows); idx != d.HeavyIndex() || id != NamespaceID(0, true) {
		t.Fatal("heavy ids must map to the heavy namespace")
	}
}

func TestRoutes(t *testing.T) {
	for _, s := range append(AllShapes(), worstBTree, worstBM25, worstPromoted) {
		bm25 := strings.Contains(s.SQL, "@@@") || strings.Contains(s.SQL, "|||")
		if (s.Route == routeBM25) != bm25 {
			t.Fatalf("%s: route %s but BM25 operators present=%v", s.Name, s.Route, bm25)
		}
		if !strings.Contains(s.SQL, "namespace_id = $1") {
			t.Fatalf("%s is not namespace scoped", s.Name)
		}
		if !strings.Contains(s.SQL, `ORDER BY sort_close_time DESC, start_time DESC, run_id COLLATE "C"`) {
			t.Fatalf("%s does not use the default order", s.Name)
		}
		switch s.Name {
		case "schedules":
		case "readyz":
			if !strings.Contains(s.SQL, "namespace_division IS NULL") || !strings.Contains(s.SQL, "status <> 6") || strings.Contains(s.SQL, "'Engine'") {
				t.Fatalf("readyz must carry only the user scope and the ContinuedAsNew filter")
			}
		default:
			if !strings.Contains(s.SQL, "namespace_division IS NULL") || !strings.Contains(s.SQL, dexFilter) {
				t.Fatalf("%s lacks the user scope or Dex filter", s.Name)
			}
		}
	}
	if strings.Contains(boundedSQL, "@@@") || strings.Contains(keysetSQL, "@@@") {
		t.Fatal("bounded and keyset queries must stay on the B-tree route")
	}
}

func TestSchemaStatements(t *testing.T) {
	stmts, err := schemaStatements("../dex-profile-schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	var indexes []string
	for _, s := range stmts {
		if isIndex(s) {
			indexes = append(indexes, indexName(s))
		}
	}
	want := []string{"default_idx", "by_flow_type", "by_workflow_id", "by_namespace_division", "visibility_core_bm25"}
	if !reflect.DeepEqual(indexes, want) {
		t.Fatalf("indexes %v, want %v", indexes, want)
	}
	raw, err := os.ReadFile("../dex-profile-schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "by_flow_type_status") || strings.Contains(string(raw), "visibility_delete_tombstones") {
		t.Fatal("the v2 schema must not contain C3 or tombstones")
	}
}

func TestUpsertSQL(t *testing.T) {
	for i := 1; i <= len(writeColumns); i++ {
		if !strings.Contains(upsertSQL, "$"+strconv.Itoa(i)+"::") {
			t.Fatalf("upsert is missing parameter %d", i)
		}
	}
	for _, want := range []string{"VALUES (", "executions_visibility._version < EXCLUDED._version"} {
		if !strings.Contains(upsertSQL, want) {
			t.Fatalf("upsert lacks %q", want)
		}
	}
	if deleteSQL != "DELETE FROM executions_visibility WHERE namespace_id = $1 AND run_id = $2" {
		t.Fatal("delete must use upstream SQL visibility semantics")
	}
}

func TestRank(t *testing.T) {
	for _, c := range []struct{ n, p, want int }{{20, 50, 9}, {20, 95, 18}, {20, 99, 19}, {100, 99, 98}, {1, 99, 0}} {
		if got := rank(c.n, c.p); got != c.want {
			t.Fatalf("rank(%d, %d) = %d, want %d", c.n, c.p, got, c.want)
		}
	}
}

func TestEvaluatePaging(t *testing.T) {
	ts := func(m int) time.Time { return time.Date(2026, 1, 1, 0, m, 0, 0, time.UTC) }
	row := func(id string, closeMin, status int) pageRow {
		k := SortKey{Close: maxSortTime, Start: ts(0), RunID: id}
		if closeMin >= 0 {
			k.Close = ts(closeMin)
		}
		return pageRow{key: k, status: status}
	}
	before := map[string]pageRow{
		"stable": row("stable", 5, 2), "dup": row("dup", 6, 2), "skip": row("skip", 7, 2),
		"open": row("open", -1, 1), "gone": row("gone", 8, 2),
	}
	after := map[string]pageRow{
		"stable": row("stable", 5, 2), "dup": row("dup", 6, 2), "skip": row("skip", 7, 2),
		"open": row("open", 50, 2), "new": row("new", -1, 1),
	}
	seen := map[string]int{"stable": 1, "dup": 2, "open": 2, "new": 1}
	r := evaluatePaging(before, after, seen)
	if r.Stable != 3 || r.StableDuplicated != 1 || r.StableSkipped != 1 || r.OpenToClosed != 1 ||
		r.OpenToClosedSeen[2] != 1 || r.Deleted != 1 || r.Inserted != 1 || r.InsertedSeen != 1 || r.OtherMoved != 0 {
		t.Fatalf("unexpected classification: %+v", r)
	}
}

func TestNewRun(t *testing.T) {
	d := testDataset()
	now := time.Now()
	r := d.NewRun(7, "invoice", "team-3", "wf-x", "run-x", now, 12345)
	if r.Status != statusRunning || r.Close != nil || *r.FlowType != "invoice" || r.Keyword["keyword1"] != "team-3" ||
		r.NamespaceID != d.NamespaceIDByIndex(7) || r.Keyword["keyword2"] != "proj-7" || r.Division != nil || r.ParentWF != nil {
		t.Fatalf("unexpected new run: %+v", r)
	}
	if _, ok := r.Keyword["DexParentFlowID"]; ok {
		t.Fatal("a new top-level run has no parent")
	}
}
