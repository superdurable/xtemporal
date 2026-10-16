# Visibility benchmark comparison

Bold numbers are best among completed variants for that row; ties are all marked.
Latency and errors: lower is better. Throughput: higher is better.

## 16 writers

### Writes

| Metric | postgres | pdb-cols | pdb-cols-hybrid | pdb-json-hybrid |
| --- | ---: | ---: | ---: | ---: |
| Writes/s | 693.81 | 1242.81 | 957.07 | **1889.56** |

### Write latency and errors

| Metric | postgres | pdb-cols | pdb-cols-hybrid | pdb-json-hybrid |
| --- | ---: | ---: | ---: | ---: |
| p50_ms | 14.86 | 11.33 | 14.48 | **6.15** |
| p95_ms | 72.18 | 27.35 | 36.24 | **22.04** |
| p99_ms | 136.76 | 38.76 | 57.75 | **31.05** |
| errors | **0** | **0** | **0** | **0** |

### Read: status_time

| Metric | postgres | pdb-cols | pdb-cols-hybrid | pdb-json-hybrid |
| --- | ---: | ---: | ---: | ---: |
| Reads/s | 2.79 | 1.13 | **4.84** | 3.62 |

### status_time latency and errors

| Metric | postgres | pdb-cols | pdb-cols-hybrid | pdb-json-hybrid |
| --- | ---: | ---: | ---: | ---: |
| p50_ms | **0.40** | 1309.44 | 0.51 | 1.26 |
| p95_ms | **4.99** | 3548.41 | 6.04 | 5.89 |
| p99_ms | **11.13** | 4313.28 | 12.80 | 11.20 |
| errors | **0** | **0** | **0** | **0** |

### Read: type_status

| Metric | postgres | pdb-cols | pdb-cols-hybrid | pdb-json-hybrid |
| --- | ---: | ---: | ---: | ---: |
| Reads/s | 2.79 | 1.13 | **4.85** | 3.62 |

### type_status latency and errors

| Metric | postgres | pdb-cols | pdb-cols-hybrid | pdb-json-hybrid |
| --- | ---: | ---: | ---: | ---: |
| p50_ms | **0.22** | 815.61 | 0.30 | 1.30 |
| p95_ms | **3.93** | 2002.24 | 5.93 | 10.26 |
| p99_ms | **8.14** | 3400.73 | 12.26 | 18.88 |
| errors | **0** | **0** | **0** | **0** |

### Read: text

| Metric | postgres | pdb-cols | pdb-cols-hybrid | pdb-json-hybrid |
| --- | ---: | ---: | ---: | ---: |
| Reads/s | 2.79 | 1.13 | **4.85** | 3.62 |

### text latency and errors

| Metric | postgres | pdb-cols | pdb-cols-hybrid | pdb-json-hybrid |
| --- | ---: | ---: | ---: | ---: |
| p50_ms | 741.70 | 828.63 | **157.21** | 316.12 |
| p95_ms | 1211.83 | 2033.61 | **741.74** | 1363.43 |
| p99_ms | 1375.89 | 3879.50 | **1279.83** | 1834.38 |
| errors | **0** | **0** | **0** | **0** |

### Read: keyword

| Metric | postgres | pdb-cols | pdb-cols-hybrid | pdb-json-hybrid |
| --- | ---: | ---: | ---: | ---: |
| Reads/s | 2.79 | 1.13 | **4.85** | 3.62 |

### keyword latency and errors

| Metric | postgres | pdb-cols | pdb-cols-hybrid | pdb-json-hybrid |
| --- | ---: | ---: | ---: | ---: |
| p50_ms | 674.43 | 869.12 | **134.64** | 289.22 |
| p95_ms | 1121.64 | 1922.27 | **676.60** | 1312.88 |
| p99_ms | 1283.21 | 2353.23 | **1118.12** | 1751.61 |
| errors | **0** | **0** | **0** | **0** |

### Read: keyword_list

| Metric | postgres | pdb-cols | pdb-cols-hybrid | pdb-json-hybrid |
| --- | ---: | ---: | ---: | ---: |
| Reads/s | 2.79 | 1.13 | **4.85** | 3.62 |

### keyword_list latency and errors

| Metric | postgres | pdb-cols | pdb-cols-hybrid | pdb-json-hybrid |
| --- | ---: | ---: | ---: | ---: |
| p50_ms | 713.28 | 868.93 | **122.09** | 263.74 |
| p95_ms | 1213.92 | 2021.54 | **665.34** | 1292.22 |
| p99_ms | 1396.88 | 2899.47 | **1155.53** | 1730.95 |
| errors | **0** | **0** | **0** | **0** |

### Read: count

| Metric | postgres | pdb-cols | pdb-cols-hybrid | pdb-json-hybrid |
| --- | ---: | ---: | ---: | ---: |
| Reads/s | 2.79 | 1.13 | **4.85** | 3.62 |

### count latency and errors

| Metric | postgres | pdb-cols | pdb-cols-hybrid | pdb-json-hybrid |
| --- | ---: | ---: | ---: | ---: |
| p50_ms | **134.31** | 948.59 | 197.53 | 186.09 |
| p95_ms | **294.70** | 2098.83 | 443.56 | 410.17 |
| p99_ms | **368.62** | 2435.26 | 564.77 | 544.59 |
| errors | **0** | **0** | **0** | **0** |

### Read: group_count

| Metric | postgres | pdb-cols | pdb-cols-hybrid | pdb-json-hybrid |
| --- | ---: | ---: | ---: | ---: |
| Reads/s | 2.79 | 1.13 | **4.84** | 3.62 |

### group_count latency and errors

| Metric | postgres | pdb-cols | pdb-cols-hybrid | pdb-json-hybrid |
| --- | ---: | ---: | ---: | ---: |
| p50_ms | 639.12 | 1209.59 | 768.67 | **572.03** |
| p95_ms | 953.56 | 2408.02 | 1158.48 | **888.43** |
| p99_ms | 1081.71 | 2745.76 | 1344.55 | **1044.18** |
| errors | **0** | **0** | **0** | **0** |

### Storage after level

| Metric | postgres | pdb-cols | pdb-cols-hybrid | pdb-json-hybrid |
| --- | ---: | ---: | ---: | ---: |
| Heap MiB | 650.88 | 659.55 | **616.20** | 663.69 |
| Index MiB | 17708.05 | 9313.55 | 9482.64 | **1981.89** |

### Query plans (before timing)

| Query | postgres | pdb-cols | pdb-cols-hybrid | pdb-json-hybrid |
| --- | --- | --- | --- | --- |
| status_time | B-tree index | ParadeDB Base | B-tree index | B-tree index |
| type_status | B-tree index | ParadeDB Base | B-tree index | B-tree index |
| text | B-tree index | ParadeDB Base | ParadeDB Base | ParadeDB Base |
| keyword | B-tree index | ParadeDB Base | ParadeDB Base | ParadeDB Base |
| keyword_list | B-tree index | ParadeDB Base | ParadeDB Base | ParadeDB Base |
| count | Bitmap heap/index | ParadeDB Aggregate | Bitmap heap/index | Bitmap heap/index |
| group_count | Bitmap heap/index | ParadeDB Aggregate | Bitmap heap/index | Heap scan/filter |

### Background load

- **postgres:** Other containers: 0.0% CPU across 0 containers. Host: `05:12:32 up 57 min,  0 user,  load average: 1.25, 1.87, 5.20`
- **pdb-cols:** Other containers: 0.0% CPU across 0 containers. Host: `05:50:20 up  1:34,  0 user,  load average: 1.48, 9.65, 19.43`
- **pdb-cols-hybrid:** Other containers: 0.0% CPU across 0 containers. Host: `06:29:25 up  2:14,  0 user,  load average: 1.28, 7.77, 18.83`
- **pdb-json-hybrid:** Other containers: 0.0% CPU across 0 containers. Host: `07:01:39 up  2:46,  0 user,  load average: 5.86, 23.99, 27.49`

## 64 writers

### Writes

| Metric | postgres | pdb-cols | pdb-cols-hybrid | pdb-json-hybrid |
| --- | ---: | ---: | ---: | ---: |
| Writes/s | 1082.43 | 1984.61 | 1570.92 | **4362.08** |

### Write latency and errors

| Metric | postgres | pdb-cols | pdb-cols-hybrid | pdb-json-hybrid |
| --- | ---: | ---: | ---: | ---: |
| p50_ms | 33.02 | 25.68 | 28.88 | **12.67** |
| p95_ms | 207.58 | 90.80 | 127.60 | **33.94** |
| p99_ms | 332.62 | 158.51 | 223.20 | **48.58** |
| errors | **0** | **0** | **0** | **0** |

### Read: status_time

| Metric | postgres | pdb-cols | pdb-cols-hybrid | pdb-json-hybrid |
| --- | ---: | ---: | ---: | ---: |
| Reads/s | 1.81 | 1.33 | **2.43** | 0.76 |

### status_time latency and errors

| Metric | postgres | pdb-cols | pdb-cols-hybrid | pdb-json-hybrid |
| --- | ---: | ---: | ---: | ---: |
| p50_ms | **0.41** | 409.95 | 0.53 | 1.41 |
| p95_ms | **5.30** | 3340.08 | 7.48 | 12.68 |
| p99_ms | **12.23** | 5401.89 | 21.06 | 26.47 |
| errors | **0** | **0** | **0** | **0** |

### Read: type_status

| Metric | postgres | pdb-cols | pdb-cols-hybrid | pdb-json-hybrid |
| --- | ---: | ---: | ---: | ---: |
| Reads/s | 1.81 | 1.33 | **2.43** | 0.76 |

### type_status latency and errors

| Metric | postgres | pdb-cols | pdb-cols-hybrid | pdb-json-hybrid |
| --- | ---: | ---: | ---: | ---: |
| p50_ms | **0.22** | 247.89 | 0.35 | 2.68 |
| p95_ms | **4.35** | 2753.20 | 9.46 | 20.42 |
| p99_ms | **10.80** | 5072.74 | 30.09 | 34.28 |
| errors | **0** | **0** | **0** | **0** |

### Read: text

| Metric | postgres | pdb-cols | pdb-cols-hybrid | pdb-json-hybrid |
| --- | ---: | ---: | ---: | ---: |
| Reads/s | 1.81 | 1.33 | **2.42** | 0.76 |

### text latency and errors

| Metric | postgres | pdb-cols | pdb-cols-hybrid | pdb-json-hybrid |
| --- | ---: | ---: | ---: | ---: |
| p50_ms | 1110.70 | **252.45** | 349.94 | 2734.36 |
| p95_ms | **1889.86** | 2818.46 | 1969.67 | 7360.73 |
| p99_ms | **2147.11** | 5142.17 | 3016.30 | 9914.22 |
| errors | **0** | **0** | **0** | **0** |

### Read: keyword

| Metric | postgres | pdb-cols | pdb-cols-hybrid | pdb-json-hybrid |
| --- | ---: | ---: | ---: | ---: |
| Reads/s | 1.81 | 1.33 | **2.42** | 0.75 |

### keyword latency and errors

| Metric | postgres | pdb-cols | pdb-cols-hybrid | pdb-json-hybrid |
| --- | ---: | ---: | ---: | ---: |
| p50_ms | 1009.47 | **227.41** | 279.83 | 2337.18 |
| p95_ms | **1768.51** | 2610.02 | 1847.79 | 6998.53 |
| p99_ms | **2068.80** | 4824.29 | 3135.05 | 10767.78 |
| errors | **0** | **0** | **0** | **0** |

### Read: keyword_list

| Metric | postgres | pdb-cols | pdb-cols-hybrid | pdb-json-hybrid |
| --- | ---: | ---: | ---: | ---: |
| Reads/s | 1.81 | 1.33 | **2.42** | 0.75 |

### keyword_list latency and errors

| Metric | postgres | pdb-cols | pdb-cols-hybrid | pdb-json-hybrid |
| --- | ---: | ---: | ---: | ---: |
| p50_ms | 1077.26 | **250.09** | 284.48 | 2391.08 |
| p95_ms | 1882.93 | 2825.40 | **1798.20** | 7039.63 |
| p99_ms | **2127.70** | 5374.43 | 3059.92 | 11666.02 |
| errors | **0** | **0** | **0** | **0** |

### Read: count

| Metric | postgres | pdb-cols | pdb-cols-hybrid | pdb-json-hybrid |
| --- | ---: | ---: | ---: | ---: |
| Reads/s | 1.81 | 1.33 | **2.42** | 0.75 |

### count latency and errors

| Metric | postgres | pdb-cols | pdb-cols-hybrid | pdb-json-hybrid |
| --- | ---: | ---: | ---: | ---: |
| p50_ms | **256.01** | 586.29 | 398.09 | 494.81 |
| p95_ms | **532.95** | 2920.93 | 772.63 | 1139.83 |
| p99_ms | **642.15** | 5431.59 | 927.53 | 1464.73 |
| errors | **0** | **0** | **0** | **0** |

### Read: group_count

| Metric | postgres | pdb-cols | pdb-cols-hybrid | pdb-json-hybrid |
| --- | ---: | ---: | ---: | ---: |
| Reads/s | 1.81 | 1.33 | **2.42** | 0.75 |

### group_count latency and errors

| Metric | postgres | pdb-cols | pdb-cols-hybrid | pdb-json-hybrid |
| --- | ---: | ---: | ---: | ---: |
| p50_ms | **984.20** | 1157.38 | 1232.67 | 1165.61 |
| p95_ms | **1553.86** | 3701.46 | 1780.93 | 1829.01 |
| p99_ms | **1864.40** | 5547.81 | 2075.13 | 2185.79 |
| errors | **0** | **0** | **0** | **0** |

### Storage after level

| Metric | postgres | pdb-cols | pdb-cols-hybrid | pdb-json-hybrid |
| --- | ---: | ---: | ---: | ---: |
| Heap MiB | 750.47 | 807.88 | **663.59** | 1008.43 |
| Index MiB | 20534.80 | 10649.61 | 10672.63 | **2783.80** |

### Query plans (before timing)

| Query | postgres | pdb-cols | pdb-cols-hybrid | pdb-json-hybrid |
| --- | --- | --- | --- | --- |
| status_time | B-tree index | ParadeDB Base | B-tree index | B-tree index |
| type_status | B-tree index | ParadeDB Base | B-tree index | B-tree index |
| text | B-tree index | ParadeDB Base | ParadeDB Base | ParadeDB Base |
| keyword | B-tree index | ParadeDB Base | ParadeDB Base | ParadeDB Base |
| keyword_list | B-tree index | ParadeDB Base | ParadeDB Base | ParadeDB Base |
| count | Bitmap heap/index | ParadeDB Aggregate | Bitmap heap/index | Bitmap heap/index |
| group_count | Bitmap heap/index | ParadeDB Aggregate | Bitmap heap/index | Heap scan/filter |

### Background load

- **postgres:** Other containers: 0.0% CPU across 0 containers. Host: `05:27:40 up  1:12,  0 user,  load average: 19.70, 19.98, 15.03`
- **pdb-cols:** Other containers: 0.0% CPU across 0 containers. Host: `06:05:28 up  1:50,  0 user,  load average: 15.67, 16.78, 18.11`
- **pdb-cols-hybrid:** Other containers: 0.0% CPU across 0 containers. Host: `06:44:34 up  2:29,  0 user,  load average: 20.92, 21.72, 21.15`
- **pdb-json-hybrid:** Other containers: 0.0% CPU across 0 containers. Host: `07:16:47 up  3:01,  0 user,  load average: 17.29, 18.49, 22.22`
