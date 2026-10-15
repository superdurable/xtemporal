package main

import (
	"context"
	"fmt"
	"regexp"
	"strings"
)

var shardPattern = regexp.MustCompile(`Remote Plans from Shard ([^ ]+)`)

func probe(ctx context.Context, c *connector, shards int) error {
	routers := 1
	if c.target == "limitless" {
		routers = max(2, len(c.ips))
	}
	for i := 0; i < routers; i++ {
		conn, e := c.DialRouter(ctx, i)
		if e != nil {
			return e
		}
		id, e := c.RouterIdentity(ctx, conn)
		if e != nil {
			conn.Close(ctx)
			return e
		}
		var maxCon string
		e = conn.QueryRow(ctx, "SHOW max_connections").Scan(&maxCon)
		if e != nil {
			conn.Close(ctx)
			return e
		}
		fmt.Printf("router %d: %s max_connections=%s\n", i, id, maxCon)
		if c.target == "limitless" {
			rows, e := conn.Query(ctx, "SELECT table_name FROM information_schema.views WHERE table_schema='rds_aurora' AND table_name LIKE 'limitless_%'")
			if e == nil {
				for rows.Next() {
					var n string
					if rows.Scan(&n) == nil {
						fmt.Println("view:", n)
					}
				}
				rows.Close()
			}
			rows, e = conn.Query(ctx, "SELECT routine_name FROM information_schema.routines WHERE routine_schema='rds_aurora' AND routine_name LIKE 'limitless_%'")
			if e == nil {
				for rows.Next() {
					var n string
					if rows.Scan(&n) == nil {
						fmt.Println("function:", n)
					}
				}
				rows.Close()
			}
		}
		conn.Close(ctx)
	}
	if c.target == "limitless" {
		conn, e := c.DialAny(ctx)
		if e != nil {
			return e
		}
		defer conn.Close(ctx)
		if _, e = conn.Exec(ctx, "SET rds_aurora.limitless_explain_options = 'shard_plans, single_shard_optimization'"); e != nil {
			fmt.Println("explain options:", e)
			return nil
		}
		for sid := 1; sid <= shards; sid++ {
			rows, e := conn.Query(ctx, fmt.Sprintf("EXPLAIN SELECT * FROM shards WHERE shard_id = %d", sid))
			if e != nil {
				fmt.Printf("shard %d explain: %v\n", sid, e)
				continue
			}
			var lines []string
			for rows.Next() {
				var line string
				if rows.Scan(&line) == nil {
					lines = append(lines, line)
				}
			}
			rows.Close()
			match := shardPattern.FindStringSubmatch(strings.Join(lines, "\n"))
			if len(match) > 1 {
				fmt.Printf("shard_id %d -> %s\n", sid, match[1])
			} else {
				fmt.Printf("shard_id %d: mapping unavailable\n", sid)
			}
		}
	}
	return nil
}
