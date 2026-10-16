# Visibility benchmark results (2026-10-16, 4-CPU cloud host)

## Environment

- 4 vCPU / 15 GB RAM cloud container, Docker 29.8, PostgreSQL 17.11 (`postgres:17.11` and `paradedb/paradedb:v0.26.1-pg17`).
- The load generator ran on the same host as the database; no other containers used CPU during the runs.
- Every variant started from a fresh volume preloaded with 1,000,000 rows.

| Run | Script | Configuration |
|---|---|---|
| `baseline/` | original `compare.sh` (plus `shm_size: 2gb` only) | `shared_buffers=8GB`, 16 and 64 writers for 15 minutes each, 8 closed-loop readers, one round per variant |
| `fixed/` | fixed `compare.sh` | `shared_buffers=4GB`, VACUUM + CHECKPOINT before timing, 60 s read-only capacity per query, 16 and 64 writers for 10 minutes each, 8 readers offering a fixed 2 reads/s per query, two rounds with rotated variant order |

To reproduce the fixed run:

```sh
BENCH_REPEATS=2 BENCH_DURATION=10m BENCH_READ_RATE=2 BENCH_SHARED_BUFFERS=4GB BENCH_EFFECTIVE_CACHE_SIZE=10GB ./compare.sh <output-dir>
```

## Known flaws in the baseline

1. Every attribute except the namespace was derived from `id`, so `type-0`, `tenant-0/20`, `tag-0` and `hello` only existed in namespace 0. About 90% of search-attribute reads returned nothing.
2. Readers ran the 7 queries in strict rotation, so every query's reads/s was paced by the slowest one and is not meaningful.
3. Closed-loop reads put different read pressure on each variant, so writes were not measured under equal conditions.
4. Docker's default 64 MB `/dev/shm` made parallel queries fail with `could not resize shared memory segment`.

The fixed run addresses each of these and checks that results match across variants in all 10 namespaces.

## Fixed run results (median of two rounds)

### Read capacity (no writes, one query at a time, closed loop)

| Query | postgres | pdb-cols | pdb-cols-hybrid | pdb-json-hybrid |
|---|---:|---:|---:|---:|
| status_time QPS (p50 ms) | 16554 (0.4) | 119 (64) | **19634 (0.4)** | 5288 (1.3) |
| type_status | 16384 (0.4) | 126 (61) | **19813 (0.3)** | 5289 (1.4) |
| count | **419 (19)** | 98 (79) | 186 (42) | 307 (30) |
| group_count | **67 (118)** | 43 (181) | 18 (448) | 45 (282) |
| text | **7067 (1.0)** | 124 (62) | 127 (60) | 113 (68) |
| keyword | **2671 (2.8)** | 129 (60) | 123 (62) | 108 (71) |
| keyword_list | **4422 (1.6)** | 137 (57) | 124 (61) | 108 (71) |

### Writes

| Metric | postgres | pdb-cols | pdb-cols-hybrid | pdb-json-hybrid |
|---|---:|---:|---:|---:|
| 16 writers, writes/s (round 1 / 2) | 595 / 641 | 1749 / 1969 | 1829 / 1608 | **3745 / 3074** |
| 16 writers, p50 / p99 ms | 14.5 / 170 | 7.2 / 29 | 8.0 / 29 | **3.5 / 19** |
| 64 writers, writes/s (round 1 / 2) | 1052 / 1100 | 2229 / 2576 | 2205 / 1905 | **5189 / 4727** |
| 64 writers, p50 / p99 ms | 36 / 316 | 22 / 133 | 24 / 164 | **10.8 / 47** |
| Index size after the 64-writer level | 17.9 GB | 9.2 GB | 10.0 GB | **2.3 GB** |

### Reads under mixed load, p50 / p99 ms (missed reads summed over both rounds)

| Query | postgres | pdb-cols | pdb-cols-hybrid | pdb-json-hybrid |
|---|---:|---:|---:|---:|
| status_time @16 | **1.8 / 13** (0) | 179 / 1134 (26) | 3.3 / 19 (3) | 3.3 / 16 (1) |
| count @16 | **52 / 198** (0) | 178 / 914 (34) | 126 / 277 (1) | 161 / 359 (5) |
| text @16 | **2.7 / 17** (0) | 161 / 948 (28) | 159 / 910 (0) | 210 / 1175 (2) |
| keyword @16 | **5.6 / 28** (0) | 145 / 967 (26) | 137 / 872 (1) | 196 / 1203 (3) |
| status_time @64 | **2.4 / 33** (0) | 428 / 3844 (729) | 2.9 / 46 (507) | 4.0 / 43 (854) |
| text @64 | **4.0 / 62** (0) | 412 / 4171 (766) | 400 / 3587 (588) | 1165 / 11782 (968) |
| keyword @64 | **9.4 / 101** (0) | 394 / 3275 (761) | 401 / 4197 (520) | 1098 / 10923 (1122) |

A missed read is a scheduled read the client dropped because all readers were busy; it is not a wrong or empty result. Full tables are in `fixed/summary.md`, the baseline in `baseline/summary.md`, and raw data under each variant directory.

## Conclusions

1. **Writes:** `pdb-json-hybrid` sustains 4.5–5x the write throughput of `postgres`, with p99 an order of magnitude lower and about 1/8 of the index size. The column-based ParadeDB variants reach 2–3x. Baseline and fixed runs agree; the gap comes mostly from maintaining the many B-tree/GIN indexes in the v12 schema.
2. **Core-field queries need B-tree.** With every query routed through BM25, `pdb-cols` serves `status_time` at only 119 QPS; the hybrid variants match `postgres`.
3. **Search-attribute queries depend on selectivity.** At 2–10% match rates, `LIMIT 20` over the time-ordered B-tree fills quickly, and `postgres` is 20–60x faster than BM25. In the baseline most reads hit empty namespaces, where `postgres` had to scan the whole namespace (about 700 ms) and BM25 came out ahead. A low-selectivity scenario is still needed before drawing a conclusion.
4. ParadeDB BM25 queries carry about 60 ms of fixed cost and top out around 110–140 QPS per query. At 64 writers they degrade to 0.4–1.2 s p50, with `pdb-json-hybrid` reaching about 11 s p99.
5. Missed reads come from the shared reader pool filling up behind slow BM25 queries, not from the query that missed being slow itself; `postgres` missed none in either round.
6. `pdb-json-hybrid` write throughput differed by about 20% between rounds; more rounds are needed for precise ratios.
