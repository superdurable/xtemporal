# ParadeDB visibility feasibility spike (Temporal v1.32.1)

**Verdict: go with changes.** A ParadeDB visibility store can reuse PostgreSQL persistence and use one ParadeDB index for common list predicates. It must replace the upstream coalesce expression with a stored sort column, use literal tokenizers for exact search attributes, and force an indexed search predicate. This is a local, single-host feasibility result, not a production capacity claim.

## Reproduction and scope

Run `docker compose -f develop/docker-compose/paradedb/docker-compose.yml -p xtpdbspike up -d --wait`; execute `spike/paradedb-schema.sql` in ParadeDB and `schema/postgresql/v12/visibility/schema.sql` in stock PostgreSQL, then `spike/load.sql` in each. The two databases each held 200,000 rows across 20 namespaces. `spike/plans-paradedb.sql` records the naive schema plans; `spike/plans-paradedb-optimized.sql` and `spike/plans-postgres.sql` record the revised and stock plans; `spike/benchmark_latency.py` and `spike/upsert.sql` measure list latency and writes. The `.out` files retain full `EXPLAIN (ANALYZE, BUFFERS)` output and timings.

The version-pinned image `paradedb/paradedb:v0.26.0-pg17` ran PostgreSQL 17.11 and `SELECT extversion FROM pg_extension WHERE extname='pg_search'` returned `0.26.0`. Its local image digest was `sha256:2be0ce766f7ac38e8ea3c914fed8a680812433641d431fdcf50045a9cda02400` (multi-platform pull; the tag is kept in Compose). The image tag and extension version are independently verified by the running container. [v0.26.0 release](https://github.com/paradedb/paradedb/releases/tag/v0.26.0); [v0.26.0 LICENSE](https://github.com/paradedb/paradedb/blob/v0.26.0/LICENSE) is GNU AGPL-3.0. This license finding needs distribution review if a product bundles the image; it does not by itself determine obligations for a separate network client.

## Verdicts on the eight open questions

1. **Version and license: confirmed.** Pin pg_search 0.26.0 / PG17; the running image reports PG 17.11 and extension 0.26.0. The repository's v0.26.0 `LICENSE` is AGPL-3.0.
2. **`key_field`: no extra column.** `CREATE INDEX ... WITH (key_field='doc_key')` emitted `WARNING: key_field is deprecated as of 0.26.0 and is a no-op`; the probe had two rows with identical `run_id='1'` in different namespaces and returned the correct one for `namespace_id @@@ 'a'`. The final index omits `key_field`; retain the existing composite primary key. [Current create-index reference](https://www.paradedb.com/docs/reference/indexing/create-index) states no unique first column is required.
3. **Index coverage: one 30-field ParadeDB index for common list paths; keep stock indexes for less common fields.** PostgreSQL rejected 41 direct BM25 fields with `cannot use more than 32 columns in an index` and rejected a second ParadeDB index with `a relation may only have one ParadeDB index`. The proposed index includes `namespace_id`, `run_id`, workflow ID/type, status, start/close/execution time, a generated close sort, task queue, history length/size, execution duration, state transition count, `Text01..03`, `Keyword01..10`, and `KeywordList01..03`. It excludes binary `memo`, `encoding`, `_version`, and `search_attributes` JSONB root. `Bool01..03`, `Datetime01..03`, `Double01..03`, `Int01..03`, all predefined Temporal generated columns, parent/root identifiers, and CHASM generated columns retain upstream B-tree/GIN indexes in the DDL. Those predicates remain correct, but an anchored ParadeDB scan may apply them as heap filters; profile them before expanding the index. Composite types can bypass PostgreSQL's 32-field syntax limit ([reference](https://www.paradedb.com/docs/reference/indexing/indexing-composite)); expanding the single index would increase write cost and is deliberately outside this measured proposal.
4. **Pushdown: requires a ParadeDB search operator; normal scalar predicates then push down if their field type/tokenizer is suitable.** `namespace_id = md5('0') AND status=1 ...` used a PostgreSQL index plan, while `namespace_id @@@ md5('0') AND status=1 AND start_time >= ... AND start_time < ...` used `Custom Scan (ParadeDB Base Scan)` and its Tantivy query contained a namespace term, status term, and two start-time ranges. The revised full sort used `TopKScanExecState` with 20 heap fetches. Workflow type/status and Keyword `IN` became Tantivy `term`/`term_set`; Text `|||` became a `match`; prefix regex became a `regex`; KeywordList term became a `term`. Count and grouped count used `Custom Scan (ParadeDB Aggregate Scan)` and the grouped plan had `grouped.terms.field=status`. The unmodified `LIKE` and JSONB `@>` forms appeared as `heap_filter` in the naive plans, despite the outer custom scan. [Filter reference](https://www.paradedb.com/docs/reference/filtering/indexed) confirms an index operator is needed; `pdb.all()` is another possible anchor, but using namespace term narrows early.
5. **Sort: a stored generated column fixes the expression barrier, with C collation for the tie breaker.** Upstream v1.32.1 uses `ORDER BY coalesce(close_time, '9999-12-31 23:59:59') DESC, start_time DESC, run_id`. On 200k rows, that expression caused a separate PostgreSQL `Sort` above `NormalScanExecState`; one anchored plan fetched 1,428 heap rows and took 61.746 ms. The final `sort_close_time` column contains the identical expression. `ORDER BY sort_close_time DESC, start_time DESC, run_id COLLATE "C" LIMIT 20` used `TopKScanExecState` and fetched only 20 heap rows (13.776 ms in the captured post-update `EXPLAIN`, 4.362 ms post-update warm p50). Without `COLLATE "C"`, the `en_US.utf8` database matched only the first two sort keys and fell back to a sort. Temporal run IDs are fixed-format UUID strings, but the converter's keyset comparison must use the same collation as the order. [Top K reference](https://www.paradedb.com/docs/reference/full-text/top-k) says arbitrary expressions do not push down.
6. **Write amplification: local batch gate passed; production gate pending.** Four 10k-row versioned `INSERT ... ON CONFLICT DO UPDATE ... WHERE old._version < EXCLUDED._version` batches achieved 4,354 warm updates/s on ParadeDB versus 1,573/s on stock PG. This measures index maintenance, not single-row network/transaction latency; the 1M-row, concurrent PGX gate remains required.
7. **Text semantics: preserve the upstream OR-of-case-sensitive-lexemes behavior as closely as possible.** PostgreSQL's unified converter splits only on spaces and maps `Text01 = 'foo bar'` to `Text01 @@ 'foo | bar'::tsquery`; the generated `TSVECTOR` casts raw JSON text. `SELECT 'Hello World'::tsvector @@ 'hello'::tsquery` returned false and `@@ 'Hello'::tsquery` true. ParadeDB's default Unicode tokenizer lowercases, so it would change behavior. The proposed raw `TEXT` generated column is indexed as `pdb.whitespace('lowercase=false')`, with `Text01 ||| 'foo bar'` for OR and `NOT (...)` for inequality. A 1,000-row probe distinguished `Hello` from `hello`. PostgreSQL's tsvector lexeme syntax and ParadeDB whitespace tokenization can still differ for quotes, punctuation, and non-space whitespace; parity tests must cover those cases. [Tokenizer reference](https://www.paradedb.com/docs/reference/token-filters/lowercase).
8. **Plugin wrapping: feasible on the reset v1.32.1 base.** The PostgreSQL plugin concrete type is unexported, so a new `paradedb` plugin cannot instantiate it directly. Its `CreateDB` can copy `config.SQL`, set `PluginName=postgres12_pgx`, call the registered `sql.NewSQLDB`, assert that the returned PostgreSQL DB also implements `sqlplugin.AdminDB`, and return one wrapper implementing **both** `sqlplugin.DB` and `sqlplugin.AdminDB`, with `PluginName()` overridden to `paradedb`. This dual-interface wrapper is necessary because `sql.NewSQLDB` and `sql.NewSQLAdminDB` call the same `Plugin.CreateDB(dbKind, ...)` signature; `dbKind` does not identify whether the caller expects a regular or admin DB. `sql.NewSQLDB` casts the returned `GenericDB` to `sqlplugin.DB`; the wrapper satisfies that interface. The visibility store gets its name through `SQLStore.GetName()` → `DB.PluginName()` and the unified converter through `sql.GetPluginVisibilityQueryConverter(name)`. PG visibility methods take converter-produced SQL and already rebind `?` placeholders, so no visibility CRUD override is needed. The admin wrapper delegates `ExpectedVersion()` and version checks to PostgreSQL; the ParadeDB schema should advertise the same v1.14 visibility version. The upstream ref `refs/upstream/v1.32.1` sets `system.visibilityEnableUnifiedQueryConverter` to **true**; this working branch's older fork sets it to false. The reset implementation must verify the target default remains true. Legacy PostgreSQL conversion cannot be blindly reused with `TEXT` instead of `TSVECTOR`; either require the unified converter or add a dedicated legacy implementation.

## Query translation

| Temporal construct | Proposed SQL / converter behavior | Verified path |
| --- | --- | --- |
| Namespace scope | `namespace_id @@@ <quoted namespace UUID>` | Search anchor, indexed term |
| `WorkflowType = 'x' AND ExecutionStatus = 'Running'` | `workflow_type_name = 'x' AND status = 1` plus namespace anchor | Two indexed terms |
| `Text01 = 'foo bar'` | `Text01 ||| 'foo bar'` with case-preserving whitespace tokenizer | Indexed OR match; check edge lexemes |
| `Text01 != 'foo'` | `NOT (Text01 ||| 'foo')` | Needs null-semantics parity test |
| `Keyword01 IN ('a','b')` | Existing SQL `IN`, with literal tokenizer | Indexed `term_set` |
| `Keyword02 STARTS_WITH 'pre'` | `Keyword02 @@@ pdb.regex('pre.*')`, after regex escaping user text | Indexed regex; raw `LIKE` stayed heap filter |
| `KeywordList01 = 'tag'` | `KeywordList01 @@@ pdb.term('tag')` with `pdb.literal` JSON tokenizer | Indexed exact array member; compare JSON `@>` |
| `KeywordList01 IN (...)` | OR of safe `pdb.term(...)` predicates | Needs converter tests |
| Numeric / boolean / datetime custom SA | Existing PostgreSQL expressions and stock B-tree indexes | Correctness retained; anchored custom scan may heap filter |
| List order / page token | `sort_close_time DESC, start_time DESC, run_id COLLATE "C"`; use same fields/collation in keyset predicate | Top K and deterministic pagination |
| Count / GroupBy status | Existing `COUNT(*)` / `GROUP BY status` plus namespace anchor | ParadeDB Aggregate Scan |

`pdb.literal` is essential: on the naive default tokenizer, `Keyword01 IN` stayed a `heap_filter`, while the revised index produced `term_set`. `Keyword02 LIKE 'prefix-12%'` and `Keyword02 @@@ pdb.regex('prefix-12.*')` each matched 2,200 rows; `KeywordList01 @> '["tag-0"]'` and `KeywordList01 @@@ pdb.term('tag-0')` each matched 10,000. With the default JSON tokenizer, `pdb.term('tag-0')` matched zero and `pdb.term('tag')` matched all 200,000; the literal JSON tokenizer corrected that. Regex escaping and null behavior need explicit converter tests.

## Local plans and benchmark

Captured plan extracts (full text in `spike/*.txt`):

```text
Naive: Limit -> Sort -> Custom Scan (ParadeDB Base Scan)
       Exec Method: NormalScanExecState; Heap Fetches: 1428; Execution Time: 61.746 ms
Revised status/time: Limit -> Custom Scan (ParadeDB Base Scan)
       Exec Method: TopKScanExecState; TopK Limit: 20; Heap Fetches: 20; Execution Time: 13.776 ms
Revised keyword IN: Custom Scan; term_set(keyword01={tenant-0,tenant-20})
Revised prefix: Custom Scan; regex(keyword02='prefix-12.*')
Revised keyword list: Custom Scan; term(keywordlist01='tag-0')
Revised count: Custom Scan (ParadeDB Aggregate Scan); COUNT(*)
Revised group count: Custom Scan (ParadeDB Aggregate Scan); grouped terms(status)
```

Warm server-side `EXPLAIN (ANALYZE)` execution milliseconds, 20 repeats per query, one query type at a time, after the 40k upsert workload:

| List query, 20-row limit | ParadeDB p50 / p95 | Stock PG p50 / p95 |
| --- | ---: | ---: |
| Namespace + status + time range | 4.362 / 6.208 | 20.854 / 27.780 |
| Namespace + workflow type + status | 4.375 / 5.429 | 22.381 / 26.298 |
| Namespace + Text01 match | 4.965 / 5.895 | 20.463 / 23.454 |
| Namespace + Keyword01 IN | 4.407 / 5.762 | 23.255 / 79.071 |
| Namespace + KeywordList01 contains | 4.284 / 5.133 | 54.821 / 136.570 |

The five predicates all returned matches. The generator uses 20 namespaces and repeatable modulo distributions, short memo, and six non-null custom search attributes. These numbers are single-client, warm-cache, local Docker figures; they do not model multi-client contention, larger memos, or merge pressure over hours. The initial 200k insert took 115.009 s in ParadeDB and 489.378 s in stock PG, but those loads overlapped and are **not** a fair throughput comparison. Stock PG had every v12 index; the candidate retains all non-core indexes and substitutes one ParadeDB index for the core indexes.

The four batch timings (seconds) were ParadeDB **5.282, 1.918, 2.204, 2.768** and stock PostgreSQL **12.457, 6.387, 6.944, 5.740**. The first batch includes cold index/cache effects. Across all 40k updates, throughput was **3,286/s versus 1,269/s** (2.59×); across the last 30k, **4,354/s versus 1,573/s** (2.77×). Both databases used default durability settings. The SQL is the same 17-column version-checked conflict update in both containers, and each 10k batch is one statement/transaction. Upstream's PGX driver writes one execution at a time with 22 fields, so this is a preliminary write-amplification result, not a final production throughput verdict.

Suggested acceptance gate for the next implementation: on the same hardware, storage, row distribution, 4–16 concurrent PGX connections, and at least 1 million rows after a sustained 30-minute update period, ParadeDB must achieve at least **80% of stock PostgreSQL's versioned single-row upsert throughput**, with p95 write latency no more than **2×**, and p95 of the five representative list queries no more than **1.25×** stock. Require exact result and pagination parity for each query before any speed claim. A 10× load test should also check index size, segment count, merge backlog, WAL volume, crash recovery, and p95 latency. A crash must leave transactional PG rows and the search index consistent; test kill/restart during upserts. The 80% gate is a proposed product threshold, not a ParadeDB guarantee.

## Plugin and schema implementation notes

- Add `x/paradedb/plugin.go` to register a `sqlplugin.Plugin` under `paradedb`. Its `CreateDB` delegates once through `sql.NewSQLDB` with a copied `config.SQL{PluginName: postgres12_pgx}`, then asserts the PostgreSQL value also implements `sqlplugin.AdminDB`. Do not call `paradedb` recursively.
- Add `x/paradedb/db.go` with a single wrapper embedding `sqlplugin.DB` and `sqlplugin.AdminDB`; explicitly forward the three overlapping `GenericDB` methods (`DbName`, `Close`, `PluginName`) so promotion is unambiguous, and return `paradedb` from `PluginName()`. Keep visibility read/write/delete methods delegated to the PostgreSQL PGX implementation. The main store config stays `postgres12_pgx`.
- Add `x/paradedb/query_converter.go` implementing `sqlplugin.VisibilityQueryConverter`: own list/count SQL, namespace search anchor, Text OR matching, KeywordList exact term, regex-escaped prefix, generated sort and C-collated keyset comparison. Reuse query AST helpers where possible. Reject unsupported expressions with converter errors; do not interpolate unescaped user input into regex or SQL.
- Add `schema/paradedb/visibility/` with the table and index DDL below and v1.14 schema metadata. `schema/embed.go` uses `//go:embed *`, so files under the new path are embedded without editing it. The SQL tool can use an explicit schema-dir; `schema.PathsByDB("sql")` lists only MySQL/PostgreSQL paths, so adding ParadeDB there is an **optional CLI discoverability hook**, not required for explicit setup.
- Required upstream hook points on the reset base: blank imports in `cmd/server/main.go` and `cmd/tools/sql/main.go`; add `paradedb` to `common/persistence/visibility/defs.go` SQL validation list; test harness cases in `tests/testcore/flag.go`, `common/persistence/persistence-tests/setup.go`, and `common/persistence/persistence-tests/persistence_test_base.go`. The last file is an **additional test harness hook** beyond the plan's list: its PostgreSQL switch handles suite setup/schema paths. The legacy converter factory is optional only while the unified converter stays enabled; reusing its PostgreSQL case with the proposed `TEXT` columns would fail. To support the feature flag being turned off, add a dedicated legacy converter in `common/persistence/visibility/store/sql/` or force the unified path for `paradedb` in `visibility_store.go` (an additional upstream hook).

## Risks and remaining proof

- The selected 30-field index does not accelerate every Temporal generated field; add fields through a composite type only after workload evidence and a repeated write gate.
- Regex prefix search needs input escaping and selective-prefix limits. Literal JSON term matching needs parity checks for arrays with punctuation, quotes, nested values, and nulls.
- `TEXT` replaces upstream `TSVECTOR` generated columns, so rolling migration needs a fresh visibility database or a carefully staged backfill and reindex. A baseline PostgreSQL legacy converter cannot query these columns.
- C collation in the tie breaker must be used identically in ORDER BY and page-token predicates. Validate UUID ordering and token continuity across open/closed rows, equal timestamps, and namespace boundaries.
- The local batch-upsert benchmark below measures index maintenance without per-row client round trips; run the proposed PGX concurrency gate before release.

## Recommended visibility schema DDL

This is the full proposed CREATE TABLE and CREATE INDEX script, mirrored in `develop/docker-compose/paradedb/spike/paradedb-schema.sql` for replay. It starts from the upstream v12 visibility table and preserves all generated search-attribute columns and the remaining PostgreSQL indexes. `VARCHAR(64)` replaces `CHAR(64)` for namespace/run IDs because the ParadeDB index rejected `BPCHAROID` in the probe.

```sql
CREATE EXTENSION IF NOT EXISTS pg_search;

DO LANGUAGE 'plpgsql' $$
    BEGIN
        IF ( NOT EXISTS (select extname from pg_extension where extname = 'btree_gin') ) THEN
            CREATE EXTENSION btree_gin;
        END IF;
    END
$$;

-- convert_ts converts a timestamp in RFC3339 to UTC timestamp without time zone.
CREATE FUNCTION convert_ts(s VARCHAR) RETURNS TIMESTAMP AS $$
BEGIN
  RETURN s::timestamptz at time zone 'UTC';
END
$$ LANGUAGE plpgsql IMMUTABLE RETURNS NULL ON NULL INPUT;

CREATE TABLE executions_visibility (
  namespace_id            VARCHAR(64)   NOT NULL,
  run_id                  VARCHAR(64)   NOT NULL,
  _version                BIGINT        NOT NULL DEFAULT 0, -- increasing version, used to reject upserts which are out of order
  start_time              TIMESTAMP     NOT NULL,
  execution_time          TIMESTAMP     NOT NULL,
  workflow_id             VARCHAR(255)  NOT NULL,
  workflow_type_name      VARCHAR(255)  NOT NULL,
  status                  INTEGER       NOT NULL,  -- enum WorkflowExecutionStatus {RUNNING, COMPLETED, FAILED, CANCELED, TERMINATED, CONTINUED_AS_NEW, TIMED_OUT}
  close_time              TIMESTAMP     NULL,
  sort_close_time         TIMESTAMP     GENERATED ALWAYS AS (coalesce(close_time, '9999-12-31 23:59:59')) STORED,
  history_length          BIGINT        NULL,
  history_size_bytes      BIGINT        NULL,
  execution_duration      BIGINT        NULL,
  state_transition_count  BIGINT        NULL,
  memo                    BYTEA         NULL,
  encoding                VARCHAR(64)   NOT NULL,
  task_queue              VARCHAR(255)  NOT NULL DEFAULT '',
  search_attributes       JSONB         NULL,
  parent_workflow_id      VARCHAR(255)  NULL,
  parent_run_id           VARCHAR(255)  NULL,
  root_workflow_id        VARCHAR(255)  NOT NULL DEFAULT '',
  root_run_id             VARCHAR(255)  NOT NULL DEFAULT '',

  -- Each search attribute has its own generated column.
  -- Since PostgreSQL doesn't support virtual columns, all columns are stored.
  -- PostgreSQL doesn't auto cast to the corresponding column type, so we need to explicitly do it.
  -- Check the `custom_search_attributes` table for complete set of examples.

  -- Pre-defined search attributes
  TemporalChangeVersion         JSONB         GENERATED ALWAYS AS (search_attributes->'TemporalChangeVersion')                    STORED,
  BinaryChecksums               JSONB         GENERATED ALWAYS AS (search_attributes->'BinaryChecksums')                          STORED,
  BatcherUser                   VARCHAR(255)  GENERATED ALWAYS AS (search_attributes->>'BatcherUser')                             STORED,
  TemporalScheduledStartTime    TIMESTAMP     GENERATED ALWAYS AS (convert_ts(search_attributes->>'TemporalScheduledStartTime'))  STORED,
  TemporalScheduledById         VARCHAR(255)  GENERATED ALWAYS AS (search_attributes->>'TemporalScheduledById')                   STORED,
  TemporalSchedulePaused        BOOLEAN       GENERATED ALWAYS AS ((search_attributes->'TemporalSchedulePaused')::boolean)        STORED,
  TemporalNamespaceDivision     VARCHAR(255)  GENERATED ALWAYS AS (search_attributes->>'TemporalNamespaceDivision')               STORED,
  BuildIds                      JSONB         GENERATED ALWAYS AS (search_attributes->'BuildIds')                                 STORED,
  TemporalPauseInfo             JSONB         GENERATED ALWAYS AS (search_attributes->'TemporalPauseInfo')                        STORED,
  TemporalReportedProblems      JSONB         GENERATED ALWAYS AS (search_attributes->'TemporalReportedProblems')                    STORED,
  TemporalWorkerDeploymentVersion    VARCHAR(255)    GENERATED ALWAYS AS (search_attributes->>'TemporalWorkerDeploymentVersion')          STORED,
  TemporalWorkflowVersioningBehavior VARCHAR(255)    GENERATED ALWAYS AS (search_attributes->>'TemporalWorkflowVersioningBehavior')       STORED,
  TemporalWorkerDeployment           VARCHAR(255)    GENERATED ALWAYS AS (search_attributes->>'TemporalWorkerDeployment')                 STORED,
  TemporalUsedWorkerDeploymentVersions JSONB GENERATED ALWAYS AS (search_attributes->'TemporalUsedWorkerDeploymentVersions') STORED,
  TemporalExternalPayloadSizeBytes BIGINT GENERATED ALWAYS AS ((search_attributes->'TemporalExternalPayloadSizeBytes')::bigint) STORED,
  TemporalExternalPayloadCount BIGINT GENERATED ALWAYS AS ((search_attributes->'TemporalExternalPayloadCount')::bigint) STORED,

  -- Pre-allocated custom search attributes
  Bool01          BOOLEAN         GENERATED ALWAYS AS ((search_attributes->'Bool01')::boolean)        STORED,
  Bool02          BOOLEAN         GENERATED ALWAYS AS ((search_attributes->'Bool02')::boolean)        STORED,
  Bool03          BOOLEAN         GENERATED ALWAYS AS ((search_attributes->'Bool03')::boolean)        STORED,
  Datetime01      TIMESTAMP       GENERATED ALWAYS AS (convert_ts(search_attributes->>'Datetime01'))  STORED,
  Datetime02      TIMESTAMP       GENERATED ALWAYS AS (convert_ts(search_attributes->>'Datetime02'))  STORED,
  Datetime03      TIMESTAMP       GENERATED ALWAYS AS (convert_ts(search_attributes->>'Datetime03'))  STORED,
  Double01        DECIMAL(20, 5)  GENERATED ALWAYS AS ((search_attributes->'Double01')::decimal)      STORED,
  Double02        DECIMAL(20, 5)  GENERATED ALWAYS AS ((search_attributes->'Double02')::decimal)      STORED,
  Double03        DECIMAL(20, 5)  GENERATED ALWAYS AS ((search_attributes->'Double03')::decimal)      STORED,
  Int01           BIGINT          GENERATED ALWAYS AS ((search_attributes->'Int01')::bigint)          STORED,
  Int02           BIGINT          GENERATED ALWAYS AS ((search_attributes->'Int02')::bigint)          STORED,
  Int03           BIGINT          GENERATED ALWAYS AS ((search_attributes->'Int03')::bigint)          STORED,
  Keyword01       VARCHAR(255)    GENERATED ALWAYS AS (search_attributes->>'Keyword01')               STORED,
  Keyword02       VARCHAR(255)    GENERATED ALWAYS AS (search_attributes->>'Keyword02')               STORED,
  Keyword03       VARCHAR(255)    GENERATED ALWAYS AS (search_attributes->>'Keyword03')               STORED,
  Keyword04       VARCHAR(255)    GENERATED ALWAYS AS (search_attributes->>'Keyword04')               STORED,
  Keyword05       VARCHAR(255)    GENERATED ALWAYS AS (search_attributes->>'Keyword05')               STORED,
  Keyword06       VARCHAR(255)    GENERATED ALWAYS AS (search_attributes->>'Keyword06')               STORED,
  Keyword07       VARCHAR(255)    GENERATED ALWAYS AS (search_attributes->>'Keyword07')               STORED,
  Keyword08       VARCHAR(255)    GENERATED ALWAYS AS (search_attributes->>'Keyword08')               STORED,
  Keyword09       VARCHAR(255)    GENERATED ALWAYS AS (search_attributes->>'Keyword09')               STORED,
  Keyword10       VARCHAR(255)    GENERATED ALWAYS AS (search_attributes->>'Keyword10')               STORED,
  Text01          TEXT            GENERATED ALWAYS AS (search_attributes->>'Text01')      STORED,
  Text02          TEXT            GENERATED ALWAYS AS (search_attributes->>'Text02')      STORED,
  Text03          TEXT            GENERATED ALWAYS AS (search_attributes->>'Text03')      STORED,
  KeywordList01   JSONB           GENERATED ALWAYS AS (search_attributes->'KeywordList01')            STORED,
  KeywordList02   JSONB           GENERATED ALWAYS AS (search_attributes->'KeywordList02')            STORED,
  KeywordList03   JSONB           GENERATED ALWAYS AS (search_attributes->'KeywordList03')            STORED,

  -- Pre-allocated CHASM search attributes
  TemporalBool01                  BOOLEAN         GENERATED ALWAYS AS ((search_attributes->'TemporalBool01')::boolean)        STORED,
  TemporalBool02                  BOOLEAN         GENERATED ALWAYS AS ((search_attributes->'TemporalBool02')::boolean)        STORED,
  TemporalDatetime01              TIMESTAMP       GENERATED ALWAYS AS (convert_ts(search_attributes->>'TemporalDatetime01'))  STORED,
  TemporalDatetime02              TIMESTAMP       GENERATED ALWAYS AS (convert_ts(search_attributes->>'TemporalDatetime02'))  STORED,
  TemporalDouble01                DECIMAL(20, 5)  GENERATED ALWAYS AS ((search_attributes->'TemporalDouble01')::decimal)      STORED,
  TemporalDouble02                DECIMAL(20, 5)  GENERATED ALWAYS AS ((search_attributes->'TemporalDouble02')::decimal)      STORED,
  TemporalInt01                   BIGINT          GENERATED ALWAYS AS ((search_attributes->'TemporalInt01')::bigint)          STORED,
  TemporalInt02                   BIGINT          GENERATED ALWAYS AS ((search_attributes->'TemporalInt02')::bigint)          STORED,
  TemporalKeyword01               VARCHAR(255)    GENERATED ALWAYS AS (search_attributes->>'TemporalKeyword01')               STORED,
  TemporalKeyword02               VARCHAR(255)    GENERATED ALWAYS AS (search_attributes->>'TemporalKeyword02')               STORED,
  TemporalKeyword03               VARCHAR(255)    GENERATED ALWAYS AS (search_attributes->>'TemporalKeyword03')               STORED,
  TemporalKeyword04               VARCHAR(255)    GENERATED ALWAYS AS (search_attributes->>'TemporalKeyword04')               STORED,
  TemporalLowCardinalityKeyword01 VARCHAR(255)    GENERATED ALWAYS AS (search_attributes->>'TemporalLowCardinalityKeyword01') STORED,
  TemporalKeywordList01           JSONB           GENERATED ALWAYS AS (search_attributes->'TemporalKeywordList01')            STORED,
  TemporalKeywordList02           JSONB           GENERATED ALWAYS AS (search_attributes->'TemporalKeywordList02')            STORED,

  PRIMARY KEY  (namespace_id, run_id)
);


CREATE INDEX visibility_core_bm25 ON executions_visibility USING paradedb ((namespace_id::pdb.literal), (run_id::pdb.literal), (workflow_id::pdb.literal), (workflow_type_name::pdb.literal), status, start_time, close_time, sort_close_time, execution_time, (task_queue::pdb.literal), history_length, history_size_bytes, execution_duration, state_transition_count, (Text01::pdb.whitespace('lowercase=false')), (Text02::pdb.whitespace('lowercase=false')), (Text03::pdb.whitespace('lowercase=false')), (Keyword01::pdb.literal), (Keyword02::pdb.literal), (Keyword03::pdb.literal), (Keyword04::pdb.literal), (Keyword05::pdb.literal), (Keyword06::pdb.literal), (Keyword07::pdb.literal), (Keyword08::pdb.literal), (Keyword09::pdb.literal), (Keyword10::pdb.literal), (KeywordList01::pdb.literal), (KeywordList02::pdb.literal), (KeywordList03::pdb.literal));
CREATE INDEX default_idx                ON executions_visibility (namespace_id, (COALESCE(close_time, '9999-12-31 23:59:59')) DESC, start_time DESC, run_id);
CREATE INDEX by_parent_workflow_id      ON executions_visibility (namespace_id, parent_workflow_id,     (COALESCE(close_time, '9999-12-31 23:59:59')) DESC, start_time DESC, run_id);
CREATE INDEX by_parent_run_id           ON executions_visibility (namespace_id, parent_run_id,          (COALESCE(close_time, '9999-12-31 23:59:59')) DESC, start_time DESC, run_id);
CREATE INDEX by_root_workflow_id        ON executions_visibility (namespace_id, root_workflow_id,       (COALESCE(close_time, '9999-12-31 23:59:59')) DESC, start_time DESC, run_id);
CREATE INDEX by_root_run_id             ON executions_visibility (namespace_id, root_run_id,            (COALESCE(close_time, '9999-12-31 23:59:59')) DESC, start_time DESC, run_id);
CREATE INDEX by_temporal_change_version       ON executions_visibility USING GIN (namespace_id, TemporalChangeVersion jsonb_path_ops);
CREATE INDEX by_binary_checksums              ON executions_visibility USING GIN (namespace_id, BinaryChecksums jsonb_path_ops);
CREATE INDEX by_build_ids                     ON executions_visibility USING GIN (namespace_id, BuildIds jsonb_path_ops);
CREATE INDEX by_temporal_pause_info           ON executions_visibility USING GIN (namespace_id, TemporalPauseInfo jsonb_path_ops);
CREATE INDEX by_temporal_reported_problems     ON executions_visibility USING GIN (namespace_id, TemporalReportedProblems jsonb_path_ops);
CREATE INDEX by_temporal_worker_deployment_version ON executions_visibility (namespace_id, TemporalWorkerDeploymentVersion,  (COALESCE(close_time, '9999-12-31 23:59:59')) DESC, start_time DESC, run_id);
CREATE INDEX by_temporal_workflow_versioning_behavior ON executions_visibility (namespace_id, TemporalWorkflowVersioningBehavior,  (COALESCE(close_time, '9999-12-31 23:59:59')) DESC, start_time DESC, run_id);
CREATE INDEX by_temporal_worker_deployment    ON executions_visibility (namespace_id, TemporalWorkerDeployment,  (COALESCE(close_time, '9999-12-31 23:59:59')) DESC, start_time DESC, run_id);
CREATE INDEX by_used_deployment_versions      ON executions_visibility USING GIN (namespace_id, TemporalUsedWorkerDeploymentVersions jsonb_path_ops);
CREATE INDEX by_batcher_user                  ON executions_visibility (namespace_id, BatcherUser,                (COALESCE(close_time, '9999-12-31 23:59:59')) DESC, start_time DESC, run_id);
CREATE INDEX by_temporal_scheduled_start_time ON executions_visibility (namespace_id, TemporalScheduledStartTime, (COALESCE(close_time, '9999-12-31 23:59:59')) DESC, start_time DESC, run_id);
CREATE INDEX by_temporal_scheduled_by_id      ON executions_visibility (namespace_id, TemporalScheduledById,      (COALESCE(close_time, '9999-12-31 23:59:59')) DESC, start_time DESC, run_id);
CREATE INDEX by_temporal_schedule_paused      ON executions_visibility (namespace_id, TemporalSchedulePaused,     (COALESCE(close_time, '9999-12-31 23:59:59')) DESC, start_time DESC, run_id);
CREATE INDEX by_temporal_namespace_division   ON executions_visibility (namespace_id, TemporalNamespaceDivision,  (COALESCE(close_time, '9999-12-31 23:59:59')) DESC, start_time DESC, run_id);
CREATE INDEX by_temporal_external_payload_size_bytes ON executions_visibility (namespace_id, TemporalExternalPayloadSizeBytes, (COALESCE(close_time, '9999-12-31 23:59:59')) DESC, start_time DESC, run_id);
CREATE INDEX by_temporal_external_payload_count ON executions_visibility (namespace_id, TemporalExternalPayloadCount, (COALESCE(close_time, '9999-12-31 23:59:59')) DESC, start_time DESC, run_id);
CREATE INDEX by_bool_01         ON executions_visibility (namespace_id, Bool01,     (COALESCE(close_time, '9999-12-31 23:59:59')) DESC, start_time DESC, run_id);
CREATE INDEX by_bool_02         ON executions_visibility (namespace_id, Bool02,     (COALESCE(close_time, '9999-12-31 23:59:59')) DESC, start_time DESC, run_id);
CREATE INDEX by_bool_03         ON executions_visibility (namespace_id, Bool03,     (COALESCE(close_time, '9999-12-31 23:59:59')) DESC, start_time DESC, run_id);
CREATE INDEX by_datetime_01     ON executions_visibility (namespace_id, Datetime01, (COALESCE(close_time, '9999-12-31 23:59:59')) DESC, start_time DESC, run_id);
CREATE INDEX by_datetime_02     ON executions_visibility (namespace_id, Datetime02, (COALESCE(close_time, '9999-12-31 23:59:59')) DESC, start_time DESC, run_id);
CREATE INDEX by_datetime_03     ON executions_visibility (namespace_id, Datetime03, (COALESCE(close_time, '9999-12-31 23:59:59')) DESC, start_time DESC, run_id);
CREATE INDEX by_double_01       ON executions_visibility (namespace_id, Double01,   (COALESCE(close_time, '9999-12-31 23:59:59')) DESC, start_time DESC, run_id);
CREATE INDEX by_double_02       ON executions_visibility (namespace_id, Double02,   (COALESCE(close_time, '9999-12-31 23:59:59')) DESC, start_time DESC, run_id);
CREATE INDEX by_double_03       ON executions_visibility (namespace_id, Double03,   (COALESCE(close_time, '9999-12-31 23:59:59')) DESC, start_time DESC, run_id);
CREATE INDEX by_int_01          ON executions_visibility (namespace_id, Int01,      (COALESCE(close_time, '9999-12-31 23:59:59')) DESC, start_time DESC, run_id);
CREATE INDEX by_int_02          ON executions_visibility (namespace_id, Int02,      (COALESCE(close_time, '9999-12-31 23:59:59')) DESC, start_time DESC, run_id);
CREATE INDEX by_int_03          ON executions_visibility (namespace_id, Int03,      (COALESCE(close_time, '9999-12-31 23:59:59')) DESC, start_time DESC, run_id);
CREATE INDEX by_temporal_bool_01                    ON executions_visibility (namespace_id, TemporalBool01, (COALESCE(close_time, '9999-12-31 23:59:59')) DESC, start_time DESC, run_id);
CREATE INDEX by_temporal_bool_02                    ON executions_visibility (namespace_id, TemporalBool02, (COALESCE(close_time, '9999-12-31 23:59:59')) DESC, start_time DESC, run_id);
CREATE INDEX by_temporal_datetime_01                ON executions_visibility (namespace_id, TemporalDatetime01, (COALESCE(close_time, '9999-12-31 23:59:59')) DESC, start_time DESC, run_id);
CREATE INDEX by_temporal_datetime_02                ON executions_visibility (namespace_id, TemporalDatetime02, (COALESCE(close_time, '9999-12-31 23:59:59')) DESC, start_time DESC, run_id);
CREATE INDEX by_temporal_double_01                  ON executions_visibility (namespace_id, TemporalDouble01, (COALESCE(close_time, '9999-12-31 23:59:59')) DESC, start_time DESC, run_id);
CREATE INDEX by_temporal_double_02                  ON executions_visibility (namespace_id, TemporalDouble02, (COALESCE(close_time, '9999-12-31 23:59:59')) DESC, start_time DESC, run_id);
CREATE INDEX by_temporal_int_01                     ON executions_visibility (namespace_id, TemporalInt01, (COALESCE(close_time, '9999-12-31 23:59:59')) DESC, start_time DESC, run_id);
CREATE INDEX by_temporal_int_02                     ON executions_visibility (namespace_id, TemporalInt02, (COALESCE(close_time, '9999-12-31 23:59:59')) DESC, start_time DESC, run_id);
CREATE INDEX by_temporal_keyword_01                 ON executions_visibility (namespace_id, TemporalKeyword01, (COALESCE(close_time, '9999-12-31 23:59:59')) DESC, start_time DESC, run_id);
CREATE INDEX by_temporal_keyword_02                 ON executions_visibility (namespace_id, TemporalKeyword02, (COALESCE(close_time, '9999-12-31 23:59:59')) DESC, start_time DESC, run_id);
CREATE INDEX by_temporal_keyword_03                 ON executions_visibility (namespace_id, TemporalKeyword03, (COALESCE(close_time, '9999-12-31 23:59:59')) DESC, start_time DESC, run_id);
CREATE INDEX by_temporal_keyword_04                 ON executions_visibility (namespace_id, TemporalKeyword04, (COALESCE(close_time, '9999-12-31 23:59:59')) DESC, start_time DESC, run_id);
CREATE INDEX by_temporal_low_cardinality_keyword_01 ON executions_visibility (namespace_id, TemporalLowCardinalityKeyword01, (COALESCE(close_time, '9999-12-31 23:59:59')) DESC, start_time DESC, run_id);
CREATE INDEX by_temporal_keyword_list_01            ON executions_visibility USING GIN (namespace_id, TemporalKeywordList01 jsonb_path_ops);
CREATE INDEX by_temporal_keyword_list_02            ON executions_visibility USING GIN (namespace_id, TemporalKeywordList02 jsonb_path_ops);

```
