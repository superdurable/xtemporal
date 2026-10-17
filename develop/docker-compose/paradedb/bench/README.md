# Visibility comparison

Run the four variants sequentially on fresh Docker volumes:

```sh
./compare.sh /tmp/visibility-compare
```

The default workload preloads 1,000,000 rows, vacuums and checkpoints, then measures each read query alone (read-only, closed loop, 60 s each) to get its capacity. It then runs 16 and 64 writers for 15 minutes each while 8 readers offer a fixed 10 reads/s per query, so every variant faces the same read pressure; reads a variant cannot keep up with are reported as `missed`. The script checks all seven read results in all ten namespaces against the first variant before timing, captures their EXPLAIN plans, and writes `summary.md` with medians across rounds. It skips variants with a `done` marker when resumed. Remove a variant's result directory to rerun it. The runner tears down its own Compose project and volumes after each variant.

Every search attribute is derived from `id / 10` while the namespace is `id % 10`, so each namespace has the same selectivity for every query.

Short smoke test:

```sh
BENCH_ROWS=20000 BENCH_DURATION=30s BENCH_CONNECTIONS=4 BENCH_READERS=2 BENCH_CAPACITY_DURATION=5s ./compare.sh /tmp/bench-compare-smoke
```

Configuration: `BENCH_VARIANTS` (space-separated), `BENCH_ROWS`, `BENCH_DURATION`, `BENCH_CONNECTIONS` (space-separated), `BENCH_READERS`, `BENCH_READ_RATE` (reads/s per query during mixed load), `BENCH_CAPACITY_DURATION`, `BENCH_REPEATS` (rounds; the variant order rotates each round), `BENCH_SHARED_BUFFERS` and `BENCH_EFFECTIVE_CACHE_SIZE` (defaults 8GB and 24GB; lower them on small hosts), `PARADEDB_TAG`, `BENCH_RUN_CRASH=1`, `BENCH_PORT` (default 56640), and `BENCH_PROJECT`. `summarize.py <output-dir>` regenerates the summary from completed variants.

`run.sh` remains the original two-container diagnostic harness. Use `compare.sh` for the fair sequential comparison.

Index sets differ by design. `postgres` uses all v12 indexes. `pdb-cols` and `pdb-cols-hybrid` keep the column schema's B-tree/GIN indexes on pre-allocated and Temporal generated columns. `pdb-json-hybrid` stores those attributes in JSON, so it has only the BM25 index plus `default_idx`. Both hybrid variants add the same core B-tree set from `core-indexes.sql`. Compare write cost with this in mind.

## Dex-profile benchmark

`dex-compare.sh <output-dir>` runs the benchmark specified in [dex-profile-benchmark.md](dex-profile-benchmark.md) against [dex-profile-schema.sql](dex-profile-schema.sql). The Go program in `dex/` does the work:

1. Preload a deterministic Dex-like dataset with parallel COPY.
2. Record EXPLAIN evidence for every query shape.
3. Run console "browsers" polling every 10 s against store-style writers (version-guarded upserts and tombstoned deletes), with and without `by_flow_type_status`. The order is A16, B16, B64, A64.
4. Measure the residual-filter worst case and page-token stability.

Steps leave markers in the output directory and the database volume is kept until the end, so rerunning the same command resumes. `dex-summarize.py <output-dir>` regenerates `summary.md`.

Configuration: `BENCH_NAMESPACES`, `BENCH_ROWS`, `BENCH_HEAVY_ROWS` (one heavy namespace for the worst-case and paging tests), `BENCH_DURATION`, `BENCH_BROWSERS`, `BENCH_POOL`, `BENCH_REPS`, `BENCH_WORST_WRITERS`, `BENCH_PAGING_WRITERS`, `BENCH_SHARED_BUFFERS`, `BENCH_EFFECTIVE_CACHE_SIZE`, `BENCH_KEEP_VOLUME=1`, `BENCH_PORT` (default 56650) and `BENCH_PROJECT`. For the 100M tier, set `BENCH_NAMESPACES=10000 BENCH_ROWS=95000000 BENCH_HEAVY_ROWS=5000000` on a host with at least 200 GB of free disk.

Short smoke test:

```sh
BENCH_NAMESPACES=100 BENCH_ROWS=100000 BENCH_HEAVY_ROWS=50000 BENCH_DURATION=30s BENCH_BROWSERS=20 BENCH_REPS=3 ./dex-compare.sh /tmp/dex-smoke
```
