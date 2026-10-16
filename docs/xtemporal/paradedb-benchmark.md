# ParadeDB visibility write benchmark

## Release gate

On the same host, with the v12 PostgreSQL visibility schema and the spike's ParadeDB schema each holding one million rows across ten namespaces, run 16, 64, and 128 writer connections for 15 minutes per database, with eight concurrent readers. The writer mix is 30% single-row running inserts, 60% version-checked single-row close upserts, and 10% single-row deletes. The readers cycle through the spike's five list predicates, count, and grouped count. An operation is one client round trip in its own implicit transaction.

The release gate is evaluated at 64 writers: ParadeDB must deliver at least 80% of PostgreSQL's successful write operations per second, with write p99 no more than twice PostgreSQL's, and p95 for each of the five list queries no more than 1.25 times PostgreSQL's. Both databases must have zero ordinary workload errors. Failure of any condition blocks release. The 16 and 128 writer cases expose saturation and are reported even though they do not set the threshold.

## Reproduction

From the repository root, run `develop/docker-compose/paradedb/bench/run.sh /absolute/path/to/output`. The script builds the isolated Go module, starts pinned `paradedb/paradedb:v0.26.0-pg17` and `postgres:17.11` containers, loads the respective schemas, preloads one million rows, runs both timed workloads, and removes its containers and volumes on success. The output directory contains JSON latency results, per-run Docker CPU and memory samples, relation and index sizes, PostgreSQL autovacuum counters, and ParadeDB segment and merge snapshots. The default run takes more than 90 minutes plus preload time. `BENCH_ROWS`, `BENCH_DURATION`, and `BENCH_CONNECTIONS` can reduce a smoke run; only the defaults qualify for the gate.

The SQL field list and conflict behavior are copied from `common/persistence/sql/sqlplugin/postgresql/visibility.go` and `sqlplugin.VisibilityRow`: insert conflicts do nothing, upserts update every field only when the new `_version` exceeds the stored one, and deletes use the composite namespace and run key. Preload is a server-side set insert so its time is excluded from the single-row measurement. Both databases use `max_connections=200` and eight readers. Stock PostgreSQL uses `shared_buffers=8GB`, `effective_cache_size=24GB`, `work_mem=80MB`, and `maintenance_work_mem=2GB`, approximately matching ParadeDB's auto-tuned 8027 MB, 24082 MB, 80 MB, and 2006 MB respectively on this Docker host. The key and attribute distributions follow the spike. Results should be interpreted with the measured affected-row count: repeated deletes of preloaded keys eventually become no-ops, while every such request still exercises the upstream delete statement and counts as a client operation.

## Results

The ParadeDB image auto-tune reported 18 CPUs and about 31 GiB of RAM on the local Docker host. The one-million-row preloads completed on both schemas before timing. PostgreSQL's initial index set occupied about 14 GiB; ParadeDB's occupied 6504 MiB, including its 30-field BM25 index.

| Engine | Writers | Duration | Successful writes/s | Write p50 / p95 / p99 (ms) | Errors | Changed rows | Avg CPU / peak CPU | Peak memory |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| PostgreSQL | 16 | 900 s | 180.0 | 38.7 / 285.5 / 673.8 | 0 | 161,874 | 1118% / 1602% | 8.54 GiB |
| PostgreSQL | 64 | 900 s | 394.0 | 63.1 / 588.9 / 1251.5 | 0 | 298,062 | 1126% / 1525% | 8.71 GiB |
| PostgreSQL | 128 | 900 s | 835.1 | 43.1 / 554.7 / 1449.0 | 0 | 599,417 | 1365% / 1990% | 9.20 GiB |
| ParadeDB v0.26.1 | 16 | 900 s | 221.1 | 26.4 / 272.4 / 657.0 | 0 | 198,847 | — | — |
| ParadeDB v0.26.1 | 64 | 900 s | 222.4 | 93.7 / 1166.3 / 2652.0 | 0 | 135,817 | — | — |

CPU percentage uses Docker's convention of 100% per core; averages are of ten-second samples. The 128-writer PostgreSQL run followed an interrupted three-minute attempt after an Orca restart, so it had a warmer and more updated database than the other scenarios and is only a saturation observation. The 64-writer release comparison is unaffected by that interruption.

Read latency p50 / p95 / p99 in milliseconds, with all seven representative queries running concurrently with writes:

| Engine / writers | Status + time | Type + status | Text | Keyword IN | Keyword list | Count | Count group-by |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| PostgreSQL / 16 | 1.0 / 9.8 / 31.3 | 0.8 / 6.0 / 20.7 | 205.2 / 538.3 / 1115.9 | 193.1 / 522.3 / 993.4 | 197.7 / 507.9 / 1007.2 | 37.7 / 144.6 / 365.4 | 240.0 / 713.0 / 1852.9 |
| PostgreSQL / 64 | 1.5 / 14.7 / 75.3 | 1.1 / 9.5 / 39.0 | 324.5 / 1083.5 / 2287.5 | 314.4 / 1047.9 / 2255.6 | 339.7 / 1222.8 / 2454.8 | 82.7 / 424.1 / 1023.9 | 328.0 / 1247.4 / 2500.9 |
| PostgreSQL / 128 | 1.1 / 17.4 / 70.4 | 0.9 / 14.0 / 44.2 | 250.4 / 758.4 / 1700.5 | 245.2 / 798.4 / 1599.8 | 246.0 / 771.4 / 1589.7 | 57.8 / 262.9 / 627.8 | 200.0 / 684.5 / 1455.4 |
| ParadeDB v0.26.1 / 16 | 71.3 / 377.4 / 706.9 | 50.4 / 273.1 / 497.6 | 51.6 / 270.1 / 441.1 | 51.9 / 270.9 / 497.9 | 52.2 / 289.1 / 520.4 | 91.7 / 568.6 / 1270.9 | 159.7 / 876.7 / 1794.1 |
| ParadeDB v0.26.1 / 64 | 344.1 / 1447.7 / 2810.8 | 180.2 / 783.4 / 1557.4 | 184.8 / 798.3 / 1491.2 | 201.4 / 831.3 / 1724.2 | 209.0 / 913.2 / 1989.9 | 617.8 / 3149.8 / 7570.1 | 925.0 / 4832.6 / 8598.3 |

PostgreSQL had zero reader errors in all three runs.

PostgreSQL's total visibility index size grew from about 14 GiB before writes to 15, 17, and 18 GiB after the 16, 64, and 128-writer runs. Its heap grew from 521 to 707 MiB. The `pg_stat_user_tables` autovacuum count was zero before and after the first run, then one after each later run; estimated dead tuples reached 280,413 after 128 writers. ParadeDB's initial `paradedb.index_info` reported eleven visible BM25 segments, and `paradedb.merge_info` returned no active merge.

**Release status: pending.** The measured PostgreSQL 64-writer baseline sets concrete ParadeDB thresholds of at least 315.2 successful writes/s, write p99 at most 2503.0 ms, and list-query p95 ceilings of 18.4 ms (status/time), 11.9 ms (type/status), 1354.4 ms (text), 1309.9 ms (keyword IN), and 1528.5 ms (keyword list). The available results do not include a qualifying 15-minute ParadeDB 64-writer run, so the comparison and release gate remain unevaluated.

## ParadeDB v0.26.1 gate run (2026-10-15)

Image `paradedb/paradedb:v0.26.1-pg17` (pg_search 0.26.1) was run with `BENCH_DATABASES=paradedb BENCH_PARADEDB_CONNECTIONS="16 64" PARADEDB_TAG=v0.26.1-pg17 develop/docker-compose/paradedb/bench/run.sh <output>`. Core dumps and full Rust backtraces were enabled. The setup was one million preloaded rows, the unchanged mixed write workload, and eight concurrent readers running all seven queries, for 15 minutes at each writer level. The Docker host also ran unrelated containers during the run: about 0.7 cores and 4 GiB for one service, 8.7 GiB for a Cassandra node, and a Kafka broker. Absolute numbers carry that noise.

- **Stability:** both 15-minute runs completed with zero write and zero read errors, and no backend exited. The only worker exits were parallel workers ending with code 1 after the harness cancelled in-flight statements at the end of each run.
- **Crash recovery:** a `SIGKILL` during a 64-writer load was followed by 151 s of WAL redo and a 251 s end-of-recovery checkpoint; the database accepted connections about 6.5 minutes after the kill. The long recovery reflects about 6 GB of WAL from the preceding 30 minutes of writes. After recovery, indexed and forced-heap counts matched exactly: namespace 102,853, status 17,192, workflow type 102,853, keyword 10,292. The log has no `ERROR`, `PANIC`, or `FATAL` entries other than `57P03` connection refusals during recovery.
- **Thresholds at 64 writers:**
  - Met: text p95 (798 ms ≤ 1354.4), keyword IN p95 (831 ms ≤ 1309.9), keyword list p95 (913 ms ≤ 1528.5).
  - Not met: write throughput (222.4/s vs ≥ 315.2), write p99 (2652 ms vs ≤ 2503.0), status/time p95 (1448 ms vs ≤ 18.4), type/status p95 (783 ms vs ≤ 11.9).
  - Write throughput was the same at 16 and 64 writers, so writes saturate on the ParadeDB side at about 220/s with this schema.
- Raw JSON results, segment and merge snapshots, and the recovery log are in `.local/plan/reports/T15-data/`.


## Schema comparison (2026-10-16)

`develop/docker-compose/paradedb/bench/compare.sh` compares four visibility variants under identical tuning. Its results are in `develop/docker-compose/paradedb/bench/results/2026-10-16-cloud-4cpu/`. The resulting schema choice and its rationale are recorded in [paradedb-schema-decision.md](paradedb-schema-decision.md).
