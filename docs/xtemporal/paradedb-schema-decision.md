# ParadeDB visibility schema decision

**Decision: adopt the `pdb-json-hybrid` design** for the ParadeDB visibility store. Search attributes are stored in typed JSONB columns covered by one BM25 index, and core fields get dedicated B-tree indexes. Core-field queries are routed to B-tree, and search-attribute queries to BM25. This is based on the 2026-10-16 comparison in `develop/docker-compose/paradedb/bench/results/2026-10-16-cloud-4cpu/`. The prerequisites listed at the end must pass before it ships.

Today the design exists only as the benchmark schema `develop/docker-compose/paradedb/bench/json-hybrid-schema.sql`. `schema/paradedb/visibility` and the `x/paradedb` plugin still use the column-based layout.

## Background

### Two kinds of index

- **B-tree** (built into PostgreSQL) works like the ordered index of a book. Equality and range lookups on one field, such as `ExecutionStatus = 'Running'` or `WorkflowId = 'order-123'`, take well under a millisecond. Each queryable field needs its own index, and every write must update all of them.
- **BM25** (provided by ParadeDB's `pg_search`, the same model as Elasticsearch) works like a search engine. One index covers many fields and handles text matching and combinations of conditions. Every query pays a fixed cost, about 60 ms in our runs, so simple lookups are much slower than on a B-tree. A table can have only one BM25 index, with at most 32 directly listed fields.

### Two ways to store search attributes

Take a workflow with three custom search attributes: `CustomerId = "c-42"` (Keyword), `Region = "us-west"` (Keyword) and `Priority = 5` (Int).

**Column-based**, as in the v12 PostgreSQL schema and the current ParadeDB schema. The table pre-allocates a fixed set of typed slots: `Keyword01..10`, `Text01..03`, `KeywordList01..03`, and `Int`, `Double`, `Bool`, `Datetime` `01..03`. Each custom attribute is mapped to a slot per namespace, for example `CustomerId → Keyword01`, `Region → Keyword02`, `Priority → Int01`. The row stores:

```
search_attributes = {"Keyword01": "c-42", "Keyword02": "us-west", "Int01": 5}
Keyword01 = 'c-42'       -- generated column extracted from search_attributes
Keyword02 = 'us-west'
Int01     = 5
Keyword03 … Keyword10, Text01 …   -- about 60 generated columns in total, mostly NULL
```

The slot count is a hard limit: an eleventh Keyword attribute has nowhere to go.

**JSON-based**, as in `pdb-json-hybrid`. There are no slots. Attributes are stored under their real names in one JSONB column per type:

```
sa_keyword = {"CustomerId": "c-42", "Region": "us-west"}
sa_number  = {"Priority": 5}
sa_text = {}   sa_datetime = {}   sa_bool = {}
```

The BM25 index covers every key in these columns. Adding an attribute means adding a key, with no schema change and no per-type slot limit.

## The four variants compared

| Variant | Database | Search-attribute storage | Indexes | Read routing |
|---|---|---|---:|---|
| `postgres` | stock PostgreSQL | column-based (v12 schema) | full v12 B-tree/GIN set | B-tree/GIN for everything |
| `pdb-cols` | ParadeDB | column-based | 50 | every query forced through BM25 with `namespace_id @@@ $1` |
| `pdb-cols-hybrid` | ParadeDB | column-based (same table as `pdb-cols`) | 55 | core fields → B-tree, search attributes → BM25 |
| `pdb-json-hybrid` | ParadeDB | JSON-based | 8 | same as `pdb-cols-hybrid` |

Index inventory of the column-based ParadeDB schema (`develop/docker-compose/paradedb/spike/paradedb-schema.sql`):

| Group | Count | Examples |
|---|---:|---|
| Primary key | 1 | `(namespace_id, run_id)` |
| BM25 | 1 | `visibility_core_bm25`: core fields plus the `Keyword`, `Text` and `KeywordList` slots |
| Default list order | 1 | `default_idx` |
| Parent and root execution | 4 | `by_parent_workflow_id`, `by_root_run_id` |
| Temporal system attributes | 16 | `by_build_ids`, `by_temporal_worker_deployment_version`, `by_batcher_user` (6 are GIN) |
| Custom `Int`/`Double`/`Bool`/`Datetime` slots, not covered by BM25 | 12 | `by_int_01..03`, `by_datetime_01..03` |
| Temporal-reserved slots | 15 | `by_temporal_keyword_01..04`, `by_temporal_keyword_list_01..02` |

`pdb-cols-hybrid` adds the five core B-tree indexes in `develop/docker-compose/paradedb/bench/core-indexes.sql` (status, workflow type, workflow ID, task queue, execution time), for 55 in total. `pdb-json-hybrid` keeps only the primary key, the BM25 index, `default_idx` and the same five core B-tree indexes: 8 in total. Its system attributes, custom attributes and reserved slots all live in the JSONB columns under the one BM25 index.

Using the example queries:

- **Core-field query.** `WorkflowType = 'OrderFlow' AND ExecutionStatus = 'Running'` in one namespace. `pdb-cols` runs it through BM25 at about 60 ms; both hybrid variants use a B-tree at about 1 ms or less.
- **Search-attribute query.** `CustomerId = 'c-42'`. All three ParadeDB variants use BM25 at about 60–70 ms. `pdb-cols-hybrid` queries the `Keyword01` slot, and `pdb-json-hybrid` queries `sa_keyword->'CustomerId'`.

## Measured results

From the fixed run: medians of two rounds, 1M rows, 4 vCPU / 15 GB host.

| Metric | postgres | pdb-cols | pdb-cols-hybrid | pdb-json-hybrid |
|---|---:|---:|---:|---:|
| Writes/s, 16 writers | ~620 | ~1860 | ~1720 | **~3410** |
| Writes/s, 64 writers | ~1080 | ~2400 | ~2055 | **~4960** |
| Write p99, 64 writers | 316 ms | 133 ms | 164 ms | **47 ms** |
| Index size after 64 writers | 17.9 GB | 9.2 GB | 10.0 GB | **2.3 GB** |
| Core-field read capacity (`status_time`) | 16554 QPS | 119 QPS | **19634 QPS** | 5288 QPS |
| Search-attribute read capacity | 2.7k–7k QPS | ~130 QPS | ~125 QPS | ~110 QPS |
| Search-attribute p50 / p99, 64 writers | **4–9 / 62–101 ms** | ~400 ms / 3.3–4.2 s | ~400 ms / 3.6–5.1 s | ~1.1 s / ~11 s |

The full tables and caveats are in the results `README.md`.

## Rationale

1. **The workload is write-heavy.** Temporal writes visibility on every workflow start, close and search-attribute upsert. `pdb-json-hybrid` sustains about 5x the write throughput of `postgres` and about 2x that of `pdb-cols-hybrid`. Its write p99 is an order of magnitude lower, and its indexes are a quarter of the size of `pdb-cols-hybrid`'s. Most of the gap is index maintenance: 8 indexes per write instead of 55, many of which cover columns that are NULL in most rows.
2. **Reads are mostly core-field lookups, and those stay fast.** Typical Web UI and CLI queries filter on namespace plus `WorkflowType`, `WorkflowId`, `ExecutionStatus` or start/close time. In both hybrid variants these use B-tree indexes and stay within a few milliseconds under write load. `pdb-cols-hybrid` has a higher core-read ceiling (about 19.6k vs 5.3k QPS), but both are far above the default visibility rate limit of `frontend.namespaceRPS.visibility` = 10 per namespace per frontend instance.
3. **Server-internal visibility reads are rare.** There are no visibility reads on the workflow hot path. History only resolves the search-attribute type map, which is in-memory metadata. The only automatic readers are the Worker Versioning v3 drainage check (a count every 3 minutes per draining version) and the deployment limit check. Both run only when workers use deployment versioning, and they can be disabled with `system.enableDeploymentVersions=false`. Batch operations, namespace deletion, migration and reachability queries run only when a user or operator triggers them.
4. **No search-attribute slot limit.** The column layout caps custom attributes per type, for example 10 Keyword and 3 Int. The JSON layout has no slots, so this cap disappears once the server stops mapping attribute names to slots.
5. **Accepted trade-offs.** Search-attribute queries pay BM25's fixed cost of about 60 ms, which caps a single query type at about 110–140 QPS. They are 20–60x slower than `postgres` for common values, and their tail latency degrades under heavy writes (about 11 s p99 at 64 writers). That last comparison is not apples to apples: `pdb-json-hybrid` was absorbing 2.4x the writes of `pdb-cols-hybrid` in the same window. Search-attribute queries are a small share of reads for this workload, and that share is bounded by the visibility rate limit.

## Prerequisites and follow-ups

Before production:

- **Crash recovery:** run the `BENCH_RUN_CRASH=1` scenario on `pdb-json-hybrid` and verify that the BM25 index and counts are consistent after `SIGKILL` and WAL replay.
- **Long-running stability:** run for several hours to observe BM25 segment merges, bloat and read tail latency over time, and tune merge settings if p99 keeps growing.
- **Realistic read mix:** add the Web UI queries to the benchmark: default namespace list, `WorkflowId =`, `WorkflowId STARTS_WITH`, `WorkflowType` plus time range, and paired list plus count. Also add a low-selectivity search-attribute case.
- **Fair search-attribute comparison:** compare `pdb-cols-hybrid` and `pdb-json-hybrid` at the same fixed write rate.
- **Production-size data:** repeat at a larger row count on hardware closer to production, with the load generator on a separate host.

Server work:

- Implement the JSON layout in `schema/paradedb/visibility` and `x/paradedb`: the query converter, JSON-path BM25 predicates, and core-field routing to B-tree.
- Drop the name-to-slot mapping and revisit the search-attribute count limits for this store.
- Replace internal "is the count zero" `CountWorkflowExecutions` calls with existence checks (`List` with page size 1). These are the deployment drainage check, namespace-deletion verification and build-ID checks. Consider a per-namespace switch to reject user `Count` requests on this store.
- Set `system.enableDeploymentVersions=false` where Worker Versioning is not used. First confirm that no deployments exist with `temporal worker deployment list`.
