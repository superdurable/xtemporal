# Aurora Limitless verification tools

These tools produced the evidence behind the `postgres12_limitless` plugin and its [operator guide](../../docs/xtemporal/limitless.md). They are run by hand; there is no CI job, because AWS offers no local Limitless. For the on-demand end-to-end run of the plugin, schema, persistence suites, and functional tests, use `scripts/xtemporal/limitless-e2e.sh`.

| Path | What it checks |
| --- | --- |
| `verify/` | Standalone Go tool (`limitless-verify`). It checks the row-lock, visibility, and fencing semantics that Temporal's SQL persistence relies on, against vanilla PostgreSQL 16 (control) and Aurora Limitless. |
| `sql/semantics.sql` | Upstream DML statements run against the Limitless schema: map upserts that SET the shard key, reference-table transactions, per-workflow `buffered_events` ids, and fan-out scans. |
| `latency-compare.sh` | Shows that the functional tests skipped by the e2e script fail from round-trip latency alone, using vanilla PostgreSQL behind toxiproxy. |

## limitless-verify

```sh
cd develop/limitless/verify
docker compose up -d                       # control PostgreSQL 16 on 127.0.0.1:55432
printf 'PG16_DSN=postgres://postgres:postgres@127.0.0.1:55432/postgres?sslmode=disable\n' > .env
go build -o limitless-verify .
./limitless-verify setup --target pg16
./limitless-verify locktest --target pg16                 # Phase A: deterministic cases
./limitless-verify fence --target pg16 --duration 10m     # Phase B: randomized fencing workload
./limitless-verify check --target pg16                    # Phase B invariants
./limitless-verify report --target pg16                   # results/<target>-<timestamp>.md and .json
```

For Aurora Limitless, add the following to `.env`, then run the same subcommands with `--target limitless`. `probe` additionally prints routers, `max_connections`, collocation, and the shard_id to shard mapping.
- `LIMITLESS_DSN=postgres://<user>:<password>@<shard-group-endpoint>:5432/postgres_limitless?sslmode=require`
- `LIMITLESS_ROUTER_IPS=<ip1>,<ip2>`, passed as an environment variable. The shard group DNS name returns one router per lookup and is cached by local resolvers, so explicit router IPs are needed to pin connections to distinct routers. Find them by resolving the endpoint repeatedly through a public resolver, e.g. `dig +short @8.8.8.8 <endpoint>`.

- **Phase A** (`locktest`) runs A1–A10 and A7b. Each case runs in same-router and cross-router topologies, with RC and RR isolation where relevant, 50 iterations by default. The cases cover:
  - a blocked `FOR SHARE`/`FOR UPDATE` returning the latest version after the lock holder commits
  - RR serialization errors
  - conditional updates after a lock wait
  - lock compatibility, `NOWAIT`, and `lock_timeout`
  - verbatim `UpdateShard`
  - single-shard plans
  - 100k cross-router read-after-write checks
  - distributed deadlock detection
- **Phase B** (`fence`, `check`) runs owners and writers per `shard_id` using Temporal's `readLockShard` → `lockAndCheckExecution` → update pattern, plus random takeovers. It optionally kills connections and cancels requests. `check` verifies four invariants:
  - no write from a fenced owner after a takeover
  - monotonic `range_id`
  - per-workflow versions exactly `1..N`
  - no takeover starvation

  On Limitless, `pg_terminate_backend` is not permitted for the master user, so `--kill-conns` has no effect there. A shard group reboot (`aws rds reboot-db-shard-group`) is the effective fault.

Results on `16.13-limitless` (2 routers, 2 shards):
- Phase A: all cases pass in both topologies.
- Phase B: fencing, monotonic, and CAS hold, including across a shard group reboot.
- `23505` hard failures after a reboot come from ambiguous commits that the harness retries with the same sequence number. They are not a Limitless fault.

## Cost

A Limitless shard group bills per ACU-hour (USD 0.16 in us-west-2 at the time of testing) with a 16 ACU minimum. Delete the shard group and cluster as soon as a run ends. `scripts/xtemporal/limitless-e2e.sh` shows a complete teardown sequence.
