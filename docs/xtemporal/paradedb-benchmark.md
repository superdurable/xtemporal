# ParadeDB visibility write and recovery benchmark

## Release gate

On the same host, with the v12 PostgreSQL visibility schema and the spike's ParadeDB schema each holding one million rows across ten namespaces, run 16, 64, and 128 writer connections for 15 minutes per database, with eight concurrent readers. The writer mix is 30% single-row running inserts, 60% version-checked single-row close upserts, and 10% single-row deletes. The readers cycle through the spike's five list predicates, count, and grouped count. An operation is one client round trip in its own implicit transaction.

The release gate is evaluated at 64 writers: ParadeDB must deliver at least 80% of PostgreSQL's successful write operations per second, with write p99 no more than twice PostgreSQL's, and p95 for each of the five list queries no more than 1.25 times PostgreSQL's. Both databases must have zero ordinary workload errors. After an ungraceful `SIGKILL` during writes, ParadeDB must restart without recovery errors and indexed counts must equal heap counts for namespace, status, workflow type, and keyword predicates. Failure of any condition blocks release. The 16 and 128 writer cases expose saturation and are reported even though they do not set the threshold.

## Reproduction

From the repository root, run `develop/docker-compose/paradedb/bench/run.sh /absolute/path/to/output`. The script builds the isolated Go module, starts pinned `paradedb/paradedb:v0.26.0-pg17` and `postgres:17.11` containers, loads the respective schemas, preloads one million rows, runs both timed workloads, performs the crash check, and removes its containers and volumes on success. It retains failed-run containers and volumes for diagnosis. The output directory contains JSON latency results, per-run Docker CPU and memory samples, relation and index sizes, PostgreSQL autovacuum counters, ParadeDB segment and merge snapshots, and the crash recovery log. The default run takes more than 90 minutes plus preload time. `BENCH_ROWS`, `BENCH_DURATION`, and `BENCH_CONNECTIONS` can reduce a smoke run; only the defaults qualify for the gate. If the timed workload fails before its planned crash check, restore a fresh one-million-row ParadeDB database and run `develop/docker-compose/paradedb/bench/crash.sh /absolute/path/to/output` for the standalone durability probe.

The SQL field list and conflict behavior are copied from `common/persistence/sql/sqlplugin/postgresql/visibility.go` and `sqlplugin.VisibilityRow`: insert conflicts do nothing, upserts update every field only when the new `_version` exceeds the stored one, and deletes use the composite namespace and run key. Preload is a server-side set insert so its time is excluded from the single-row measurement. Both databases use `max_connections=200` and eight readers. Stock PostgreSQL uses `shared_buffers=8GB`, `effective_cache_size=24GB`, `work_mem=80MB`, and `maintenance_work_mem=2GB`, approximately matching ParadeDB's auto-tuned 8027 MB, 24082 MB, 80 MB, and 2006 MB respectively on this Docker host. The key and attribute distributions follow the spike. Results should be interpreted with the measured affected-row count: repeated deletes of preloaded keys eventually become no-ops, while every such request still exercises the upstream delete statement and counts as a client operation.

## Results

The ParadeDB image auto-tune reported 18 CPUs and about 31 GiB of RAM on the local Docker host. The one-million-row preloads completed on both schemas before timing. PostgreSQL's initial index set occupied about 14 GiB; ParadeDB's occupied 6504 MiB, including its 30-field BM25 index. A planned clean ParadeDB restart after preload took about seven minutes: WAL redo was logged at 136 seconds and the end-of-recovery checkpoint completed later. That preload restart is not the crash test below.

| Engine | Writers | Duration | Successful writes/s | Write p50 / p95 / p99 (ms) | Errors | Changed rows | Avg CPU / peak CPU | Peak memory |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| PostgreSQL | 16 | 900 s | 180.0 | 38.7 / 285.5 / 673.8 | 0 | 161,874 | 1118% / 1602% | 8.54 GiB |
| PostgreSQL | 64 | 900 s | 394.0 | 63.1 / 588.9 / 1251.5 | 0 | 298,062 | 1126% / 1525% | 8.71 GiB |
| PostgreSQL | 128 | 900 s | 835.1 | 43.1 / 554.7 / 1449.0 | 0 | 599,417 | 1365% / 1990% | 9.20 GiB |
| ParadeDB | 16 | 44 s, aborted by server process failure | 1.7 before failure | 1243.0 / 30,714.9 / 31,060.9 | 16 writes, 8 reads | 75 | 2629% / 4678% | 2.10 GiB |
| ParadeDB | 64 | Not run after 16-writer failure | — | — | — | — | — | — |
| ParadeDB | 128 | Not run after 16-writer failure | — | — | — | — | — | — |

CPU percentage uses Docker's convention of 100% per core; averages are of ten-second samples. The 128-writer PostgreSQL run followed an interrupted three-minute attempt after an Orca restart, so it had a warmer and more updated database than the other scenarios and is only a saturation observation. The 64-writer release comparison is unaffected by that interruption. The ParadeDB 16-writer latency figures are censored by failure and must not be treated as steady-state latency.

Read latency p50 / p95 / p99 in milliseconds, with all seven representative queries running concurrently with writes:

| Engine / writers | Status + time | Type + status | Text | Keyword IN | Keyword list | Count | Count group-by |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| PostgreSQL / 16 | 1.0 / 9.8 / 31.3 | 0.8 / 6.0 / 20.7 | 205.2 / 538.3 / 1115.9 | 193.1 / 522.3 / 993.4 | 197.7 / 507.9 / 1007.2 | 37.7 / 144.6 / 365.4 | 240.0 / 713.0 / 1852.9 |
| PostgreSQL / 64 | 1.5 / 14.7 / 75.3 | 1.1 / 9.5 / 39.0 | 324.5 / 1083.5 / 2287.5 | 314.4 / 1047.9 / 2255.6 | 339.7 / 1222.8 / 2454.8 | 82.7 / 424.1 / 1023.9 | 328.0 / 1247.4 / 2500.9 |
| PostgreSQL / 128 | 1.1 / 17.4 / 70.4 | 0.9 / 14.0 / 44.2 | 250.4 / 758.4 / 1700.5 | 245.2 / 798.4 / 1599.8 | 246.0 / 771.4 / 1589.7 | 57.8 / 262.9 / 627.8 | 200.0 / 684.5 / 1455.4 |
| ParadeDB / 16, before failure | 2733.6 / 4460.9 / 4460.9 | 710.4 / 1457.2 / 1457.2 | 821.9 / 2566.8 / 2566.8 | 1201.7 / 2939.7 / 2939.7 | 1313.6 / 3051.1 / 3051.1 | 7123.0 / 13,783.3 / 13,783.3 | 3000.6 / 11,507.0 / 11,507.0 |

The ParadeDB read samples contain only 10–16 successful calls per query, plus three failed count calls and five failed grouped-count calls. They describe the failure window, not a statistically useful p99. PostgreSQL had zero reader errors in all three runs.

PostgreSQL's total visibility index size grew from about 14 GiB before writes to 15, 17, and 18 GiB after the 16, 64, and 128-writer runs. Its heap grew from 521 to 707 MiB. The `pg_stat_user_tables` autovacuum count was zero before and after the first run, then one after each later run; estimated dead tuples reached 280,413 after 128 writers. ParadeDB's initial `paradedb.index_info` reported eleven visible BM25 segments, and `paradedb.merge_info` returned no active merge. The process failure prevented a post-run segment or size snapshot on that first ParadeDB volume.

At 22:04:59 UTC during the 16-writer ParadeDB run, the database log reported `server process (PID 19979) exited with exit code 2`, followed by termination of other backends and process recovery. The container itself was not OOM-killed. The client observed `unexpected EOF` on writes and count queries. The failed-run JSON and captured log excerpt are under `.local/plan/reports/T9-data/`. The original runner removed that volume on its error path before consistency verification; the runner now retains failed-run volumes.

For the separate durability check, a fresh database with the same schema and one million rows received 16 writers and eight readers. After ten seconds and 6,358 successful writes, `docker kill --signal KILL` stopped the ParadeDB container. It restarted and reached ready state about 15 seconds after the kill. Indexed and forced-heap counts matched exactly after recovery: namespace 100,129, running status 14,411, workflow type 100,129, and keyword 10,012. The expected connection failures during the kill appear in the recovery log; there were no `ERROR`, `PANIC`, or `FATAL` entries after the database became ready. The container and volume were removed after the successful verification. This establishes consistency for the checked predicates after one deliberate crash; it does not erase the spontaneous backend failure under sustained reads and writes.

**Release verdict: fail.** The measured PostgreSQL 64-writer baseline sets concrete ParadeDB thresholds of at least 315.2 successful writes/s, write p99 at most 2503.0 ms, and list-query p95 ceilings of 18.4 ms (status/time), 11.9 ms (type/status), 1354.4 ms (text), 1309.9 ms (keyword IN), and 1528.5 ms (keyword list). The candidate could not sustain even 16 writers with eight readers for 15 minutes, so the zero-error requirement fails before a 64-writer throughput or latency comparison is possible. The 64- and 128-writer ParadeDB measurements are unavailable because continuing the unsafe workload would not establish a steady-state result. The plugin should not ship from this schema/version combination until the backend exit is diagnosed and the full gate is rerun successfully.

## Investigation paused

A diagnostic rerun on `paradedb/paradedb:v0.26.0-pg17` loaded one million rows, then ran the unchanged mixed writer workload with 16 writers and all seven read queries with eight readers for 120 seconds. It completed 40,053 writes (333.76/s) and approximately 700 successful calls per read query with zero workload errors. The previous backend exit at 44 seconds was **not reproduced** in this run. PostgreSQL `debug1` messages, verbose errors, statement logging, Rust backtraces, and core dumps were enabled; no backend crash, signal, or core occurred. The diagnostic run's logs and JSON are in `.local/plan/reports/T14-data/`.

`develop/docker-compose/paradedb/bench/diagnose.sh` and `diagnose.yml` capture logs, container state, index segments, and merge state. The harness now accepts `-write-op` and `-read-query` to isolate insert, upsert, delete, or an individual read query. The v0.26.1 and v0.25.11 images were downloaded but **not run**. The [v0.25.11 release notes](https://github.com/paradedb/paradedb/releases/tag/v0.25.11) mention a fix for a crash when a search scan is cancelled or terminated; there is no evidence yet that it explains the observed exit. No version or configuration has passed the full 15-minute 16- and 64-writer gate. **Verdict remains NO-SHIP.**
