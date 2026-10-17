# ParadeDB Dex-profile visibility store

## Decision and scope

Use a dedicated `store.VisibilityStore` selected through `customDatastore.name: paradedb-dex`. Keep the `paradedb` SQL plugin registered only in `temporal-sql-tool` for schema administration. The former SQL visibility plugin and its column slots are retired. This profile serves Dex foundation and `projects_shared`; a full `cloud_shared` profile requires a separate design. Version 2.0 supports **fresh visibility databases only**. There is no migration from the v1.x column schema.

The physical schema is `schema/paradedb/visibility/schema.sql`. It matches `versioned/v2.0/schema.sql` and the benchmark's `dex-profile-schema.sql` byte for byte. It requires ParadeDB 0.26.1 with `pg_search`. The only indexes are the `(namespace_id, run_id)` primary key, four B-trees (`default_idx`, `by_flow_type`, `by_workflow_id`, `by_namespace_division`), and `visibility_core_bm25`. The default, FlowType, and division indexes are partial. User reads retain `namespace_division IS NULL`; schedules use the non-null division index. The benchmark rejected `by_flow_type_status`: it added 1.5–1.7 GiB without a reliable read gain and induced poor generic plans.

## Architecture and configuration

The factory in `x/paradedb` owns a PGX pool, typed JSONB codec, query conversion, and version check. It receives the cluster search-attribute type provider, mapper provider, CHASM registry, resolver, logger, and metrics through the existing custom-store factory interface. `cmd/server/main.go` supplies `temporal.WithCustomVisibilityStoreFactory(...)` in P5. No shared `sqlplugin.VisibilityRow` or SQL visibility store interface change is needed. Each service process owns a pool; deployments must budget their combined connections. The dedicated visibility database is separate from the default Temporal database.

```yaml
persistence:
  visibilityStore: dex-visibility
  dataStores:
    dex-visibility:
      customDatastore:
        name: paradedb-dex
        indexName: dex_visibility
        options:
          profile: dex
          sql:
            pluginName: postgres12_pgx
            connectAddr: paradedb-visibility:5432
            connectProtocol: tcp
            databaseName: visibility
            user: temporal
            password: ${PARADEDB_VISIBILITY_PASSWORD}
            connectAttributes:
              plan_cache_mode: force_custom_plan
```

The factory validates every option, rejects unknown fields and profiles, and checks schema version **exactly 2.0** and the extension version before serving. It must not log connection secrets. `indexName` is the stable cluster metadata key, not an SQL index identifier. Every visibility connection uses `force_custom_plan`: generic plans were slow for selective JSON residuals in the benchmark. The SQL visibility configuration `pluginName: paradedb` is rejected during server version checking, before any connection attempt, with an error pointing to `customDatastore`. The sql-tool plugin remains usable for `setup-schema` and `update-schema`.

For a fresh database, either run `setup-schema --schema-name paradedb/visibility --version 2.0`, or run `setup-schema --version 0.0` and then `update-schema --schema-name paradedb/visibility`. Both paths must produce the same catalog and version metadata. A v1.x database must be replaced, not updated.

## Data and writes

Core identity, status, times, task queue, parent/root references, counters, memo, encoding, and `_version` are ordinary columns. Registered search attributes use `sa_keyword`, `sa_text`, `sa_number`, `sa_datetime`, and `sa_bool` JSONB maps under their real names. `TemporalNamespaceDivision` maps to `namespace_division`. Scalar `FlowType` maps to `flow_type` and is reconstructed as a search attribute on read. **Scalar-only FlowType is recommended, pending owner confirmation.** Until confirmed, P2/P4 must explicitly reject or correctly handle lists; they must not silently discard elements. Other supported registered types can carry arrays, with `KeywordList` always an array. Nil and empty values omit keys.

Writes validate names against the cluster type map, retain large Int precision with `json.Decoder.UseNumber`, normalize datetimes to UTC microseconds, and reject unknown names, type mismatches, invalid datetime values, oversized strings, and out-of-range values. Registry failure returns Unavailable. A start, close, or search-attribute upsert uses `INSERT ... ON CONFLICT (namespace_id, run_id) DO UPDATE ... WHERE executions_visibility._version < EXCLUDED._version`. Stale or equal TaskIDs affect zero rows and succeed. Delete is a plain `DELETE` by namespace and run ID. There is no tombstone table or advisory lock. A rare delayed write can recreate a deleted row, as in upstream SQL visibility; the owner accepts this. These operations are single SQL statements, so crashes do not leave partial write state. Retry uses the version guard for extant rows.

## Query routing and paging

The shared Temporal parser provides typed expressions, alias resolution, and implicit division semantics. The new converter keeps a private typed expression tree and renders SQL only after classifying the whole predicate. It validates each search-attribute name against the registry and binds all user values. Every List and Count has `namespace_id = $1` outside user Boolean expressions. The BM25 route adds `namespace_id @@@ pdb.term($1::text)` as an index anchor while retaining the ordinary equality. Explicit division predicates, including namespace deletion, replace the implicit `IS NULL`; CHASM uses its archetype division. Unsupported custom ordering and `STARTS_WITH` return InvalidArgument.

| Shape | Route | Predicate behavior |
| --- | --- | --- |
| Empty/default list, status-only, time, workflow type | `default_idx` | Ordinary SQL residuals; user reads require `namespace_division IS NULL`. |
| Scalar `FlowType =/IN` | `by_flow_type` | Keep status, type, and selective custom fields as typed residuals. |
| `WorkflowId =/IN` | `by_workflow_id` | Sort after lookup if needed. |
| Named non-null division | `by_namespace_division` | Preserve schedule and CHASM reads. |
| Positive or negated Text matching | BM25 | Token semantics require the text index. |
| Selective custom-only query | BM25 | Typed JSONB path term, term-set, or range expressions. |
| Broad core prefix plus rare/absent residual | Bounded B-tree scan, then BM25 fallback | Scan at most 1,000 ordered core-prefix rows. If fewer than a page match and the scan reached its bound, use the complete BM25 query. |

An OR gets a core B-tree route only if every satisfiable arm has a usable core prefix; a core predicate outside the OR also qualifies. Negative and missing-key predicates preserve Elasticsearch-style semantics, including arrays: positive comparisons match any element, and negative comparisons match no element or an absent key. Typed numeric, boolean, and datetime JSON paths need real-driver equivalence tests before release. Count uses the same namespace/division/routing rules and supports required internal GROUP BY forms, even when the Dex user Count API is disabled by separate configuration.

Both routes order by `sort_close_time DESC, start_time DESC, run_id COLLATE "C"`. Page tokens carry these three values and a token version. The keyset predicate uses the lexicographic OR chain **and** the redundant `sort_close_time <= $cursor` bound, which makes deep pages an index condition. Reject malformed tokens with InvalidArgument. Pagination under concurrent sort-key movement is best effort; a stable snapshot must have no duplicates or omissions.

## Upstream touch points

| Area | Required work |
| --- | --- |
| `cmd/server/main.go` | P5 installs the custom visibility factory. It does not import the schema-tool plugin. |
| `common/persistence/sql/version_checker.go` | P1 rejects `paradedb` as a server SQL visibility plugin before connecting. |
| `common/persistence/visibility/defs.go` and SQL visibility store | P1 removes the legacy SQL-only/list and query-converter hooks. |
| `service/frontend/operator_handler.go` | P5 uses cluster-level Add/Remove/List registration for `paradedb-dex`, including list values. |
| `tests/testcore`, persistence fixtures, advanced visibility suites | P6 provisions a dedicated fresh database and custom factory, then restores functional CI coverage. |
| `schema/embed.go`, `cmd/tools/sql/main.go` | Existing embedding and plugin registration support v2 tooling; no runtime plugin import is required. |

The frontend Dex profile retains Scan and v1 Schedule reads. The separate Dex configuration disables unused user Count, Worker Versioning/deployments, batch, CHASM Scheduler creation, and old filtered list paths while the store still implements required internal methods. Set `system.visibilityAllowList=true`, cap frontend pages at 100, and choose the visibility RPS limit after the larger gate. Do not dual-write this profile without an explicit migration design.

## Verification and release gates

P1 checks schema-byte parity, both sql-tool setup paths and catalog parity on ParadeDB 0.26.1, exact version rejection of 1.14, and the startup configuration guard. P2 tests typed codec, list/nil handling, provider errors, PGX connection settings, read round trips, and the shared persistence suite. P3 tests versioned start/close/upsert, stale/equal TaskIDs, delete/rewrite races under the accepted semantics, force delete, and retry. P4 tests nested AND/OR/NOT, division, CHASM, injection-safe binds, every supported type/operator, route/EXPLAIN, keyset depth, Count/GROUP BY, and the 1,000-row fallback. P5 tests cluster registration and server boot. P6 runs both advanced visibility variants, schedules, and functional CI through the custom store. P7 repeats renderer-produced SQL at 10M and 100M rows, with a heavy namespace, 16/64 writers, rare residuals, BM25 merges, and SIGKILL/WAL replay.

The 10M-row benchmark found the extra status index unhelpful and exposed large residual scans under a broad FlowType. The bounded scan caps that work before fallback; on small namespaces it normally completes in the B-tree path. On heavy namespaces BM25 latency may rise after write bursts while segments merge. Increasing load by 10x therefore requires measuring route latency, index/heap growth, connection pressure, and recovery rather than extrapolating the small run. Query binding and registry validation prevent SQL injection; options validation and secret-safe logging protect credentials. The owner still needs to confirm the scalar-only FlowType contract and final scale thresholds.

## Delivery sequence

| PR | Dependency | Result |
| --- | --- | --- |
| P1 | None | Fresh v2 schema, retire old plugin path, tooling guard, CI schema parity, design record. |
| P2 | P1 | Custom factory, pool, typed codec, read path. |
| P3 | P2 | Versioned writes and plain delete. |
| P4 | P2 | Typed converter, B-tree/BM25 routing, bounded fallback, keyset/Count. |
| P5 | P2 | Server factory and cluster search-attribute registration. |
| P6 | P3–P5 | Dedicated test mode and functional/advanced CI. |
| P7 | P4–P6 | Large-scale performance and recovery gate. |
