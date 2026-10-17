# Dex-profile visibility benchmark (json-hybrid, 10M and 100M rows)

This is the self-contained input for the next ParadeDB visibility benchmark. It collects the design decisions, ParadeDB 0.26.1 behavior, the exact SQL shapes the future store will generate, and the Dex workload the benchmark must model. The earlier decision is in [paradedb-schema-decision.md](../../../../docs/xtemporal/paradedb-schema-decision.md); the existing harness is in this directory (`compare.sh`, `main.go`, `results/2026-10-16-cloud-4cpu/`).

## Goal

Measure the Dex-profile design at realistic shape and scale, using SQL that matches what the store will generate. Answer three questions:

1. Does B-tree-prefix routing with residual JSON filters keep console polling fast?
2. Where does it break?
3. Is BM25 needed for the remaining shapes?

Two results feed open decisions:

- whether to create `by_flow_type_status` (C3);
- the latency and throughput targets for the store.

## Why the previous numbers are not representative

1. The current plugin (`x/paradedb/query_converter.go:28-30`) rewrites `namespace_id =` into `namespace_id @@@`. That forces every query through BM25, so core queries never reach a B-tree. The new store removes this rewrite.
2. The old `default_idx` was built on `COALESCE(close_time, …)` and `run_id` without `COLLATE "C"`. The query orders by `sort_close_time DESC, start_time DESC, run_id COLLATE "C"`, so the planner could not use the index. The indexes below match the query order exactly.
3. The old benchmark used hand-written core SQL without `COLLATE "C"` and without `namespace_division IS NULL`. Its json-hybrid table also lacked the parent/root columns the store reads.

## Decisions already made

| Item | Decision |
| --- | --- |
| Search attribute model | Elasticsearch-style: attributes are registered cluster-wide under their real names, with no per-namespace aliases and no slot limits, and list values are allowed. |
| Core columns | `namespace_id`, `run_id`, `workflow_id`, `workflow_type_name`, `status`, `start_time`, `close_time`, generated `sort_close_time`, `namespace_division` (`TemporalNamespaceDivision`). |
| `FlowType` | The Dex `FlowType` attribute is promoted to a typed `flow_type` column, with a hard-coded mapping. |
| Everything else | Stored in typed JSONB columns: `sa_keyword` (Keyword and KeywordList), `sa_text`, `sa_number`, `sa_datetime` (epoch microseconds), `sa_bool`. This includes the Temporal predefined attributes. |
| `DexWorkQueuePermissions` (R1) | Stays in JSON and is evaluated as a residual filter next to the FlowType/status B-tree. A `text[]` column with GIN is the fallback. |
| Routing (R2) | A query with a usable core prefix uses the B-tree, with the other conditions as residual SQL. A Text match, or a query whose only selective filter is a custom attribute, uses BM25. |
| `ContinuedAsNew` filter (C2) | `status <> 6` is a residual filter. The partial indexes filter on division only. |
| Generic indexes (C4) | No `by_status`, `by_task_queue` or `by_execution_time` index in the Dex profile. |
| `by_flow_type_status` (C3) | **Open:** decided by this benchmark. |
| Unsupported | Custom-attribute `ORDER BY` and `STARTS_WITH` return InvalidArgument. Do not benchmark them. |
| Count | Disabled for users in the Dex profile (`xtemporal.frontend.disableCountWorkflowExecutions`). Internal Count (namespace deletion) still runs. It is not part of the read mix. |

## Schema

[`dex-profile-schema.sql`](dex-profile-schema.sql) is the exact DDL. Benchmark two variants: without `by_flow_type_status` (as written) and with it (uncomment).

### Write rules

The data generator and the write workload must follow the same rules as the store:

- **Nil values are omitted.** Absent keys are never stored as JSON `null`.
- **KeywordList values are JSON arrays.** Empty arrays are omitted.
- **Datetime attributes are UTC epoch microseconds,** stored as JSON numbers.
- **`FlowType` is written to `flow_type`** and does not appear in `sa_keyword`.
- **Writes are version-guarded.** Every write is `INSERT … ON CONFLICT (namespace_id, run_id) DO UPDATE … WHERE executions_visibility._version < EXCLUDED._version`.
- **Deletes are tombstoned.** A delete records `(namespace_id, run_id, _version)` in `visibility_delete_tombstones`, so a stale write cannot recreate the row. Report tombstone table growth.
- **Writes and deletes of a run are serialized.** Both take `pg_advisory_xact_lock` on the run as a separate statement before the write, in the same transaction. A lock taken inside the write statement does not work: under READ COMMITTED the statement's snapshot predates the lock.

### `TemporalNamespaceDivision`

`TemporalNamespaceDivision` separates system rows from user rows. System rows include Schedules (`TemporalScheduler`), the batcher and worker deployments, and they carry a non-null division.

- Every user-facing query carries an implicit `namespace_division IS NULL`.
- Schedule listing uses `namespace_division = 'TemporalScheduler'`.

## ParadeDB 0.26.1 behavior to respect

These rules come from an executed spike on `pg_search` 0.26.1 (PostgreSQL 17.11).

- **BM25 index settings.**
  - Core text columns and `sa_keyword` use `pdb.literal`, which keeps case and punctuation.
  - `sa_text` uses `pdb.whitespace`.
  - `sa_number`, `sa_datetime` and `sa_bool` use `pdb.whitespace('columnar=true')`. Without `columnar=true`, a JSON range fails with "RangeQuery on JSON is only supported for fast fields".
- **One ParadeDB index per table.** At most 32 fields.
- **Use a custom plan for BM25 queries.**
  - Run BM25 queries with `plan_cache_mode=force_custom_plan`. A generic prepared plan with JSON keys either raises an assertion or loses the ParadeDB Custom Scan.
  - Keep JSON keys as SQL string literals and bind only the values.
- **KeywordList `=` means membership.**
  - On BM25: `sa_keyword->'K' @@@ pdb.term(v)`.
  - On SQL: `sa_keyword->'K' ? v`.
  - `sa_keyword->'K' = '"v"'::jsonb` does not match array members.
- **ES-style negation includes rows where the key is missing.**
  - On BM25: `NOT (path @@@ pdb.term(v))`.
  - On SQL: `coalesce(NOT (positive), true)`.
  - Plain SQL `<>` excludes missing keys.
- **Numbers and dates.** Doubles must use `double precision`, not `numeric`. Int ranges use `(sa_number->>'K')::bigint`, and datetime ranges use `(sa_datetime->>'K')::bigint`. Both push down to the columnar JSON field.
- **Sort pushdown.** The core sort `sort_close_time DESC, start_time DESC, run_id COLLATE "C" LIMIT n` uses `TopKScanExecState` on BM25. Ordering by a JSON key falls back to a PostgreSQL sort, which is why it is unsupported.
- **Broken on this release.** JSON regex returns wrong results, and `phrase_prefix` silently truncates at 50 expansions. `STARTS_WITH` is unsupported for this reason.
- **Fixed cost.** BM25 has a fixed cost of about 60 ms per query in earlier runs. With small namespaces, that fixed cost dominates. This is why core-prefix queries must avoid `@@@`.

## SQL shapes to generate

`$ns` is the namespace ID. Every query has `namespace_id = $ns AND namespace_division IS NULL` outside the parentheses that hold the user predicates. The Dex server adds `status <> 6` (6 = ContinuedAsNew) to every query. Dex Web also adds `workflow_type_name = 'Engine'`; the readiness probe, dexcli and the SDKs do not.

### Default order and keyset paging

```sql
ORDER BY sort_close_time DESC, start_time DESC, run_id COLLATE "C"
LIMIT $n
-- next page; the redundant first bound makes the leading sort key an index condition:
AND sort_close_time <= $c
AND ((sort_close_time < $c)
  OR (sort_close_time = $c AND start_time < $s)
  OR (sort_close_time = $c AND start_time = $s AND run_id COLLATE "C" > $r))
```

### B-tree route: no `@@@` anywhere

```sql
SELECT ... FROM executions_visibility
WHERE namespace_id = $1 AND namespace_division IS NULL
  AND flow_type = $2 AND workflow_type_name = 'Engine' AND status <> 6
  AND sa_keyword->'DexWorkQueuePermissions' ?| $3::text[]      -- KeywordList membership
  AND sa_keyword->>'keyword2' = $4                               -- Keyword
  AND (sa_number->>'int1')::bigint >= $5                         -- Int range
  AND (sa_datetime->>'datetime1')::bigint < $6                   -- Datetime range
ORDER BY sort_close_time DESC, start_time DESC, run_id COLLATE "C"
LIMIT 50;
```

### BM25 route

Use it only for a Text match, or when a custom attribute is the only selective filter. Run with `SET plan_cache_mode = force_custom_plan`.

```sql
SELECT ... FROM executions_visibility
WHERE namespace_id = $1 AND namespace_id @@@ pdb.term($1::text)
  AND namespace_division IS NULL
  AND workflow_type_name = 'Engine' AND status <> 6
  AND sa_keyword->'CustomKeywordField' @@@ pdb.term($2::text)     -- or: sa_text->'text1' ||| $2
ORDER BY sort_close_time DESC, start_time DESC, run_id COLLATE "C"
LIMIT 50;
```

### Route selection

1. **A Text match anywhere uses BM25.**
2. **A core prefix uses the B-tree.**
   - A core prefix is a `FlowType =`/`IN`, `WorkflowId =`/`IN`, or `ExecutionStatus =`/`IN` that every satisfying branch implies. For an `OR`, every arm needs one.
   - `FlowType` uses `by_flow_type`, or `by_flow_type_status` if it exists.
   - `WorkflowId` uses `by_workflow_id`.
   - `status` alone uses `default_idx` with a residual filter.
3. **Core-only conditions use `default_idx`.** This covers an empty query, `WorkflowType`, a time range and `status <> 6`. None of them uses `@@@`.
4. **Otherwise, only custom attributes are selective, so the query uses BM25.**

## Dex workload

### Callers

The source is a survey of the Dex server, Dex Web and the Dex console. It covers source code only, not traffic traces.

- **The Dex server.**
  - `SearchFlows` is its only visibility read, and it calls `ListWorkflowExecutions`. There is no Count, Scan, or ListOpen/ListClosed.
  - Every query becomes `(<caller query>) AND ExecutionStatus != "ContinuedAsNew"`, or just the status filter when the caller query is empty.
  - The default page size is 1000. Dex never adds `ORDER BY` and relies on the default newest-first order and on replayable page tokens.
- **Dex Web v2.**
  - Queries look like `FlowType = 'T'`, plus optional `(DexWorkQueuePermissions = 'p1' OR DexWorkQueuePermissions = 'p2' …)`, `WorkflowId =`, `ExecutionStatus =`, `StartTime`/`CloseTime` ranges, and application attributes.
  - Dex Web wraps them in `WorkflowType = "Engine"`.
  - The page size is at most 50. The Work Queue page defaults to `ExecutionStatus = 'Running'`.
- **The readiness probe.** It sends an empty query (only the ContinuedAsNew filter, no Engine filter) with page size 1. Only the platform Dex Web calls it, at about **0.2 QPS per replica**.
- **The Dex console Runs and Inbox pages.** These are the hot user-facing queries. The console polls **every 10 s while the browser tab is visible**: page size 50, first page only, no `ORDER BY`.
  - **Runs** runs, per displayed FlowType:
    - the main list;
    - a "Need a person" group: `status = Running` plus a `DexWorkQueuePermissions` membership;
    - a "Done today" group: `CloseTime >= today`;
    - a Running group;
    - other status groups.
  - **Inbox** is the main list plus a permissions filter, optionally with `ExecutionStatus = 'Running'`.
  - **Estimated load per visible browser:** Runs about **0.5·F QPS** and Inbox about **0.1·F QPS**, where F is the number of FlowTypes shown.
- **dexcli and the SDKs (Go, Java, Python, Rust, TypeScript).** They pass arbitrary query strings through. They are a compatibility surface, not a measured load.
- **Project deletion.** It queries `FlowType = 'X' AND keyword2 = '<project>' AND keyword3 IN (...)`. This is low frequency.

### Search attributes Dex writes

| Attribute | Type | Use |
| --- | --- | --- |
| `FlowType` | Keyword, promoted to `flow_type` | Written once at Flow/SubFlow start. Almost every query has `=`. |
| `DexParentFlowID` | Keyword | Written, never queried. |
| `ActiveStepTypes` | KeywordList | Off by default. |
| `DexWorkQueuePermissions` | KeywordList | Membership queries as an OR chain. Hot on the Inbox page. |
| Application slots: `keyword1…`, `int1…`, `datetime1…`, `bool1…`, `text1…` | Various | `=`, OR, ranges. Text uses full-text match. |

Each continue-as-new (by default after 100 operations) adds a new closed row. Attribute upserts are not deduplicated on unchanged values.

### Row-count estimates

These are illustrative assumptions:

- A Live namespace (20 runs/day, 30-day retention): about **2 open and 600 closed** rows.
- The shared platform namespace: about **9 open and 10 closed rows per project**.
- With 10,000 projects, the shared cluster has about **6M rows** in total.
- A heavy app (1,000 runs/day per project) reaches about **300M rows**.

Per-namespace data is small, so the BM25 fixed cost, not the scan cost, dominates.

## Benchmark specification

### Tiers

- **10M rows:** about 10,000 namespaces of about 1,000 rows each. This is the realistic tier.
- **100M rows:** the heavy-app stress tier. Include at least one namespace with several million rows.

### Data

- **Workflow type.** Every user row has `workflow_type_name = 'Engine'`.
- **`flow_type`.** Drawn from about 10 values, with a skewed distribution.
- **Status.**
  - About **0.3%** of rows are open (Running).
  - Some rows are ContinuedAsNew. Report the ratio you used.
- **System rows.** About **0.1%** of rows have a non-null `namespace_division`. Use mostly `TemporalScheduler`.
- **`sa_keyword`.** Holds `DexWorkQueuePermissions` (an array of 1–3 values from a small set) and `keyword1..3`, including a few **rare** values.
- **Other JSONB columns.** `sa_number`, `sa_datetime` and `sa_text` hold a few application slots. `sa_text` contains real words for full-text matching.
- **Unindexed columns.** Fill parent/root IDs, memo and the history stats with realistic sizes.

### Read mix

Concurrent "browsers" each pick a namespace and poll every 10 s. Per FlowType on the page, each poll runs:

| Shape | SQL route |
| --- | --- |
| Main list, page 50, first page | `by_flow_type` |
| "Need a person": `status = 1` and permissions `?|` | `by_flow_type` or `by_flow_type_status`, plus a residual |
| "Done today": `close_time >= today` | `by_flow_type`, plus a residual |
| `status = 1` (Running) | `by_flow_type` or `by_flow_type_status` |
| One custom attribute `=` or range | `by_flow_type`, plus a residual |
| `WorkflowId =` | `by_workflow_id` |

Also include:

- the readiness probe: an empty query plus the Dex filters, page size 1, on `default_idx`;
- a Text-match query on the BM25 route;
- a query with only a custom attribute on the BM25 route;
- a Schedule listing, `namespace_division = 'TemporalScheduler'`, on `by_namespace_division`.

### Write mix

Run a steady mixed write workload in parallel with the reads, using the version guard:

- starts (inserts);
- closes, which move `sort_close_time`;
- attribute upserts;
- continue-as-new, which closes a row and inserts a new run;
- deletes with tombstones.

Run 16 and 64 writers, as in `compare.sh`.

### Required cases

1. **The worst case for residual filtering.**
   - In the multi-million-row namespace, run `FlowType = X` (matching most rows, with no status filter) plus a residual filter on a **rare** attribute value. Report its latency.
   - Compare it with:
     - the same predicate on the BM25 route;
     - a variant where that attribute is a promoted typed column with a B-tree index.
2. **Page-token stability under concurrent writes with residual filters.**
   - Page through a namespace while writes are running.
   - Assert that rows whose sort key did not change are never duplicated or skipped across pages.
   - Report open-to-closed transitions separately. They legitimately move in the sort order, which is existing Temporal semantics.

### Measurements

- **Per-shape latency and throughput:** p50, p95 and p99 latency and achieved QPS per query shape, for both tiers and for both `by_flow_type_status` variants.
- **Writes:** writes per second and write p50/p95/p99.
- **Plan evidence:** run `EXPLAIN (ANALYZE, BUFFERS)` per shape. Confirm the intended path: the right partial index with no `@@@`, a heap filter, or a single ParadeDB scan.
- **Storage:** heap size, B-tree index size, BM25 index size and segment count, and tombstone table growth.
- **Environment:**
  - Record the host spec and background load (see `compare.sh`).
  - Use a host comparable to the 2026-10-16 cloud run, ideally with the load generator on a separate host.
  - Use the image `paradedb/paradedb:${PARADEDB_TAG:-v0.26.1-pg17}`, with the same tuning as `compare.sh`.

## Deliverables

- A PR with the harness changes and a new `results/<date>-dex-profile/README.md`. The README contains:
  - the per-shape tables for both tiers;
  - the EXPLAIN evidence;
  - the page-token result.
- A clear recommendation for the worst case, one of:
  - stay on residual filtering;
  - promote that attribute to a column;
  - route that shape to BM25;
  - a bounded B-tree scan that falls back to BM25.
- A recommendation for C3 (`by_flow_type_status`) based on its read latency gain against its write cost.
