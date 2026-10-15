package limitless

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/require"
	"github.com/superdurable/xtemporal/common/persistence"
)

type column struct {
	name, typ, nullable, defaultValue string
	unique                            bool
}

type table struct {
	columns []column
	pk      []string
	uniques [][]string
}

type index struct {
	table   string
	columns []string
	unique  bool
}

type conversion struct {
	kind   string
	keys   []string
	anchor string
}

type parsedSchema struct {
	tables      map[string]table
	indexes     map[string]index
	conversions map[string]conversion
	statements  []string
	createdAt   map[string]int
	convertedAt map[string]int
}

var (
	createTable  = regexp.MustCompile(`(?is)^CREATE TABLE\s+(\w+)\s*\((.*)\)$`)
	createIndex  = regexp.MustCompile(`(?i)^CREATE\s+(UNIQUE\s+)?INDEX\s+(\w+)\s+ON\s+(\w+)\s*\(([^)]+)\)$`)
	callTable    = regexp.MustCompile(`(?i)^CALL rds_aurora\.limitless_alter_table_type_(sharded|reference)\('public\.(\w+)'(?:,\s*ARRAY\[([^]]+)\])?(?:,\s*'public\.(\w+)')?\)$`)
	primaryKey   = regexp.MustCompile(`(?i)^PRIMARY KEY\s*\(([^)]+)\)$`)
	tableUnique  = regexp.MustCompile(`(?i)^(?:CONSTRAINT\s+\w+\s+)?UNIQUE\s*\(([^)]+)\)$`)
	columnType   = regexp.MustCompile(`(?i)^(\w+)\s+([A-Z]+(?:\(\d+\))?)(.*)$`)
	defaultValue = regexp.MustCompile(`(?i)\bDEFAULT\s+(.+)$`)
)

func splitTopLevel(s string) []string {
	var parts []string
	depth := 0
	quoted := false
	start := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\'':
			quoted = !quoted
		case '(':
			if !quoted {
				depth++
			}
		case ')':
			if !quoted {
				depth--
			}
		case ',':
			if !quoted && depth == 0 {
				parts = append(parts, strings.TrimSpace(s[start:i]))
				start = i + 1
			}
		default:
		}
	}
	return append(parts, strings.TrimSpace(s[start:]))
}

func names(s string) []string {
	parts := strings.Split(s, ",")
	for i := range parts {
		parts[i] = strings.ToLower(strings.Trim(strings.TrimSpace(parts[i]), "'"))
	}
	return parts
}

func parseSchema(t *testing.T, data []byte) parsedSchema {
	t.Helper()
	stmts, err := persistence.LoadAndSplitQueryFromReaders([]io.Reader{bytes.NewReader(data)})
	require.NoError(t, err)
	result := parsedSchema{tables: map[string]table{}, indexes: map[string]index{}, conversions: map[string]conversion{}, statements: stmts, createdAt: map[string]int{}, convertedAt: map[string]int{}}
	for pos, stmt := range stmts {
		stmt = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(stmt), ";"))
		if m := createTable.FindStringSubmatch(stmt); m != nil {
			name := strings.ToLower(m[1])
			_, exists := result.tables[name]
			require.False(t, exists, "duplicate table %s", name)
			tbl := table{}
			for _, part := range splitTopLevel(m[2]) {
				if match := primaryKey.FindStringSubmatch(part); match != nil {
					tbl.pk = names(match[1])
					continue
				}
				if match := tableUnique.FindStringSubmatch(part); match != nil {
					tbl.uniques = append(tbl.uniques, names(match[1]))
					continue
				}
				if strings.HasPrefix(strings.ToUpper(part), "CONSTRAINT ") {
					continue
				}
				match := columnType.FindStringSubmatch(part)
				require.NotNil(t, match, "unparsed column: %s", part)
				tail := strings.ToUpper(match[3])
				col := column{name: strings.ToLower(match[1]), typ: strings.ToUpper(match[2]), nullable: "implicit", unique: strings.Contains(tail, "UNIQUE")}
				if strings.Contains(tail, "NOT NULL") {
					col.nullable = "not null"
				} else if strings.Contains(tail, "NULL") {
					col.nullable = "null"
				}
				if def := defaultValue.FindStringSubmatch(match[3]); def != nil {
					col.defaultValue = strings.TrimSpace(def[1])
				}
				if strings.Contains(tail, "PRIMARY KEY") {
					tbl.pk = []string{col.name}
				}
				if col.unique {
					tbl.uniques = append(tbl.uniques, []string{col.name})
				}
				tbl.columns = append(tbl.columns, col)
			}
			result.tables[name] = tbl
			result.createdAt[name] = pos
			continue
		}
		if m := createIndex.FindStringSubmatch(stmt); m != nil {
			result.indexes[strings.ToLower(m[2])] = index{table: strings.ToLower(m[3]), columns: names(m[4]), unique: m[1] != ""}
			continue
		}
		if m := callTable.FindStringSubmatch(stmt); m != nil {
			name := strings.ToLower(m[2])
			_, exists := result.conversions[name]
			require.False(t, exists, "duplicate conversion %s", name)
			c := conversion{kind: strings.ToLower(m[1]), anchor: strings.ToLower(m[4])}
			if m[3] != "" {
				c.keys = names(m[3])
			}
			result.conversions[name] = c
			result.convertedAt[name] = pos
		}
	}
	return result
}

func containsAll(columns, keys []string) bool {
	set := map[string]bool{}
	for _, col := range columns {
		set[col] = true
	}
	for _, key := range keys {
		if !set[key] {
			return false
		}
	}
	return true
}

func findColumn(tbl table, name string) (column, bool) {
	for _, col := range tbl.columns {
		if col.name == name {
			return col, true
		}
	}
	return column{}, false
}

func expectedConversions() map[string]conversion {
	result := map[string]conversion{}
	add := func(kind, anchor string, keys []string, tables ...string) {
		for _, name := range tables {
			tableAnchor := anchor
			if name == anchor {
				tableAnchor = ""
			}
			result[name] = conversion{kind: kind, anchor: tableAnchor, keys: keys}
		}
	}
	add("sharded", "shards", []string{"shard_id"}, "shards", "executions", "current_executions", "current_chasm_executions", "buffered_events", "history_immediate_tasks", "history_scheduled_tasks", "transfer_tasks", "timer_tasks", "replication_tasks", "replication_tasks_dlq", "visibility_tasks", "activity_info_maps", "timer_info_maps", "child_execution_info_maps", "request_cancel_info_maps", "signal_info_maps", "signals_requested_sets", "chasm_node_maps", "history_node", "history_tree")
	add("sharded", "tasks", []string{"range_hash"}, "tasks", "task_queues", "tasks_v2", "task_queues_v2")
	add("sharded", "task_queue_user_data", []string{"task_queue_name"}, "task_queue_user_data", "build_id_to_task_queue")
	add("sharded", "queues", []string{"queue_type", "queue_name"}, "queues", "queue_messages")
	add("sharded", "cluster_membership", []string{"membership_partition"}, "cluster_membership")
	add("reference", "", nil, "namespaces", "namespace_metadata", "cluster_metadata_info", "nexus_endpoints", "nexus_endpoints_partition_status", "queue", "queue_metadata")
	return result
}

func TestLimitlessSchema(t *testing.T) {
	data, err := os.ReadFile("temporal/schema.sql")
	require.NoError(t, err)
	baseline, err := os.ReadFile("temporal/versioned/v1.19/schema.sql")
	require.NoError(t, err)
	require.Equal(t, data, baseline)
	got := parseSchema(t, data)
	expected := expectedConversions()
	require.Len(t, got.tables, len(expected))
	require.Equal(t, expected, got.conversions)
	for name, tbl := range got.tables {
		c, exists := expected[name]
		require.True(t, exists, "unmapped table %s", name)
		require.Equal(t, got.createdAt[name]+1, got.convertedAt[name], "conversion must immediately follow %s", name)
		if c.kind != "sharded" {
			continue
		}
		require.True(t, containsAll(tbl.pk, c.keys), "%s primary key", name)
		for _, u := range tbl.uniques {
			require.True(t, containsAll(u, c.keys), "%s unique %v", name, u)
		}
		for _, key := range c.keys {
			col, ok := findColumn(tbl, key)
			require.True(t, ok, "%s.%s", name, key)
			require.NotEqual(t, "BYTEA", col.typ, "%s.%s", name, key)
			if c.anchor != "" && c.anchor != name {
				require.Less(t, got.convertedAt[c.anchor], got.convertedAt[name], "anchor %s", c.anchor)
				anchorCol, ok := findColumn(got.tables[c.anchor], key)
				require.True(t, ok)
				require.Equal(t, anchorCol.typ, col.typ, "%s.%s", name, key)
			}
		}
	}
	for name, idx := range got.indexes {
		if idx.unique {
			require.True(t, containsAll(idx.columns, expected[idx.table].keys), "%s", name)
			pos := -1
			for i, stmt := range got.statements {
				if strings.Contains(strings.ToLower(stmt), "index "+name+" ") {
					pos = i
					break
				}
			}
			require.Greater(t, pos, got.convertedAt[idx.table])
		}
	}
}

func TestLimitlessV12Drift(t *testing.T) {
	upstreamData, err := os.ReadFile("../postgresql/v12/temporal/schema.sql")
	require.NoError(t, err)
	limitlessData, err := os.ReadFile("temporal/schema.sql")
	require.NoError(t, err)
	upstream := parseSchema(t, upstreamData)
	limitless := parseSchema(t, limitlessData)
	require.Len(t, limitless.tables, len(upstream.tables))
	for name, want := range upstream.tables {
		got, ok := limitless.tables[name]
		require.True(t, ok, "missing %s", name)
		if name == "buffered_events" {
			for i := range want.columns {
				if want.columns[i].name == "id" {
					want.columns[i].typ = "BIGINT"
					want.columns[i].defaultValue = ""
					want.columns[i].unique = false
				}
			}
			want.uniques = nil
		}
		require.Equal(t, want, got, "table %s drifted", name)
	}
	require.Len(t, limitless.indexes, len(upstream.indexes))
	for name, want := range upstream.indexes {
		got, ok := limitless.indexes[name]
		require.True(t, ok, "missing index %s", name)
		if name == "cm_idx_rolehost" {
			want.columns = []string{"membership_partition", "role", "host_id"}
		}
		require.Equal(t, want, got, "index %s drifted", name)
	}
}

func TestLimitlessSchemaOnPostgres(t *testing.T) {
	dsn := os.Getenv("XTEMPORAL_LIMITLESS_TEST_DSN")
	if dsn == "" {
		t.Skip("XTEMPORAL_LIMITLESS_TEST_DSN is unset")
	}
	data, err := os.ReadFile("temporal/schema.sql")
	require.NoError(t, err)
	parsed := parseSchema(t, data)
	db, err := sql.Open("pgx", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	conn, err := db.Conn(t.Context())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, conn.Close()) })
	schemaName := fmt.Sprintf("limitless_schema_test_%d_%d", os.Getpid(), time.Now().UnixNano())
	_, err = conn.ExecContext(t.Context(), "CREATE SCHEMA "+schemaName)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, err := db.ExecContext(context.Background(), "DROP SCHEMA "+schemaName+" CASCADE")
		require.NoError(t, err)
	})
	_, err = conn.ExecContext(t.Context(), "SET search_path TO "+schemaName+", public")
	require.NoError(t, err)
	var hasProcedure bool
	err = conn.QueryRowContext(t.Context(), "SELECT EXISTS (SELECT 1 FROM pg_proc p JOIN pg_namespace n ON n.oid = p.pronamespace WHERE n.nspname = 'rds_aurora' AND p.proname = 'limitless_alter_table_type_reference')").Scan(&hasProcedure)
	require.NoError(t, err)
	for _, stmt := range parsed.statements {
		if !hasProcedure && strings.HasPrefix(strings.ToUpper(strings.TrimSpace(stmt)), "CALL RDS_AURORA.") {
			continue
		}
		if hasProcedure {
			stmt = strings.ReplaceAll(stmt, "public.", schemaName+".")
		}
		_, err = conn.ExecContext(t.Context(), stmt)
		require.NoError(t, err, "statement: %s", stmt)
	}
	rows, err := conn.QueryContext(t.Context(), "SELECT table_name FROM information_schema.tables WHERE table_schema = $1", schemaName)
	require.NoError(t, err)
	seen := map[string]bool{}
	for rows.Next() {
		var name string
		require.NoError(t, rows.Scan(&name))
		seen[name] = true
	}
	require.NoError(t, rows.Err())
	require.NoError(t, rows.Close())
	for name := range parsed.tables {
		require.True(t, seen[name], "missing %s", name)
	}
	if hasProcedure {
		typeRows, err := conn.QueryContext(t.Context(), "SELECT table_name, table_type, distribution_key::text FROM rds_aurora.limitless_tables WHERE schema_name = $1", schemaName)
		require.NoError(t, err)
		actualTypes := map[string]string{}
		for typeRows.Next() {
			var name, kind string
			var distribution sql.NullString
			require.NoError(t, typeRows.Scan(&name, &kind, &distribution))
			actualTypes[name] = kind
			if expectedConversions()[name].kind == "sharded" {
				for _, key := range expectedConversions()[name].keys {
					require.Contains(t, distribution.String, key)
				}
			}
		}
		require.NoError(t, typeRows.Err())
		require.NoError(t, typeRows.Close())
		for name, c := range expectedConversions() {
			require.Equal(t, c.kind, actualTypes[name], name)
		}
		collocationRows, err := conn.QueryContext(t.Context(), "SELECT table_name, collocation_id::text FROM rds_aurora.limitless_table_collocations WHERE schema_name = $1", schemaName)
		require.NoError(t, err)
		groups := map[string]string{}
		for collocationRows.Next() {
			var name, id string
			require.NoError(t, collocationRows.Scan(&name, &id))
			groups[name] = id
		}
		require.NoError(t, collocationRows.Err())
		require.NoError(t, collocationRows.Close())
		for name, c := range expectedConversions() {
			if c.kind == "reference" {
				require.NotContains(t, groups, name)
				continue
			}
			require.NotEmpty(t, groups[name], name)
			if c.anchor != "" {
				require.Equal(t, groups[c.anchor], groups[name], name)
			}
		}
		anchors := []string{"shards", "tasks", "task_queue_user_data", "queues", "cluster_membership"}
		anchorGroups := map[string]bool{}
		for _, anchor := range anchors {
			require.False(t, anchorGroups[groups[anchor]], "shared group for %s", anchor)
			anchorGroups[groups[anchor]] = true
		}
	}
}
