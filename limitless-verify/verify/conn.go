package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/url"
	"os"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
)

type connector struct {
	target, dsn, host string
	ips               []string
}

func newConnector(target, dsn string) (*connector, error) {
	u, e := url.Parse(dsn)
	if e != nil {
		return nil, e
	}
	c := &connector{target: target, dsn: dsn, host: u.Hostname()}
	if target == "limitless" {
		// The shard group DNS name returns one router IP per lookup and the
		// local resolver caches it, so explicit router IPs are required to
		// reliably pin connections to distinct routers.
		if v := os.Getenv("LIMITLESS_ROUTER_IPS"); v != "" {
			c.ips = strings.Split(v, ",")
			return c, nil
		}
		ips, e := net.LookupHost(c.host)
		if e != nil {
			return nil, e
		}
		sort.Strings(ips)
		c.ips = ips
	}
	return c, nil
}
func (c *connector) DialAny(ctx context.Context) (*pgx.Conn, error) { return pgx.Connect(ctx, c.dsn) }
func (c *connector) DialRouter(ctx context.Context, i int) (*pgx.Conn, error) {
	if c.target == "pg16" {
		return c.DialAny(ctx)
	}
	if len(c.ips) == 0 {
		return nil, fmt.Errorf("no router IPs for %s", c.host)
	}
	if len(c.ips) == 1 && i > 0 {
		first, e := c.DialAny(ctx)
		if e != nil {
			return nil, e
		}
		id, e := c.RouterIdentity(ctx, first)
		first.Close(ctx)
		if e != nil {
			return nil, e
		}
		for attempt := 0; attempt < 32; attempt++ {
			conn, e := c.DialAny(ctx)
			if e != nil {
				return nil, e
			}
			other, e := c.RouterIdentity(ctx, conn)
			if e == nil && other != id {
				return conn, nil
			}
			conn.Close(ctx)
		}
		return nil, fmt.Errorf("distinct router unavailable after 32 dials")
	}
	cfg, e := pgx.ParseConfig(c.dsn)
	if e != nil {
		return nil, e
	}
	cfg.Host = c.ips[i%len(c.ips)]
	if cfg.TLSConfig == nil {
		cfg.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	} else {
		cfg.TLSConfig = cfg.TLSConfig.Clone()
	}
	cfg.TLSConfig.ServerName = c.host
	cfg.Fallbacks = nil
	return pgx.ConnectConfig(ctx, cfg)
}
func (c *connector) RouterIdentity(ctx context.Context, conn *pgx.Conn) (string, error) {
	var addr string
	if e := conn.QueryRow(ctx, "SELECT COALESCE(inet_server_addr()::text, 'local')").Scan(&addr); e != nil {
		return "", e
	}
	if c.target == "limitless" {
		var names []string
		rows, e := conn.Query(ctx, "SELECT table_name FROM information_schema.views WHERE table_schema='rds_aurora' AND table_name LIKE 'limitless_%'")
		if e == nil {
			for rows.Next() {
				var n string
				if rows.Scan(&n) == nil {
					names = append(names, n)
				}
			}
			rows.Close()
		}
		if len(names) > 0 {
			addr += " views=" + strings.Join(names, ",")
		}
	}
	return addr, nil
}
