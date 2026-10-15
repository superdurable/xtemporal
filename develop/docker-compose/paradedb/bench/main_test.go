package main

import (
	"strings"
	"testing"
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
