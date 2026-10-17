# Dex-profile benchmark summary

```
date: 2026-10-16T23:27:41Z
cpus: 4
model name	: Intel(R) Xeon(R) Processor @ 2.80GHz
               total        used        free      shared  buff/cache   available
Mem:              15           4           0           4          15          10
/dev/vda        252G   32G  7.1G  82% /
docker 29.8.2
image: paradedb/paradedb:v0.26.1-pg17
shared_buffers: 4GB effective_cache_size: 10GB
load generator: same host as the database
```

Loaded 12,000,000 rows in 260 s; vacuum and analyze 30 s. Index builds: by_flow_type 48 s, by_namespace_division 2 s, by_workflow_id 39 s, default_idx 45 s, visibility_core_bm25 460 s.
`by_flow_type_status` built in 56 s, 1576 MiB.

## Read and write latency

### 16 writers

| Shape | no C3 QPS | no C3 p50 | no C3 p95 | no C3 p99 | C3 QPS | C3 p50 | C3 p95 | C3 p99 | C3, custom plans QPS | C3, custom plans p50 | C3, custom plans p95 | C3, custom plans p99 |
|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| main_list | 57.5 | 11.1 | 42.0 | 67.5 | 43.1 | 10635.9 | 20573.3 | 22771.6 | 49.5 | 32.6 | 2133.7 | 5037.5 |
| need_person | 57.5 | 8.5 | 35.9 | 56.3 | 43.1 | 10634.5 | 20570.8 | 22771.4 | 49.5 | 25.4 | 2085.9 | 4990.6 |
| done_today | 57.5 | 8.6 | 40.1 | 81.0 | 43.0 | 10727.1 | 20848.9 | 23133.6 | 49.5 | 30.9 | 2194.5 | 5078.3 |
| running | 57.5 | 8.1 | 34.9 | 52.7 | 43.1 | 10642.4 | 20577.3 | 22779.7 | 49.5 | 25.2 | 2068.3 | 4984.9 |
| custom_eq | 57.5 | 9.3 | 43.1 | 102.4 | 43.0 | 10727.6 | 20965.2 | 23270.3 | 49.5 | 31.3 | 2206.6 | 5088.8 |
| custom_range | 57.5 | 9.5 | 43.9 | 105.0 | 43.0 | 10731.4 | 21001.4 | 23282.3 | 49.5 | 32.0 | 2183.0 | 5101.5 |
| workflow_id | 57.5 | 8.1 | 36.8 | 55.3 | 43.1 | 10649.9 | 20595.8 | 22784 | 49.5 | 28.0 | 2095.5 | 5003.2 |
| readyz | 0.6 | 9.3 | 37.2 | 54.2 | 0.1 | 9237.3 | 15112.2 | 20856.9 | 0.6 | 34.5 | 144.5 | 2654.4 |
| schedules | 6.0 | 5.4 | 29.8 | 46.6 | 4.5 | 10845.5 | 14783.2 | 16929.3 | 5.2 | 20.5 | 69.3 | 142.2 |
| text_bm25 | 6.5 | 1986.4 | 11825.5 | 13913.0 | 4.9 | 3970.8 | 26032.5 | 32058.2 | 5.4 | 5882.3 | 20560.9 | 23534.6 |
| custom_only_bm25 | 6.0 | 3335.3 | 15335.3 | 17436.4 | 4.6 | 4004.5 | 29991.8 | 37238.6 | 5.2 | 11156.2 | 21472.2 | 54703.5 |

| Variant | write statements/s | polls | late polls |
|---|---:|---:|---:|
| no C3 | 1277.2 | 5755 | 607 |
| C3 | 631.1 | 4419 | 2591 |
| C3, custom plans | 782.8 | 4949 | 1517 |

| Write op | no C3 /s | no C3 p50 | no C3 p99 | C3 /s | C3 p50 | C3 p99 | C3, custom plans /s | C3, custom plans p50 | C3, custom plans p99 |
|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| close | 331.8 | 8.1 | 56.4 | 163.7 | 14.3 | 137.2 | 203.7 | 14.7 | 76.0 |
| continue_as_new_close | 166.2 | 7.8 | 54.4 | 82.3 | 15.2 | 154.2 | 101.9 | 15.9 | 82.2 |
| continue_as_new_start | 166.1 | 8.5 | 57.7 | 82.3 | 16.3 | 159.9 | 101.8 | 15.8 | 81.9 |
| delete | 111.1 | 7.2 | 49.9 | 55.1 | 14.1 | 148.6 | 68.0 | 13.9 | 68.9 |
| start | 280.6 | 9.2 | 62.4 | 139.3 | 21.0 | 204.1 | 172.5 | 19.2 | 105.3 |
| upsert | 221.4 | 7.8 | 54.6 | 108.5 | 13.9 | 123.9 | 134.9 | 14.4 | 74.5 |

### 64 writers

| Shape | no C3 QPS | no C3 p50 | no C3 p95 | no C3 p99 | C3 QPS | C3 p50 | C3 p95 | C3 p99 | C3, custom plans QPS | C3, custom plans p50 | C3, custom plans p95 | C3, custom plans p99 |
|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| main_list | 5.6 | 37.6 | 6434.1 | 6685.3 | 9.9 | 22.0 | 159.6 | 278.0 | 12.4 | 35.5 | 117.2 | 405.1 |
| need_person | 5.6 | 32.9 | 6542.1 | 7090.9 | 9.9 | 14.6 | 110.5 | 249.2 | 12.4 | 27.7 | 136.1 | 507.1 |
| done_today | 5.6 | 33.3 | 6552.8 | 7002.7 | 9.9 | 22.8 | 315.9 | 871.8 | 12.4 | 36.3 | 166.1 | 525.0 |
| running | 5.6 | 32.7 | 6527.1 | 7069.6 | 9.9 | 14.1 | 104.8 | 252.1 | 12.4 | 27.5 | 107.8 | 375.6 |
| custom_eq | 5.6 | 36.6 | 6546.2 | 7051.4 | 9.9 | 23.2 | 322.4 | 785.5 | 12.4 | 35.5 | 182.7 | 540.7 |
| custom_range | 5.6 | 36.0 | 6552.4 | 6966.7 | 9.9 | 23.6 | 315.8 | 783.6 | 12.4 | 35.7 | 177.8 | 477.4 |
| workflow_id | 5.6 | 35.6 | 6454.3 | 6696.4 | 9.9 | 17.6 | 111.6 | 242.7 | 12.4 | 29.0 | 102.8 | 376.9 |
| readyz | 0.6 | 22.7 | 105.1 | 4378.1 | 0.6 | 35.4 | 138.6 | 178.5 | 0.6 | 19.4 | 77.8 | 107.7 |
| schedules | 0.3 | 23.9 | 70.8 | 110.0 | 0.7 | 20.3 | 61.7 | 95.2 | 1.1 | 21.4 | 66.5 | 95.1 |
| text_bm25 | 0.7 | 117721.3 | 190947.8 | 196338.7 | 1.3 | 45433.8 | 128910.0 | 134875.6 | 1.3 | 55950.5 | 96894.3 | 103262.5 |
| custom_only_bm25 | 0.3 | 177742.1 | 194709.4 | 200304.1 | 0.7 | 96790.7 | 119423.6 | 130839.9 | 1.1 | 67826.6 | 106802.8 | 115389.0 |

| Variant | write statements/s | polls | late polls |
|---|---:|---:|---:|
| no C3 | 1650.7 | 562 | 466 |
| C3 | 1430.0 | 991 | 734 |
| C3, custom plans | 1446.6 | 1242 | 850 |

| Write op | no C3 /s | no C3 p50 | no C3 p99 | C3 /s | C3 p50 | C3 p99 | C3, custom plans /s | C3, custom plans p50 | C3, custom plans p99 |
|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| close | 428.7 | 30.9 | 141.7 | 372.3 | 33.5 | 206.0 | 375.1 | 33.6 | 208.6 |
| continue_as_new_close | 213.6 | 30.7 | 150.7 | 184.5 | 34.8 | 220.8 | 186.7 | 35.2 | 231.0 |
| continue_as_new_start | 213.5 | 31.8 | 148.4 | 184.4 | 36.0 | 236.9 | 186.7 | 35.6 | 225.9 |
| delete | 143.1 | 30.1 | 103.7 | 126.1 | 31.0 | 116.8 | 125.9 | 31.8 | 116.0 |
| start | 366.0 | 34.1 | 162.2 | 317.1 | 41.0 | 260.6 | 321.3 | 39.4 | 256.5 |
| upsert | 285.7 | 30.6 | 153.3 | 245.5 | 33.4 | 227.4 | 250.9 | 33.9 | 222.4 |


## Plan evidence

### Plans without C3

| Shape / namespace | Intended | Custom plan | ms | Generic plan | ms |
|---|---:|---:|---:|---:|---:|
| main_list/heavy | by_flow_type | by_flow_type (ordered scan) + heap filter | 1.9 | by_flow_type (ordered scan) + heap filter | 0.2 |
| main_list/regular | by_flow_type | by_flow_type (ordered scan) + heap filter | 2.2 | by_flow_type (ordered scan) + heap filter | 0.3 |
| need_person/heavy | by_flow_type[_status] + residual | by_flow_type (bitmap + top-N sort) + heap filter | 9157.8 | by_flow_type (ordered scan) + heap filter | 0.6 |
| need_person/regular | by_flow_type[_status] + residual | by_flow_type (ordered scan) + heap filter | 4.2 | by_flow_type (ordered scan) + heap filter | 0.7 |
| done_today/heavy | by_flow_type + residual | by_flow_type (ordered scan) + heap filter | 15.7 | by_flow_type (ordered scan) + heap filter | 5.3 |
| done_today/regular | by_flow_type + residual | by_flow_type (ordered scan) + heap filter | 1.0 | by_flow_type (ordered scan) + heap filter | 0.7 |
| running/heavy | by_flow_type[_status] | by_flow_type (ordered scan) + heap filter | 0.4 | by_flow_type (ordered scan) + heap filter | 0.2 |
| running/regular | by_flow_type[_status] | by_flow_type (ordered scan) + heap filter | 1.3 | by_flow_type (ordered scan) + heap filter | 0.6 |
| custom_eq/heavy | by_flow_type + residual | by_flow_type (ordered scan) + heap filter | 22.8 | by_flow_type (ordered scan) + heap filter | 2.9 |
| custom_eq/regular | by_flow_type + residual | by_flow_type (ordered scan) + heap filter | 1.2 | by_flow_type (ordered scan) + heap filter | 0.6 |
| custom_range/heavy | by_flow_type + residual | by_flow_type (ordered scan) + heap filter | 10.7 | by_flow_type (ordered scan) + heap filter | 3.1 |
| custom_range/regular | by_flow_type + residual | by_flow_type (ordered scan) + heap filter | 1.2 | by_flow_type (ordered scan) + heap filter | 1.0 |
| workflow_id/heavy | by_workflow_id | by_workflow_id (scan + sort) + heap filter | 1.1 | by_workflow_id (scan + sort) + heap filter | 0.1 |
| workflow_id/regular | by_workflow_id | by_workflow_id (scan + sort) + heap filter | 0.7 | by_workflow_id (scan + sort) + heap filter | 0.1 |
| readyz/heavy | default_idx | default_idx (ordered scan) + heap filter | 2.9 | default_idx (ordered scan) + heap filter | 0.0 |
| readyz/regular | default_idx | default_idx (ordered scan) + heap filter | 2.6 | default_idx (ordered scan) + heap filter | 0.0 |
| schedules/heavy | by_namespace_division | by_namespace_division (ordered scan) | 1.9 | by_namespace_division (ordered scan) | 0.3 |
| schedules/regular | by_namespace_division | by_namespace_division (ordered scan) | 0.6 | by_namespace_division (ordered scan) | 0.0 |
| text_bm25/heavy | ParadeDB scan | ParadeDB TopKScanExecState | 105.3 | — | — |
| text_bm25/regular | ParadeDB scan | ParadeDB TopKScanExecState | 21.5 | — | — |
| custom_only_bm25/heavy | ParadeDB scan | ParadeDB TopKScanExecState | 131.5 | — | — |
| custom_only_bm25/regular | ParadeDB scan | ParadeDB TopKScanExecState | 9.8 | — | — |

### Plans with C3

| Shape / namespace | Intended | Custom plan | ms | Generic plan | ms |
|---|---:|---:|---:|---:|---:|
| main_list/heavy | by_flow_type | by_flow_type (ordered scan) + heap filter | 4.8 | by_flow_type (ordered scan) + heap filter | 0.6 |
| main_list/regular | by_flow_type | by_flow_type (ordered scan) + heap filter | 1.9 | by_flow_type (ordered scan) + heap filter | 0.2 |
| need_person/heavy | by_flow_type[_status] + residual | by_flow_type_status (bitmap + top-N sort) + heap filter | 18.9 | by_flow_type_status (ordered scan) + heap filter | 0.4 |
| need_person/regular | by_flow_type[_status] + residual | by_flow_type_status (ordered scan) + heap filter | 0.2 | by_flow_type_status (ordered scan) + heap filter | 0.1 |
| done_today/heavy | by_flow_type + residual | by_flow_type (ordered scan) + heap filter | 16.2 | by_flow_type_status (bitmap + top-N sort) + heap filter | 3993.4 |
| done_today/regular | by_flow_type + residual | by_flow_type_status (bitmap + top-N sort) + heap filter | 8.7 | by_flow_type_status (bitmap + top-N sort) + heap filter | 0.8 |
| running/heavy | by_flow_type[_status] | by_flow_type_status (ordered scan) + heap filter | 0.9 | by_flow_type_status (ordered scan) + heap filter | 0.3 |
| running/regular | by_flow_type[_status] | by_flow_type_status (ordered scan) + heap filter | 0.1 | by_flow_type_status (ordered scan) + heap filter | 0.1 |
| custom_eq/heavy | by_flow_type + residual | by_flow_type (ordered scan) + heap filter | 13.0 | by_flow_type_status (bitmap + top-N sort) + heap filter | 1483.4 |
| custom_eq/regular | by_flow_type + residual | by_flow_type_status (bitmap + top-N sort) + heap filter | 4.0 | by_flow_type_status (bitmap + top-N sort) + heap filter | 0.9 |
| custom_range/heavy | by_flow_type + residual | by_flow_type (ordered scan) + heap filter | 15.3 | by_flow_type_status (bitmap + top-N sort) + heap filter | 1652.4 |
| custom_range/regular | by_flow_type + residual | by_flow_type_status (bitmap + top-N sort) + heap filter | 2.9 | by_flow_type_status (bitmap + top-N sort) + heap filter | 1.1 |
| workflow_id/heavy | by_workflow_id | by_workflow_id (scan + sort) + heap filter | 1.0 | by_workflow_id (scan + sort) + heap filter | 0.1 |
| workflow_id/regular | by_workflow_id | by_workflow_id (scan + sort) + heap filter | 0.4 | by_workflow_id (scan + sort) + heap filter | 0.1 |
| readyz/heavy | default_idx | default_idx (ordered scan) + heap filter | 0.2 | default_idx (ordered scan) + heap filter | 0.1 |
| readyz/regular | default_idx | default_idx (ordered scan) + heap filter | 1.5 | default_idx (ordered scan) + heap filter | 0.1 |
| schedules/heavy | by_namespace_division | by_namespace_division (ordered scan) | 1.2 | by_namespace_division (ordered scan) | 0.3 |
| schedules/regular | by_namespace_division | by_namespace_division (ordered scan) | 0.2 | by_namespace_division (ordered scan) | 0.1 |
| text_bm25/heavy | ParadeDB scan | ParadeDB TopKScanExecState | 398.9 | — | — |
| text_bm25/regular | ParadeDB scan | ParadeDB TopKScanExecState | 49.5 | — | — |
| custom_only_bm25/heavy | ParadeDB scan | ParadeDB TopKScanExecState | 762.4 | — | — |
| custom_only_bm25/regular | ParadeDB scan | ParadeDB TopKScanExecState | 42.5 | — | — |


## Worst case

### Idle, no writers, BM25 merged (29 segments)

Flow rows and rare matches: heavy_flow_rows = 732986, heavy_rare_matches = 234, regular_flow_rows = 330, regular_rare_matches = 0.
Promoted index: 1513 MiB, built in 56.2 s.

| Variant / namespace / value | quiet p50 | quiet p99 | rows | fallbacks |
|---|---:|---:|---:|---:|
| bm25/heavy/absent | 48.7 | 51.6 | 0 | — |
| bm25/heavy/rare | 143.8 | 176.0 | 50 | — |
| bm25/regular/absent | 52.9 | 69.9 | 0 | — |
| bm25/regular/rare | 59.3 | 76.8 | 0 | — |
| bounded_1000/heavy/absent | 58.1 | 65.5 | 0 | 20 |
| bounded_1000/heavy/rare | 152.6 | 191.1 | 50 | 20 |
| bounded_1000/regular/absent | 1.4 | 2.5 | 0 | 0 |
| bounded_1000/regular/rare | 1.5 | 2.9 | 0 | 0 |
| bounded_10000/heavy/absent | 110.6 | 125.9 | 0 | 20 |
| bounded_10000/heavy/rare | 206.6 | 227.4 | 50 | 20 |
| bounded_10000/regular/absent | 1.4 | 2.3 | 0 | 0 |
| bounded_10000/regular/rare | 1.5 | 2.1 | 0 | 0 |
| btree_residual/heavy/absent | 1627.4 | 1756.3 | 0 | — |
| btree_residual/heavy/rare | 220.6 | 289.5 | 50 | — |
| btree_residual/regular/absent | 0.9 | 1.1 | 0 | — |
| btree_residual/regular/rare | 0.8 | 1.7 | 0 | — |
| promoted_column/heavy/absent | 0.4 | 0.4 | 0 | — |
| promoted_column/heavy/rare | 0.8 | 1.4 | 50 | — |
| promoted_column/regular/absent | 1.0 | 1.2 | 0 | — |
| promoted_column/regular/rare | 1.0 | 1.6 | 0 | — |

| Plan | Path | ms |
|---|---:|---:|
| bm25/heavy/absent | ParadeDB TopKScanExecState | 48.6 |
| bm25/heavy/rare | ParadeDB TopKScanExecState | 215.5 |
| bm25/regular/absent | ParadeDB TopKScanExecState | 54.2 |
| bm25/regular/rare | ParadeDB TopKScanExecState | 76.8 |
| btree_residual/heavy/absent | by_flow_type (ordered scan) + heap filter | 1639.9 |
| btree_residual/heavy/rare | by_flow_type (ordered scan) + heap filter | 260.7 |
| btree_residual/regular/absent | by_flow_type (ordered scan) + heap filter | 1.3 |
| btree_residual/regular/rare | by_flow_type (ordered scan) + heap filter | 0.9 |
| promoted_column/heavy/absent | wc_promoted_keyword3 (ordered scan) + heap filter | 0.0 |
| promoted_column/heavy/rare | wc_promoted_keyword3 (ordered scan) + heap filter | 0.3 |
| promoted_column/regular/absent | wc_promoted_keyword3 (ordered scan) + heap filter | 0.0 |
| promoted_column/regular/rare | wc_promoted_keyword3 (ordered scan) + heap filter | 0.1 |

### Under writers, first run (right after the mixed phases)

Flow rows and rare matches: heavy_flow_rows = 609093, heavy_rare_matches = 191, regular_flow_rows = 321, regular_rare_matches = 0.
Promoted index: 1473 MiB, built in 58.7 s.

| Variant / namespace / value | quiet p50 | quiet p99 | rows | fallbacks | under_16_writers p50 | under_16_writers p99 | rows | fallbacks |
|---|---:|---:|---:|---:|---:|---:|---:|---:|
| bm25/heavy/absent | 1502.2 | 1669.2 | 0 | — | 166.8 | 355.8 | 0 | — |
| bm25/heavy/rare | 1601.9 | 1665.7 | 50 | — | 216.2 | 347.1 | 50 | — |
| bm25/regular/absent | 1521.8 | 1686.5 | 0 | — | 85.0 | 143.7 | 0 | — |
| bm25/regular/rare | 1513.0 | 1623.2 | 0 | — | 149.6 | 2990.9 | 0 | — |
| bounded_1000/heavy/absent | 1541.4 | 1657.0 | 0 | 20 | 229.2 | 425.6 | 0 | 20 |
| bounded_1000/heavy/rare | 1594.7 | 1825.2 | 50 | 20 | 193.6 | 279.9 | 50 | 20 |
| bounded_1000/regular/absent | 1.4 | 2.2 | 0 | 0 | 2.7 | 5.6 | 0 | 0 |
| bounded_1000/regular/rare | 1.6 | 4.3 | 0 | 0 | 2.2 | 7.1 | 0 | 0 |
| bounded_10000/heavy/absent | 1624.6 | 1710.2 | 0 | 20 | 267.0 | 389.3 | 0 | 20 |
| bounded_10000/heavy/rare | 1633.2 | 1895.6 | 50 | 20 | 332.7 | 511.0 | 50 | 20 |
| bounded_10000/regular/absent | 1.4 | 2.0 | 0 | 0 | 2.4 | 9.7 | 0 | 0 |
| bounded_10000/regular/rare | 1.7 | 2.6 | 0 | 0 | 1.7 | 4.8 | 0 | 0 |
| btree_residual/heavy/absent | 1512.0 | 1630.4 | 0 | — | 2638.4 | 3013.5 | 0 | — |
| btree_residual/heavy/rare | 355.2 | 401.4 | 50 | — | 663.3 | 754.9 | 50 | — |
| btree_residual/regular/absent | 0.7 | 1.4 | 0 | — | 0.6 | 2.9 | 0 | — |
| btree_residual/regular/rare | 0.8 | 3.7 | 0 | — | 1.6 | 7.0 | 0 | — |
| promoted_column/heavy/absent | 0.3 | 0.3 | 0 | — | 0.1 | 1.0 | 0 | — |
| promoted_column/heavy/rare | 0.9 | 1.3 | 50 | — | 1.3 | 4.3 | 50 | — |
| promoted_column/regular/absent | 0.4 | 0.8 | 0 | — | 0.2 | 1.0 | 0 | — |
| promoted_column/regular/rare | 0.3 | 0.3 | 0 | — | 0.3 | 2.8 | 0 | — |

| Plan | Path | ms |
|---|---:|---:|
| bm25/heavy/absent | ParadeDB TopKScanExecState | 1636.8 |
| bm25/heavy/rare | ParadeDB TopKScanExecState | 1672.7 |
| bm25/regular/absent | ParadeDB TopKScanExecState | 1789.0 |
| bm25/regular/rare | ParadeDB TopKScanExecState | 1804.5 |
| btree_residual/heavy/absent | by_flow_type (ordered scan) + heap filter | 2377.4 |
| btree_residual/heavy/rare | by_flow_type (ordered scan) + heap filter | 831.5 |
| btree_residual/regular/absent | by_flow_type (ordered scan) + heap filter | 0.8 |
| btree_residual/regular/rare | by_flow_type (ordered scan) + heap filter | 3.0 |
| promoted_column/heavy/absent | wc_promoted_keyword3 (ordered scan) + heap filter | 0.0 |
| promoted_column/heavy/rare | wc_promoted_keyword3 (ordered scan) + heap filter | 1.5 |
| promoted_column/regular/absent | wc_promoted_keyword3 (ordered scan) + heap filter | 0.0 |
| promoted_column/regular/rare | wc_promoted_keyword3 (ordered scan) + heap filter | 0.1 |

### Under writers, rerun

Flow rows and rare matches: heavy_flow_rows = 714603, heavy_rare_matches = 224, regular_flow_rows = 329, regular_rare_matches = 0.
Promoted index: 1513 MiB, built in 60.5 s.

| Variant / namespace / value | quiet p50 | quiet p99 | rows | fallbacks | under_16_writers p50 | under_16_writers p99 | rows | fallbacks |
|---|---:|---:|---:|---:|---:|---:|---:|---:|
| bm25/heavy/absent | 1303.8 | 1355.2 | 0 | — | 209.1 | 313.4 | 0 | — |
| bm25/heavy/rare | 1375.0 | 1441.6 | 50 | — | 392.6 | 766.4 | 50 | — |
| bm25/regular/absent | 1258.5 | 1356.1 | 0 | — | 297.8 | 511.4 | 0 | — |
| bm25/regular/rare | 1279.9 | 1312.0 | 0 | — | 249.6 | 5055.0 | 0 | — |
| bounded_1000/heavy/absent | 1348.0 | 1420.0 | 0 | 20 | 131.5 | 238.1 | 0 | 20 |
| bounded_1000/heavy/rare | 1347.7 | 1415.7 | 50 | 20 | 283.9 | 372.9 | 50 | 20 |
| bounded_1000/regular/absent | 1.3 | 1.4 | 0 | 0 | 1.5 | 4.9 | 0 | 0 |
| bounded_1000/regular/rare | 1.4 | 2.9 | 0 | 0 | 2.3 | 7.3 | 0 | 0 |
| bounded_10000/heavy/absent | 1341.6 | 1568.4 | 0 | 20 | 440.9 | 517.5 | 0 | 20 |
| bounded_10000/heavy/rare | 1420.1 | 1535.3 | 50 | 20 | 358.6 | 440.5 | 50 | 20 |
| bounded_10000/regular/absent | 1.2 | 1.5 | 0 | 0 | 1.6 | 2.1 | 0 | 0 |
| bounded_10000/regular/rare | 1.4 | 2.6 | 0 | 0 | 1.7 | 2.4 | 0 | 0 |
| btree_residual/heavy/absent | 1572.8 | 1737.5 | 0 | — | 3044.3 | 3352.4 | 0 | — |
| btree_residual/heavy/rare | 243.1 | 282.4 | 50 | — | 436.3 | 545.1 | 50 | — |
| btree_residual/regular/absent | 0.8 | 0.9 | 0 | — | 0.9 | 3.3 | 0 | — |
| btree_residual/regular/rare | 0.8 | 2.5 | 0 | — | 1.6 | 4.1 | 0 | — |
| promoted_column/heavy/absent | 0.3 | 0.3 | 0 | — | 0.1 | 3.6 | 0 | — |
| promoted_column/heavy/rare | 1.0 | 1.3 | 50 | — | 1.6 | 3.3 | 50 | — |
| promoted_column/regular/absent | 0.3 | 0.3 | 0 | — | 0.3 | 0.9 | 0 | — |
| promoted_column/regular/rare | 0.3 | 1.2 | 0 | — | 0.8 | 4.1 | 0 | — |

| Plan | Path | ms |
|---|---:|---:|
| bm25/heavy/absent | ParadeDB TopKScanExecState | 1282.9 |
| bm25/heavy/rare | ParadeDB TopKScanExecState | 1399.7 |
| bm25/regular/absent | ParadeDB TopKScanExecState | 1448.4 |
| bm25/regular/rare | ParadeDB TopKScanExecState | 1633.6 |
| btree_residual/heavy/absent | by_flow_type (ordered scan) + heap filter | 1734.2 |
| btree_residual/heavy/rare | by_flow_type (ordered scan) + heap filter | 294.6 |
| btree_residual/regular/absent | by_flow_type (ordered scan) + heap filter | 0.8 |
| btree_residual/regular/rare | by_flow_type (ordered scan) + heap filter | 0.8 |
| promoted_column/heavy/absent | wc_promoted_keyword3 (ordered scan) + heap filter | 0.0 |
| promoted_column/heavy/rare | wc_promoted_keyword3 (ordered scan) + heap filter | 0.3 |
| promoted_column/regular/absent | wc_promoted_keyword3 (ordered scan) + heap filter | 0.1 |
| promoted_column/regular/rare | wc_promoted_keyword3 (ordered scan) + heap filter | 0.0 |


## Page-token stability

| Item | Value |
|---|---:|
| namespace | heavy |
| filter | flow_type = approval AND keyword1 = team-3 |
| set before / after | 30084 / 117195 |
| pages | 196 |
| paging seconds | 236.0 |
| stable rows | 6839 |
| stable duplicated | 0 |
| stable skipped | 0 |
| open to closed | 136 |
| open to closed, times seen | {"1": 136} |
| moved otherwise | 0 |
| deleted during paging | 23109 |
| inserted during paging (seen) | 110220 (358) |
| passed | True |

| Concurrent write op | count | /s | p99 ms |
|---|---:|---:|---:|
| close | 111556 | 472.6 | 12.8 |
| delete | 55995 | 237.2 | 11.8 |
| rewrite | 224808 | 952.4 | 11.1 |
| start | 112688 | 477.4 | 12.9 |


## Storage

| Snapshot | live rows | heap MiB | B-tree MiB | BM25 MiB | BM25 segments | tombstones | tombstone MiB |
|---|---:|---:|---:|---:|---:|---:|---:|
| sizes-before | 12001009 | 9231.5 | 5102.4 | 2901.0 | 4 | 0 | 0.0 |
| mix-no-c3-16-sizes | 12114248 | 9470.4 | 5403.8 | 2901.4 | 53 | 37261 | 8.5 |
| mix-c3-16-sizes | 12162023 | 9594.9 | 7076.8 | 2901.4 | 29 | 53726 | 12.1 |
| mix-c3-64-sizes | 12274930 | 9879.4 | 7451.5 | 2901.4 | 140 | 91316 | 20.4 |
| mix-no-c3-64-sizes | 12406302 | 10211.9 | 5939.5 | 2901.4 | 141 | 133848 | 30.5 |
| mix-c3-custom-16-sizes | 12615858 | 10687.2 | 7893.1 | 2901.6 | 41 | 203765 | 45.9 |
| mix-c3-custom-64-sizes | 12731188 | 10973.2 | 8251.1 | 2901.6 | 170 | 240870 | 54.4 |
| sizes-after | 12564699 | 10533.6 | 6182.9 | 2901.6 | 24 | 183682 | 41.2 |

| Index | MiB |
|---|---:|
| by_flow_type | 1484.7 |
| by_namespace_division | 1.6 |
| by_workflow_id | 776.6 |
| default_idx | 1412.9 |
| executions_visibility_pkey | 1426.6 |
| visibility_core_bm25 | 2901.0 |
