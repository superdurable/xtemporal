# Visibility comparison

Run the four variants sequentially on fresh Docker volumes:

```sh
./compare.sh /tmp/visibility-compare
```

The default workload preloads 1,000,000 rows, then runs 16 and 64 writers for 15 minutes each with 8 readers. The script checks all seven read results against the first variant before timing, captures their EXPLAIN plans, and writes `summary.md`. It skips variants with a `done` marker when resumed. Remove a variant's result directory to rerun it. The runner tears down its own Compose project and volumes after each variant.

Short smoke test:

```sh
BENCH_ROWS=20000 BENCH_DURATION=30s BENCH_CONNECTIONS=4 BENCH_READERS=2 ./compare.sh /tmp/bench-compare-smoke
```

Configuration: `BENCH_VARIANTS` (space-separated), `BENCH_ROWS`, `BENCH_DURATION`, `BENCH_CONNECTIONS` (space-separated), `BENCH_READERS`, `PARADEDB_TAG`, `BENCH_RUN_CRASH=1`, `BENCH_PORT` (default 56640), and `BENCH_PROJECT`. `summarize.py <output-dir>` regenerates the summary from completed variants.

`run.sh` remains the original two-container diagnostic harness. Use `compare.sh` for the fair sequential comparison.

Index sets differ by design. `postgres` uses all v12 indexes. `pdb-cols` and `pdb-cols-hybrid` keep the column schema's B-tree/GIN indexes on pre-allocated and Temporal generated columns. `pdb-json-hybrid` stores those attributes in JSON, so it has only the BM25 index plus `default_idx`. Both hybrid variants add the same core B-tree set from `core-indexes.sql`. Compare write cost with this in mind.
