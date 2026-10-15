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
	if deleteSQL != "DELETE FROM executions_visibility WHERE namespace_id = $1 AND run_id = $2" {
		t.Fatal("delete predicate changed")
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
			_, err := run(context.Background(), "", true, 1, 0, 0, time.Second, tc.writeOp, tc.readQuery)
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
