# Visibility 基准测试结果（2026-10-16，4 核云主机）

## 环境

- 4 vCPU / 15 GB 内存云容器，Docker 29.8，PostgreSQL 17.11（`postgres:17.11` 和 `paradedb/paradedb:v0.26.1-pg17`）。
- 压测程序与数据库在同一台主机上；测试期间没有其他容器占用 CPU。
- 每个变体用全新 volume，预加载 1,000,000 行。

| 运行 | 脚本 | 配置 |
|---|---|---|
| `baseline/` | 原版 `compare.sh`（仅加 `shm_size: 2gb`） | `shared_buffers=8GB`，16/64 writers 各 15 分钟，8 个闭环 reader，每个变体 1 轮 |
| `fixed/` | 修正版 `compare.sh` | `shared_buffers=4GB`，计时前 VACUUM + CHECKPOINT，单查询读容量各 60 秒，16/64 writers 各 10 分钟，8 个 reader 按每条查询 2 次/秒固定速率发起读，2 轮并轮换变体顺序 |

复现修正版：

```sh
BENCH_REPEATS=2 BENCH_DURATION=10m BENCH_READ_RATE=2 BENCH_SHARED_BUFFERS=4GB BENCH_EFFECTIVE_CACHE_SIZE=10GB ./compare.sh <output-dir>
```

## 基线的已知缺陷

1. 除 namespace 外的属性都由 `id` 派生，导致 `type-0`、`tenant-0/20`、`tag-0` 和 `hello` 只出现在 namespace 0。约 90% 的搜索属性查询返回空结果。
2. reader 轮流执行 7 条查询，每条查询的 Reads/s 被最慢的查询拖住，没有意义。
3. 闭环读使各变体承受的读压力不同，写入数据不在同等条件下。
4. Docker 默认 64 MB `/dev/shm` 使并行查询报 `could not resize shared memory segment`。

修正版逐一修复了以上问题，并在 10 个 namespace 上校验结果一致。

## 修正版结果（2 轮中位数）

### 读容量（无写入，单查询闭环）

| 查询 | postgres | pdb-cols | pdb-cols-hybrid | pdb-json-hybrid |
|---|---:|---:|---:|---:|
| status_time QPS (p50 ms) | 16554 (0.4) | 119 (64) | **19634 (0.4)** | 5288 (1.3) |
| type_status | 16384 (0.4) | 126 (61) | **19813 (0.3)** | 5289 (1.4) |
| count | **419 (19)** | 98 (79) | 186 (42) | 307 (30) |
| group_count | **67 (118)** | 43 (181) | 18 (448) | 45 (282) |
| text | **7067 (1.0)** | 124 (62) | 127 (60) | 113 (68) |
| keyword | **2671 (2.8)** | 129 (60) | 123 (62) | 108 (71) |
| keyword_list | **4422 (1.6)** | 137 (57) | 124 (61) | 108 (71) |

### 写入

| 指标 | postgres | pdb-cols | pdb-cols-hybrid | pdb-json-hybrid |
|---|---:|---:|---:|---:|
| 16 writers 写入/s（第 1 / 2 轮） | 595 / 641 | 1749 / 1969 | 1829 / 1608 | **3745 / 3074** |
| 16 writers p50 / p99 ms | 14.5 / 170 | 7.2 / 29 | 8.0 / 29 | **3.5 / 19** |
| 64 writers 写入/s（第 1 / 2 轮） | 1052 / 1100 | 2229 / 2576 | 2205 / 1905 | **5189 / 4727** |
| 64 writers p50 / p99 ms | 36 / 316 | 22 / 133 | 24 / 164 | **10.8 / 47** |
| 64 级结束后索引大小 | 17.9 GB | 9.2 GB | 10.0 GB | **2.3 GB** |

### 混合负载下的读 p50 / p99 ms（括号内为 2 轮 missed 合计）

| 查询 | postgres | pdb-cols | pdb-cols-hybrid | pdb-json-hybrid |
|---|---:|---:|---:|---:|
| status_time @16 | **1.8 / 13** (0) | 179 / 1134 (26) | 3.3 / 19 (3) | 3.3 / 16 (1) |
| count @16 | **52 / 198** (0) | 178 / 914 (34) | 126 / 277 (1) | 161 / 359 (5) |
| text @16 | **2.7 / 17** (0) | 161 / 948 (28) | 159 / 910 (0) | 210 / 1175 (2) |
| keyword @16 | **5.6 / 28** (0) | 145 / 967 (26) | 137 / 872 (1) | 196 / 1203 (3) |
| status_time @64 | **2.4 / 33** (0) | 428 / 3844 (729) | 2.9 / 46 (507) | 4.0 / 43 (854) |
| text @64 | **4.0 / 62** (0) | 412 / 4171 (766) | 400 / 3587 (588) | 1165 / 11782 (968) |
| keyword @64 | **9.4 / 101** (0) | 394 / 3275 (761) | 401 / 4197 (520) | 1098 / 10923 (1122) |

完整表格见 `fixed/summary.md`，基线见 `baseline/summary.md`，原始数据在各变体目录下。

## 结论

1. **写入**：`pdb-json-hybrid` 写入吞吐约为 postgres 的 4.5–5 倍，p99 低一个数量级，索引约为 1/8；列式 ParadeDB 变体约为 2–3 倍。基线与修正版结论一致，主要源于 v12 schema 的大量 B-tree/GIN 索引维护成本。
2. **核心字段查询**必须走 B-tree：`pdb-cols` 全走 BM25 时 `status_time` 只有 119 QPS，hybrid 变体与 postgres 持平。
3. **搜索属性查询取决于选择性**：匹配率 2%–10% 时，`LIMIT 20` 配合按时间排序的 B-tree 很快就能凑满结果，postgres 比 BM25 快 20–60 倍。基线中多为空 namespace，postgres 需要扫完整个 namespace（约 700 ms），BM25 反而更快。需要补充低选择性场景再下结论。
4. ParadeDB BM25 查询有约 60 ms 固定开销，单查询上限约 110–140 QPS；64 writers 下恶化到 p50 0.4–1.2 秒，`pdb-json-hybrid` 的 p99 约 11 秒。
5. `missed` 来自共享 reader 池被慢的 BM25 查询占满，并不代表该查询本身慢；postgres 两轮均为 0。
6. `pdb-json-hybrid` 两轮写入相差约 20%，需要更多轮次才能给出精确倍数。
