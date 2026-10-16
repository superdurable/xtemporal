package main

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestVisibilityStatements(t *testing.T) {
	if len(fields) != len(values(1, 1, true)) {
		t.Fatalf("fields=%d, values=%d", len(fields), len(values(1, 1, true)))
	}
	if fields[len(fields)-1] != "_version" {
		t.Fatal("version must be the final insert field")
	}
	if !strings.Contains(insertSQL, "ON CONFLICT (namespace_id, run_id) DO NOTHING") {
		t.Fatal("insert conflict behavior changed")
	}
	if !strings.Contains(upsertSQL, "WHERE executions_visibility._version < EXCLUDED._version") {
		t.Fatal("upsert version guard missing")
	}
	for _, field := range fields {
		if !strings.Contains(upsertSQL, field+" = excluded."+field) {
			t.Fatalf("upsert does not update %s", field)
		}
	}
	if len(jsonFields) != len(jsonValues(1, 1, true)) {
		t.Fatalf("json fields=%d, values=%d", len(jsonFields), len(jsonValues(1, 1, true)))
	}
	for _, field := range []string{"sa_keyword", "sa_text", "sa_number", "sa_datetime", "sa_bool"} {
		if !strings.Contains(jsonUpsertSQL, field+" = excluded."+field) {
			t.Fatalf("json upsert does not update %s", field)
		}
	}
	if !strings.Contains(jsonUpsertSQL, "WHERE executions_visibility._version < EXCLUDED._version") {
		t.Fatal("json upsert version guard missing")
	}
	if deleteSQL != "DELETE FROM executions_visibility WHERE namespace_id = $1 AND run_id = $2" {
		t.Fatal("delete predicate changed")
	}
}

func TestQueryRouting(t *testing.T) {
	for _, variant := range []string{"postgres", "pdb-cols", "pdb-cols-hybrid", "pdb-json-hybrid"} {
		for _, q := range queries {
			sql := querySQL(q, variant)
			if sql == "" {
				t.Fatalf("%s %s has no query", variant, q.name)
			}
			core := q.name == "status_time" || q.name == "type_status" || q.name == "count" || q.name == "group_count"
			if core && strings.Contains(variant, "hybrid") && sql != q.postgres {
				t.Fatalf("%s %s should use PostgreSQL SQL", variant, q.name)
			}
			if !core && variant == "pdb-json-hybrid" && !strings.Contains(sql, "sa_") {
				t.Fatalf("%s does not query typed JSON", q.name)
			}
		}
	}
}

func TestDiagnosticFiltersRejectUnknownValues(t *testing.T) {
	for _, tc := range []struct {
		name, writeOp, readQuery, want string
	}{
		{"write", "replace", "all", "unknown write operation"},
		{"read", "mixed", "missing", "unknown read query"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := run(context.Background(), "", "pdb-cols", 1, 0, 0, time.Second, tc.writeOp, tc.readQuery, 0)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("run error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestKeysAreStableAndScoped(t *testing.T) {
	ns1, run1 := key(1)
	ns2, run2 := key(11)
	if ns1 != ns2 || run1 == run2 {
		t.Fatal("unexpected namespace or run distribution")
	}
	if ns1 != "c4ca4238a0b923820dcc509a6f75849b" || run1 != ns1 {
		t.Fatal("preload and writer keys differ")
	}
}

func TestAttributesAreIndependentOfNamespace(t *testing.T) {
	for ns := 0; ns < 10; ns++ {
		var typeZero, tenant, tag, text int
		for id := ns; id < 10000; id += 10 {
			row := values(id, 1, true)
			if row[2] == "type-0" {
				typeZero++
			}
			attrs := row[15].(string)
			if strings.Contains(attrs, `"tenant-0"`) || strings.Contains(attrs, `"tenant-20"`) {
				tenant++
			}
			if strings.Contains(attrs, `"tag-0"`) {
				tag++
			}
			if strings.Contains(attrs, "hello world") {
				text++
			}
		}
		if typeZero != 100 || tenant != 20 || tag != 50 || text != 100 {
			t.Fatalf("namespace %d selectivity: type=%d tenant=%d tag=%d text=%d", ns, typeZero, tenant, tag, text)
		}
	}
}

func TestFixedRateReadsCountMissedWork(t *testing.T) {
	result, err := run(context.Background(), "postgres://postgres:spike@127.0.0.1:1/visibility?sslmode=disable&connect_timeout=1",
		"postgres", 10, 0, 1, 300*time.Millisecond, "mixed", "count", 100)
	if err == nil || !strings.Contains(err.Error(), "count query had 1 errors") {
		t.Fatalf("run error = %v", err)
	}
	count := result["reads"].(map[string]any)["count"].(map[string]any)
	if count["missed"].(int64) == 0 {
		t.Fatal("unserved scheduled reads must be reported as missed")
	}
	if result["read_rate"].(float64) != 100 {
		t.Fatal("read rate not recorded")
	}
}
