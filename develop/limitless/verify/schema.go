package main

import (
	"context"
	"fmt"
	"github.com/jackc/pgx/v5"
	"strings"
)

var tables = []struct{ name, ddl string }{
	{"shards", "CREATE TABLE shards(shard_id INT PRIMARY KEY, range_id BIGINT NOT NULL, data BYTEA, data_encoding TEXT)"},
	{"execs", "CREATE TABLE execs(shard_id INT, wf_id TEXT, db_record_version BIGINT NOT NULL, PRIMARY KEY(shard_id,wf_id))"},
	{"fence_log", "CREATE TABLE fence_log(shard_id INT, range_id BIGINT, writer_id INT, seq BIGINT, wf_id TEXT, written_version BIGINT, PRIMARY KEY(shard_id,writer_id,seq))"},
	{"takeover_log", "CREATE TABLE takeover_log(shard_id INT, old_range BIGINT, observed_count BIGINT, wait_ms DOUBLE PRECISION, PRIMARY KEY(shard_id,old_range))"},
	{"range_history", "CREATE TABLE range_history(shard_id INT, seq BIGINT, range_id BIGINT, PRIMARY KEY(shard_id,seq))"},
}

func setup(ctx context.Context, c *connector, shards int) error {
	conn, e := c.DialAny(ctx)
	if e != nil {
		return e
	}
	defer conn.Close(ctx)
	for i := len(tables) - 1; i >= 0; i-- {
		if _, e = conn.Exec(ctx, "DROP TABLE IF EXISTS "+tables[i].name+" CASCADE"); e != nil {
			return e
		}
	}
	for _, t := range tables {
		tx, e := conn.Begin(ctx)
		if e != nil {
			return e
		}
		defer tx.Rollback(ctx)
		if c.target == "limitless" {
			settings := []string{"SET LOCAL rds_aurora.limitless_create_table_mode='sharded'", "SET LOCAL rds_aurora.limitless_create_table_shard_key='{" + `"shard_id"` + "}'"}
			if t.name != "shards" {
				settings = append(settings, "SET LOCAL rds_aurora.limitless_create_table_collocate_with='shards'")
			}
			for _, q := range settings {
				if _, e = tx.Exec(ctx, q); e != nil {
					return fmt.Errorf("%s: %w", t.name, e)
				}
			}
		}
		if _, e = tx.Exec(ctx, t.ddl); e != nil {
			return fmt.Errorf("%s: %w", t.name, e)
		}
		if e = tx.Commit(ctx); e != nil {
			return e
		}
	}
	for i := 1; i <= shards; i++ {
		if _, e = conn.Exec(ctx, "INSERT INTO shards(shard_id,range_id) VALUES($1,5)", i); e != nil {
			return e
		}
		if _, e = conn.Exec(ctx, "INSERT INTO execs(shard_id,wf_id,db_record_version) VALUES($1,'main',0)", i); e != nil {
			return e
		}
		if _, e = conn.Exec(ctx, "INSERT INTO range_history(shard_id,seq,range_id) VALUES($1,0,5)", i); e != nil {
			return e
		}
	}
	if c.target == "limitless" {
		rows, err := conn.Query(ctx, "SELECT * FROM rds_aurora.limitless_table_collocations")
		if err != nil {
			return err
		}
		var collocations strings.Builder
		for rows.Next() {
			values, err := rows.Values()
			if err != nil {
				rows.Close()
				return err
			}
			fmt.Fprintln(&collocations, values)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return err
		}
		rows.Close()
		for _, table := range tables {
			if !strings.Contains(collocations.String(), table.name) {
				return fmt.Errorf("%s absent from limitless_table_collocations", table.name)
			}
		}
	}
	return nil
}
func resetRow(ctx context.Context, conn *pgx.Conn, id int) error {
	_, e := conn.Exec(ctx, "UPDATE shards SET range_id=5 WHERE shard_id=$1", id)
	return e
}
