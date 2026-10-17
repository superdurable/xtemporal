# Dex-profile visibility benchmark results (2026-10-17, 4-CPU cloud host)

This is the result of the benchmark specified in [dex-profile-benchmark.md](../../dex-profile-benchmark.md): the json-hybrid ParadeDB design with Dex-shaped data, console polling and a version-guarded, tombstoned write mix. Only the realistic tier ran here. The 100M tier did not fit on this host (see [Deviations](#deviations-from-the-spec)).

The numbers below were measured before the review fixes in [Review corrections](#review-corrections). Those fixes do not change any recommendation, but they qualify several numbers, as noted there.

## Answers

1. **Does B-tree-prefix routing with residual JSON filters keep Studio-style polling fast?** Yes, at realistic namespace sizes. With the default prepared-statement behavior and no `by_flow_type_status`, every console shape had p50 8–11 ms and p99 53–105 ms under 16 writers (about 1,280 writes/s) with 200 browsers polling three FlowTypes each. Single queries in a 330-row namespace take 0.8–4 ms. It holds only while the planner keeps choosing the intended index, which it did not always do (below).
2. **Where does it break?**
   - **A huge flow with a residual that rarely or never matches.** `FlowType = X` matches 733k rows of the roughly 2M in the heavy namespace; a residual on an absent value scans all of them: 1.6 s idle, 2.6–3.0 s under 16 writers. The cost is linear in the flow size.
   - **Plan instability.** Without C3, the custom plan for "Need a person" in the heavy namespace is a parallel bitmap scan plus sort that takes 1.0 s warm and 9.2 s cold, while the generic plan is an ordered scan of 0.6 ms. With C3, the *generic* plan for shapes without a status equality picks `by_flow_type_status`, which breaks the sort order and costs 1.5–4.0 s.
   - **Deep keyset pages.** The keyset predicate is an OR chain, so it runs as a heap filter and each page skips everything before the cursor: 1.5 ms at offset 100, 47.6 ms at offset 8,000. Adding a redundant `sort_close_time <= $cursor` brings it back to 1.4 ms.
   - **Saturation.** On 4 cores, 64 writers plus 200 browsers keep the CPU at about 345% and push B-tree p99 to 6.7–7.1 s without C3.
3. **Is BM25 needed for the remaining shapes?** Only for Text matches and queries whose only selective filter is a custom attribute; no other shape needs it. Its cost depends on merge state: 48–144 ms with 29 segments and no writers, 1.3–1.6 s measured right after write bursts (141–170 segments), and 2–178 s p50 across the mixed phases (16 and 64 writers), where it queues behind its own 32-connection pool on a saturated host. The mixed-phase BM25 numbers are lower bounds (see [Review corrections](#review-corrections)).

## Recommendations

- **Residual-filter worst case: use a bounded B-tree scan with BM25 fallback** (scan at most 1,000 rows of the core prefix in sort order, apply the residual, and fall back to BM25 only when the bound is hit without a full page). Heavy namespace: 58 ms (absent value) and 153 ms (rare value), against 1,627 ms and 221 ms for pure residual filtering, and 49 / 144 ms for BM25 alone. Regular namespaces never fall back: 1.4 ms, against 53–59 ms for BM25. Promote an attribute to a column only when a hot, selective filter justifies it: the index takes about 1.5 GB per attribute at 12M rows and a minute to build, and brings the query to 0.4–0.8 ms.
- **C3 (`by_flow_type_status`): do not create it.** It adds 1.5–1.7 GiB (1,546–1,778 MiB), cut write throughput by 39–51% at 16 writers (1,277 to 631–783 statements/s) and 12–13% at 64 writers (1,651 to 1,430–1,447), gave no read gain at 16 writers (p50 8–11 ms without it, 25–35 ms with it using custom plans, 10.6 s using generic plans), and makes small namespaces slower too (`done_today`, 1.0 ms to 8.7 ms). Its one benefit, fixing the "Need a person" custom-plan trap, is better handled through plan choice. The write-throughput drop is not pure index maintenance: in the C3 phases, slow read plans held the read pools and most of the CPU, so part of the drop is read-side contention. The recommendation rests on the size, the missing read gain and the generic-plan trap, which do not depend on that number.
- **Plan choice.** Keep the default prepared statements (generic plans after five executions) for B-tree shapes and do not add C3. Check this on production-sized data, because both custom and generic plans were wrong for some shape in some configuration.
- **Keyset paging.** Add a redundant `sort_close_time <= $cursor` bound to the page predicate.
- **JIT.** Turning it off saves 0.15–0.3 s on slow plans (warm), and costs nothing for this workload.
- **Write lock.** The store must take the run's advisory lock in its own statement before the upsert or delete, in the same transaction. An advisory lock inside the write statement does not protect against resurrection (see [Review corrections](#review-corrections)).

## Recommended indexes

The schema as benchmarked, without `by_flow_type_status`. Sizes are after the load (12M rows); every index was used by at least one measured shape.

| Index | Size | Serves |
|---|---:|---|
| `executions_visibility_pkey (namespace_id, run_id)` | 1.4 GiB | `ON CONFLICT` of every write |
| `default_idx (namespace_id, sort_close_time DESC, start_time DESC, run_id COLLATE "C") WHERE namespace_division IS NULL` | 1.4 GiB | default list, readiness probe |
| `by_flow_type (namespace_id, flow_type, <same sort>) WHERE namespace_division IS NULL` | 1.5 GiB | main list, Need a person, Done today, Running, custom `=` and range: most console shapes |
| `by_workflow_id (namespace_id, workflow_id)` | 0.8 GiB | `WorkflowId =` |
| `by_namespace_division (namespace_id, namespace_division, <sort>) WHERE namespace_division IS NOT NULL` | 2 MiB | schedule listing |
| `visibility_core_bm25` | 2.9 GiB | Text matches, custom-attribute-only queries, the BM25 fallback |

Not created: `by_flow_type_status` (C3), and the generic `by_status`, `by_task_queue` and `by_execution_time` indexes (excluded by the spec, not re-measured). This benchmark did not test dropping any of the six, so the list is "nothing measured was unused", not a proven minimum. Status-only and workflow-type-only queries, which the spec routes to `default_idx` with a residual filter, were not benchmarked either.

## `namespace_division IS NULL` in the Dex profile: keep it

`namespace_division` marks system workflows that live in a user's namespace, such as Schedules (`TemporalScheduler`), batch operations (`TemporalBatcher`, `TemporalAdminBatcher`) and worker controllers. The server hides them from user queries by adding `namespace_division IS NULL`. These workflows also have dedicated `temporal-sys-*` workflow types, which raised the question of whether the Dex profile could drop the division filter, turn the partial indexes into ordinary indexes, and drop `by_namespace_division`. The answer is no, for four reasons:

- **Not every Dex query has the Engine filter.**
  - Only Dex Web adds `WorkflowType = "Engine"`.
  - The Dex server's `SearchFlows` only adds `ExecutionStatus != "ContinuedAsNew"`.
  - The readiness probe sends an empty query.
  - dexcli and the SDKs pass the caller's query through unchanged.
  - Without the division filter, an SDK `SearchFlows("")` would also return the namespace's system workflows, such as the Schedule that Dex uses for blob cleanup.
  - Any query with a `FlowType` condition does exclude system rows, because they have no `FlowType`. Queries without one do not.
- **The store cannot apply the filter selectively.**
  - The shared upstream converter injects the division predicate (`common/persistence/visibility/store/query/converter.go:201-231`) into every query that does not name the division.
  - The store does not know whether the caller is Dex Web, an SDK or an operator tool. Dropping the predicate would change upstream semantics for every caller.
- **There is nothing to gain.**
  - Every query implies the partial-index predicate, so the planner uses those indexes at no extra cost. Every B-tree shape above used its intended index.
  - System rows are 0.1% of the data, so ordinary indexes would be the same size and no faster.
- **`by_namespace_division` (2 MiB) still has a reader.**
  - The Dex profile turns off `ListSchedules`. The remaining reader is namespace deletion, which lists `TemporalNamespaceDivision IS NULL OR IS NOT NULL` (`service/worker/deletenamespace/deleteexecutions/activities.go:105`).
  - With this index, the store can serve the two halves as ordered scans of `default_idx` and `by_namespace_division` and merge them. Without it, the system-row half is a full sort of the namespace.
  - Only system rows enter this index, so its write cost is negligible.

## Setup

| | |
|---|---|
| Host | 4 vCPU Intel Xeon @ 2.80 GHz, 15 GB RAM, Docker 29.8.2, load generator on the same host |
| Database | `paradedb/paradedb:v0.26.1-pg17`, `shared_buffers=4GB`, `effective_cache_size=10GB`, `work_mem=80MB`, `/dev/shm=2GB`, `max_connections=200` |
| Schema | [dex-profile-schema.sql](../../dex-profile-schema.sql) as written, plus `by_flow_type_status` for the C3 phases |
| Data | 12,000,000 rows: 10,000 namespaces of 1,000 rows plus one heavy namespace of 2,000,000 rows. Every user row is an `Engine` flow with one of 10 skewed FlowTypes; 0.3% open, 5% ContinuedAsNew, 0.1% system rows (`TemporalScheduler`, `TemporalBatcher`); 0.03% carry the rare `keyword3` value. |
| Load | COPY in 260 s; index builds: BM25 460 s, `by_flow_type` 48 s, `default_idx` 45 s, `by_workflow_id` 39 s; vacuum 30 s |
| Reads | 200 browsers, each polling every 10 s for three FlowTypes: main list, Need a person, Done today, Running, custom `=`, custom range, `WorkflowId =`; plus one Text-match or custom-only BM25 query per poll, a schedule listing, and a readiness probe at 0.6 QPS. Pools of 32 connections per route, so latency includes pool wait. |
| Writes | 16 or 64 writers: starts, closes of own runs and of preloaded open rows, search-attribute upserts, continue-as-new (close plus insert), tombstoned deletes, all version-guarded |
| Phases | 5 minutes each. This run executed no C3 / 16, C3 / 16, C3 / 64, no C3 / 64, then the worst case and paging, then C3 with custom plans / 16 and 64. The custom-plan phases were added to `dex-compare.sh` after the first pass, so a fresh run executes them before the worst case. `worst-idle.json` and `worst-rerun.json` are manual `-mode worst` runs outside the script. |
| Background load | No other containers were reported, but the all-container snapshots are not in this directory. Database CPU averaged 320–354% of 400% in every phase (`mix-*-docker-stats.csv`). |

## Deviations from the spec

- **Only the 10M tier.** The host has about 28 GB of disk and the 12M-row database takes 18 GB, so 100M rows (about 150 GB) cannot run here. `dex-compare.sh` supports it with `BENCH_ROWS=95000000 BENCH_HEAVY_ROWS=5000000` on a host with at least 200 GB free. The heavy namespace here (2M rows, 733k in one flow) stands in for the "several million row" namespace.
- **5-minute phases instead of 10,** to keep disk use under control; the script aborts below 1.5 GB free.
- **Shared host.** Load generator and database share 4 cores and the CPU is saturated, so absolute numbers are pessimistic and the 64-writer phases mostly measure CPU contention.
- **Order and state drift.** The phases run on one database whose heap grows from 9.2 GB to 11.0 GB, which accumulates up to 241k tombstones, and has between 24 and 170 BM25 segments. Phase order was A16, B16, B64, A64 to balance this, and the custom-plan phases ran last, so cross-phase differences carry some order effect.
- **Polls wait for their slowest query.** A browser's next poll starts 10 s after the previous one began or when it finishes, so slow BM25 queries lower the offered QPS (late polls are reported).
- **The partial first run.** A first mixed phase was stopped after about 45 s, leaving about 12.5k extra rows and 4k tombstones (about 0.1%).
- **The promoted column is emulated** by an expression B-tree index `(namespace_id, (sa_keyword->>'keyword3'), <sort>)`, which has the same shape as an index on a typed column.

## Read and write latency

Latency in ms (p50 / p99). Full tables with p95 and QPS are in [summary.md](summary.md).

#### 16 writers: p50 / p99 (ms)

| Shape | No C3 | C3 | C3, custom plans |
|---|---:|---:|---:|
| main_list | 11.1 / 67.5 | 10636 / 22772 | 32.6 / 5037 |
| need_person | 8.5 / 56.3 | 10635 / 22771 | 25.4 / 4991 |
| done_today | 8.6 / 81.0 | 10727 / 23134 | 30.9 / 5078 |
| running | 8.1 / 52.7 | 10642 / 22780 | 25.2 / 4985 |
| custom_eq | 9.3 / 102 | 10728 / 23270 | 31.3 / 5089 |
| custom_range | 9.5 / 105 | 10731 / 23282 | 32.0 / 5102 |
| workflow_id | 8.1 / 55.3 | 10650 / 22784 | 28.0 / 5003 |
| readyz | 9.3 / 54.2 | 9237 / 20857 | 34.5 / 2654 |
| schedules | 5.4 / 46.6 | 10845 / 16929 | 20.5 / 142 |
| text_bm25 | 1986 / 13913 | 3971 / 32058 | 5882 / 23535 |
| custom_only_bm25 | 3335 / 17436 | 4004 / 37239 | 11156 / 54704 |

| | No C3 | C3 | C3, custom plans |
|---|---:|---:|---:|
| Write statements/s | 1277 | 631 | 783 |
| Write p50 / p99, start op (ms) | 9.2 / 62.4 | 21.0 / 204 | 19.2 / 105 |
| Late polls / polls | 607 / 5755 | 2591 / 4419 | 1517 / 4949 |

#### 64 writers: p50 / p99 (ms)

| Shape | No C3 | C3 | C3, custom plans |
|---|---:|---:|---:|
| main_list | 37.6 / 6685 | 22.0 / 278 | 35.5 / 405 |
| need_person | 32.9 / 7091 | 14.6 / 249 | 27.7 / 507 |
| done_today | 33.3 / 7003 | 22.8 / 872 | 36.3 / 525 |
| running | 32.7 / 7070 | 14.1 / 252 | 27.5 / 376 |
| custom_eq | 36.6 / 7051 | 23.2 / 786 | 35.5 / 541 |
| custom_range | 36.0 / 6967 | 23.6 / 784 | 35.7 / 477 |
| workflow_id | 35.6 / 6696 | 17.6 / 243 | 29.0 / 377 |
| readyz | 22.7 / 4378 | 35.4 / 179 | 19.4 / 108 |
| schedules | 23.9 / 110 | 20.3 / 95.2 | 21.4 / 95.1 |
| text_bm25 | 117721 / 196339 | 45434 / 134876 | 55951 / 103262 |
| custom_only_bm25 | 177742 / 200304 | 96791 / 130840 | 67827 / 115389 |

| | No C3 | C3 | C3, custom plans |
|---|---:|---:|---:|
| Write statements/s | 1651 | 1430 | 1447 |
| Write p50 / p99, start op (ms) | 34.1 / 162 | 41.0 / 261 | 39.4 / 257 |
| Late polls / polls | 466 / 562 | 734 / 991 | 850 / 1242 |

Observations:

- Without C3 and at 16 writers the console shapes are fast and uniform. The BM25 shapes are slow throughout: seconds, because of CPU contention and the shared pool.
- With C3 and generic plans, every B-tree shape sits at about 10.6 s p50. This is not per-shape cost: a few shapes (`done_today`, `custom_eq`, `custom_range`) run 1.5–4 s generic plans, and they occupy the 32-connection pool so everything queues behind them. Forcing custom plans removes that, leaving p50 25–35 ms but a 2.1 s p95 and 5 s p99.
- At 64 writers the no-C3 phase (last, on the most bloated state) had p99 6.7–7.1 s, while both C3 phases had p99 below 900 ms. The host is saturated in all four phases, so this does not show a C3 benefit; it shows how much state and order matter at saturation.

## Plan evidence

`EXPLAIN (ANALYZE, BUFFERS)` per shape and namespace (regular = 1,000 rows; heavy = 2M rows, 733k in the FlowType), with both the custom plan and the generic plan forced with `plan_cache_mode = force_generic_plan`. In auto mode, a cached prepared statement switches to the generic plan only when its estimated cost is no worse than the average of the custom plans seen so far. The forced generic plan is the plan a statement may settle on, not necessarily the one it does. Raw plans are in `explain-no-c3.json` and `explain-c3.json`. No B-tree shape contains `@@@`, and both BM25 shapes use a single ParadeDB scan with `TopKScanExecState`.

| Shape | Namespace | No C3, custom plan | ms | No C3, generic plan | ms | C3, custom plan | ms | C3, generic plan | ms |
|---|---|---|---:|---|---:|---|---:|---|---:|
| main_list | regular | by_flow_type (ordered scan) | 2.2 | by_flow_type (ordered scan) | 0.3 | by_flow_type (ordered scan) | 1.9 | by_flow_type (ordered scan) | 0.2 |
| main_list | heavy | by_flow_type (ordered scan) | 1.9 | by_flow_type (ordered scan) | 0.2 | by_flow_type (ordered scan) | 4.8 | by_flow_type (ordered scan) | 0.6 |
| need_person | regular | by_flow_type (ordered scan) | 4.2 | by_flow_type (ordered scan) | 0.7 | by_flow_type_status (ordered scan) | 0.2 | by_flow_type_status (ordered scan) | 0.1 |
| need_person | heavy | by_flow_type (bitmap, top-N sort) | 9157.8 | by_flow_type (ordered scan) | 0.6 | by_flow_type_status (bitmap, top-N sort) | 18.9 | by_flow_type_status (ordered scan) | 0.4 |
| done_today | regular | by_flow_type (ordered scan) | 1.0 | by_flow_type (ordered scan) | 0.7 | by_flow_type_status (bitmap, top-N sort) | 8.7 | by_flow_type_status (bitmap, top-N sort) | 0.8 |
| done_today | heavy | by_flow_type (ordered scan) | 15.7 | by_flow_type (ordered scan) | 5.3 | by_flow_type (ordered scan) | 16.2 | by_flow_type_status (bitmap, top-N sort) | 3993.4 |
| running | regular | by_flow_type (ordered scan) | 1.3 | by_flow_type (ordered scan) | 0.6 | by_flow_type_status (ordered scan) | 0.1 | by_flow_type_status (ordered scan) | 0.1 |
| running | heavy | by_flow_type (ordered scan) | 0.4 | by_flow_type (ordered scan) | 0.2 | by_flow_type_status (ordered scan) | 0.9 | by_flow_type_status (ordered scan) | 0.3 |
| custom_eq | regular | by_flow_type (ordered scan) | 1.2 | by_flow_type (ordered scan) | 0.6 | by_flow_type_status (bitmap, top-N sort) | 4.0 | by_flow_type_status (bitmap, top-N sort) | 0.9 |
| custom_eq | heavy | by_flow_type (ordered scan) | 22.8 | by_flow_type (ordered scan) | 2.9 | by_flow_type (ordered scan) | 13.0 | by_flow_type_status (bitmap, top-N sort) | 1483.4 |
| custom_range | regular | by_flow_type (ordered scan) | 1.2 | by_flow_type (ordered scan) | 1.0 | by_flow_type_status (bitmap, top-N sort) | 2.9 | by_flow_type_status (bitmap, top-N sort) | 1.1 |
| custom_range | heavy | by_flow_type (ordered scan) | 10.7 | by_flow_type (ordered scan) | 3.1 | by_flow_type (ordered scan) | 15.3 | by_flow_type_status (bitmap, top-N sort) | 1652.4 |
| workflow_id | regular | by_workflow_id (ordered scan, sort) | 0.7 | by_workflow_id (ordered scan, sort) | 0.1 | by_workflow_id (ordered scan, sort) | 0.4 | by_workflow_id (ordered scan, sort) | 0.1 |
| workflow_id | heavy | by_workflow_id (ordered scan, sort) | 1.1 | by_workflow_id (ordered scan, sort) | 0.1 | by_workflow_id (ordered scan, sort) | 1.0 | by_workflow_id (ordered scan, sort) | 0.1 |
| readyz | regular | default_idx (ordered scan) | 2.6 | default_idx (ordered scan) | 0.0 | default_idx (ordered scan) | 1.5 | default_idx (ordered scan) | 0.1 |
| readyz | heavy | default_idx (ordered scan) | 2.9 | default_idx (ordered scan) | 0.0 | default_idx (ordered scan) | 0.2 | default_idx (ordered scan) | 0.1 |
| schedules | regular | by_namespace_division (ordered scan) | 0.6 | by_namespace_division (ordered scan) | 0.0 | by_namespace_division (ordered scan) | 0.2 | by_namespace_division (ordered scan) | 0.1 |
| schedules | heavy | by_namespace_division (ordered scan) | 1.9 | by_namespace_division (ordered scan) | 0.3 | by_namespace_division (ordered scan) | 1.2 | by_namespace_division (ordered scan) | 0.3 |
| text_bm25 | regular | ParadeDB TopKScanExecState | 21.5 | — | — | ParadeDB TopKScanExecState | 49.5 | — | — |
| text_bm25 | heavy | ParadeDB TopKScanExecState | 105.3 | — | — | ParadeDB TopKScanExecState | 398.9 | — | — |
| custom_only_bm25 | regular | ParadeDB TopKScanExecState | 9.8 | — | — | ParadeDB TopKScanExecState | 42.5 | — | — |
| custom_only_bm25 | heavy | ParadeDB TopKScanExecState | 131.5 | — | — | ParadeDB TopKScanExecState | 762.4 | — | — |

The intended path was used everywhere except the entries that differ from the table above: the residual filters show as heap filters on `by_flow_type`; the "Need a person" custom plan without C3 in the heavy namespace is a parallel bitmap scan (9.2 s cold, 1.0 s warm); and with C3 the generic plans of `done_today`, `custom_eq` and `custom_range` in the heavy namespace use `by_flow_type_status` with a bitmap scan and top-N sort (1.5–4.0 s).

## Worst case: residual filtering in the heavy namespace

`FlowType = approval` matches 733k of the roughly 2M heavy-namespace rows; the residual is `keyword3 = X`, where "rare" has 234 matches and "absent" has none. p50 / p99 in ms, 20 runs each, no C3. With 20 runs, the percentile method used here takes the 19th value for both p95 and p99, so "p99" is the second-largest run. The B-tree variants ran on an auto-mode connection, so later runs may have used a generic plan. Idle means no writers and a merged BM25 index (29 segments); all values are in `worst-idle.json`, `worst.json` and `worst-rerun.json`.

| Approach | Namespace | Absent, idle | Rare, idle | Absent, 16 writers | Rare, 16 writers |
|---|---|---:|---:|---:|---:|
| B-tree + residual | heavy | 1627 / 1756 | 221 / 290 | 2638 / 3014, 3044 / 3352 | 663 / 755, 436 / 545 |
| BM25 | heavy | 49 / 52 | 144 / 176 | 167 / 356, 209 / 313 | 216 / 347, 393 / 766 |
| Bounded scan 1,000 + BM25 fallback | heavy | 58 / 66 | 153 / 191 | 229 / 426, 132 / 238 | 194 / 280, 284 / 373 |
| Bounded scan 10,000 + BM25 fallback | heavy | 111 / 126 | 207 / 227 | 267 / 389, 441 / 518 | 333 / 511, 359 / 441 |
| Promoted column + B-tree | heavy | 0.4 / 0.4 | 0.8 / 1.4 | 0.1 / 1.0, 0.1 / 3.6 | 1.3 / 4.3, 1.6 / 3.3 |
| B-tree + residual | regular | 0.9 / 1.1 | 0.8 / 1.7 | | |
| BM25 | regular | 53 / 70 | 59 / 77 | | |
| Bounded scan 1,000 or 10,000 | regular | 1.4 / 2.5 | 1.5 / 2.9 | | |
| Promoted column + B-tree | regular | 1.0 / 1.2 | 1.0 / 1.6 | | |

The two values per cell under writers are the first run and the rerun. Every bounded scan in the heavy namespace fell back to BM25 (20 of 20 runs), and none in a regular namespace did.

The promoted index took 56–60 s to build and is 1.54–1.59 GB at this size.

**BM25 latency depends on merge state.** The same query takes 1.3–1.6 s in the quiet phase of both under-writer runs (right after mixed phases that left 141–170 segments) and 48–76 ms when idle with 29 segments, measured with the harness's own SQL. The cause was not isolated; the correlation with segment count is strong. Plan for BM25 to be much slower for a while after write bursts.

## Page-token stability

Paging the heavy namespace with `flow_type = approval AND keyword1 = team-3` (a residual filter) and 8 concurrent writers that close, rewrite, insert into and delete from the same set:

| | |
|---|---|
| Set before / after | 30,084 / 117,195 rows |
| Pages / seconds | 196 / 236 |
| Rows whose sort key did not change | 6,839; duplicated **0**, skipped **0** |
| Open to closed during paging | 136, each seen exactly once (a legitimate move) |
| Moved any other way | 0 |
| Deleted / inserted during paging | 23,109 / 110,220 (358 inserted rows were reached) |
| Result | passed |

Writers made 112k starts, 112k closes, 225k rewrites (same row, higher version, unchanged sort key) and 56k tombstoned deletes during the run, with zero errors. These counts include no-op statements: only 23,109 rows were deleted, and most rewrites of deleted rows were rejected by the tombstone check. So the write pressure on the stable rows was lower than the totals suggest. Pages averaged 1.2 s under 8 concurrent writers on the saturated host. The keyset filter's cost grows with depth (see the probe below) but was only 48 ms at offset 8,000 (page 160) on an idle database, so it does not explain the average; the rest was not isolated.

## Probes behind the recommendations

Run with `psql` against the heavy namespace after the last phase; the SQL follows the shapes in `dex/queries.go`.

- **Keyset depth** (`flow_type = 'approval' AND keyword1 = 'team-3'`, cursor taken at a given offset): the plan is `Index Scan using by_flow_type` with the keyset OR chain as a `Filter`. Offset 100: 196 rows removed, 1.55 ms. Offset 8,000: 16,803 rows removed, 47.6 ms. With `AND sort_close_time <= $cursor` added, the bound becomes an `Index Cond` and offset 8,000 takes 1.4 ms with 1 row removed.
- **JIT** on the "Need a person" custom plan (heavy namespace, no C3), warm: 1.16–1.33 s with JIT on (0.65 s of JIT time) and 1.01 s with JIT off. The cold run was 9.3 s, so most of the 9 s was cold reads, not JIT.

## Storage

| Snapshot (chronological) | Heap MiB | B-tree MiB | BM25 MiB | BM25 segments | Tombstones |
|---|---:|---:|---:|---:|---:|
| After load | 9,232 | 5,102 | 2,901 | 4 | 0 |
| After no C3 / 16 | 9,470 | 5,404 | 2,901 | 53 | 37,261 |
| After C3 / 16 | 9,595 | 7,077 (C3: 1,546) | 2,901 | 29 | 53,726 |
| After C3 / 64 | 9,879 | 7,451 (C3: 1,706) | 2,901 | 140 | 91,316 |
| After no C3 / 64 | 10,212 | 5,939 | 2,901 | 141 | 133,848 |
| End of the first pass (after worst case and paging, C3 dropped) | 10,534 | 6,183 | 2,902 | 24 | 183,682 |
| After C3, custom plans / 16 | 10,687 | 7,893 (C3: 1,617) | 2,902 | 41 | 203,765 |
| After C3, custom plans / 64 | 10,973 | 8,251 (C3: 1,778) | 2,902 | 170 | 240,870 |

The BM25 index stays at 2.9 GB through the mixed phases; the B-tree indexes grow with the data and `by_flow_type_status` adds 1.5–1.7 GiB. The tombstone table grew by 16k–70k rows per phase, which is the cost of tombstoned deletes (10% of the write operations here); this benchmark does not prune it.

## Review corrections

A review after the run found these issues. Fixes 1, 2, 6 and 7 are in the harness now; the numbers above predate all of them.

1. **Tombstone race in the write path. Fixed.**
   - **Cause:** the upsert took its advisory lock in a CTE inside the write statement. Under READ COMMITTED, a statement's snapshot predates any lock it takes, and the planner evaluates the uncorrelated tombstone check as an InitPlan before the lock CTE.
   - **Effect:** a stale write that ran concurrently with a delete could resurrect the deleted run. This was reproduced on `paradedb/paradedb:v0.26.1-pg17`: the old form left `1 stale` row, and the new form leaves none.
   - **Fix:** writers now send `pg_advisory_xact_lock` as its own statement, followed by the write, in one pgx batch. pgx runs a batch as a single implicit transaction, so the write's snapshot is taken after the lock is held.
   - **Impact on the results:** the published write rates come from a cheaper, incorrect write. The paging test passed, but this race could in principle have produced spurious skips.
2. **No-op writes counted as writes. Fixed.** Writes that affect no row (stale version or tombstoned) are now counted separately as `noops`. Preloaded closes use a fixed version, so they become no-ops in later phases.
3. **Right-censored, closed-loop reads. Not fixed.**
   - Queries still running when a phase ends are dropped.
   - Each browser waits for its slowest query before starting its next poll.
   - In the slow phases, BM25 p50 values of 2–178 s and the 10.6 s C3 generic-plan p50 are therefore lower bounds, measured at a fraction of the offered load. For example, the 64-writer no-C3 phase completed 562 of the expected polls, against 5,755 at 16 writers.
4. **C3 write cost is confounded. Not fixed.** See the C3 recommendation. Isolating it needs a write-only phase with and without the index.
5. **Forced generic plans in EXPLAIN. Not fixed.** See [Plan evidence](#plan-evidence). `boundedSQL` was never explained.
6. **Percentiles. Fixed.** They now use the nearest rank (`ceil(n·p/100)`), and sample latency is taken before the recorder's mutex.
7. **Other harness changes.**
   - The readiness probe now matches the Dex server: an empty `SearchFlows` with only the ContinuedAsNew filter and no Engine filter.
   - The paging test uses the recommended redundant `sort_close_time <= $cursor` bound.
   - `dex-compare.sh` now also runs on macOS, which lacks `nproc`, `free` and GNU `df`.

## Not measured

- The 100M-row tier, and any host larger than 4 vCPU.
- A load generator on a separate host.
- Crash recovery for the json-hybrid schema.
- Custom-attribute `ORDER BY` and `STARTS_WITH`, which are unsupported by design.
- The generic-versus-custom plan choice on production-sized data. This is the main follow-up.
