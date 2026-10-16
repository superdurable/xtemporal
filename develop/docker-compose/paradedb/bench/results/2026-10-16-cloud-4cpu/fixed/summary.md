# Visibility benchmark comparison

Numbers are medians across completed rounds; rounds per variant: postgres=2, pdb-cols=2, pdb-cols-hybrid=2, pdb-json-hybrid=2.
Bold numbers are best among completed variants for that row; ties are all marked.
Latency, errors and missed reads: lower is better. Throughput: higher is better.

## Read capacity (read-only, closed loop, one query at a time)

### Reads/s

| Metric | postgres | pdb-cols | pdb-cols-hybrid | pdb-json-hybrid |
| --- | ---: | ---: | ---: | ---: |
| status_time | 16554.32 | 118.51 | **19633.75** | 5287.66 |
| type_status | 16383.75 | 126.16 | **19812.71** | 5288.60 |
| text | **7067.35** | 123.71 | 126.80 | 112.61 |
| keyword | **2671.26** | 129.16 | 123.32 | 107.67 |
| keyword_list | **4421.48** | 136.46 | 123.45 | 107.86 |
| count | **418.87** | 98.26 | 185.82 | 307.25 |
| group_count | **66.65** | 43.39 | 17.67 | 45.14 |

### Read p50_ms

| Metric | postgres | pdb-cols | pdb-cols-hybrid | pdb-json-hybrid |
| --- | ---: | ---: | ---: | ---: |
| status_time | 0.41 | 63.47 | **0.35** | 1.35 |
| type_status | 0.41 | 60.56 | **0.35** | 1.35 |
| text | **0.98** | 61.73 | 59.92 | 67.61 |
| keyword | **2.76** | 59.83 | 61.97 | 70.59 |
| keyword_list | **1.63** | 56.97 | 61.40 | 70.98 |
| count | **18.71** | 78.61 | 42.30 | 30.06 |
| group_count | **118.13** | 180.69 | 448.10 | 281.46 |

### Read p99_ms

| Metric | postgres | pdb-cols | pdb-cols-hybrid | pdb-json-hybrid |
| --- | ---: | ---: | ---: | ---: |
| status_time | 1.73 | 121.63 | **1.35** | 4.27 |
| type_status | 1.78 | 114.67 | **1.37** | 4.21 |
| text | **3.53** | 115.70 | 113.19 | 123.76 |
| keyword | **7.71** | 107.76 | 112.66 | 126.92 |
| keyword_list | **5.02** | 99.14 | 116.15 | 126.74 |
| count | **35.91** | 132.72 | 75.61 | 54.86 |
| group_count | **174.93** | 271.35 | 626.35 | 400.86 |

## 16 writers

Readers offer a fixed 2 reads/s per query (14/s in total), so every variant faces the same read pressure. Missed reads mean the variant could not keep up.

### Writes

| Metric | postgres | pdb-cols | pdb-cols-hybrid | pdb-json-hybrid |
| --- | ---: | ---: | ---: | ---: |
| Writes/s | 618.20 | 1859.31 | 1718.54 | **3409.47** |
| Writes/s per round | 595 / 641 | 1749 / 1969 | 1829 / 1608 | 3745 / 3074 |

### Write latency and errors

| Metric | postgres | pdb-cols | pdb-cols-hybrid | pdb-json-hybrid |
| --- | ---: | ---: | ---: | ---: |
| p50_ms | 14.46 | 7.21 | 8.02 | **3.51** |
| p95_ms | 86.18 | 19.39 | 19.87 | **12.43** |
| p99_ms | 170.08 | 29.01 | 29.28 | **19.14** |
| errors | **0.00** | **0.00** | **0.00** | **0.00** |

### Reads/s served

| Metric | postgres | pdb-cols | pdb-cols-hybrid | pdb-json-hybrid |
| --- | ---: | ---: | ---: | ---: |
| status_time | **2.00** | 1.98 | 2.00 | 2.00 |
| type_status | **2.00** | 1.97 | 2.00 | 2.00 |
| text | 2.00 | 1.97 | **2.00** | 2.00 |
| keyword | **2.00** | 1.98 | 2.00 | 1.99 |
| keyword_list | **2.00** | 1.97 | 2.00 | 2.00 |
| count | **2.00** | 1.97 | 2.00 | 1.99 |
| group_count | 2.00 | 1.98 | **2.00** | 1.99 |

### Read p50_ms

| Metric | postgres | pdb-cols | pdb-cols-hybrid | pdb-json-hybrid |
| --- | ---: | ---: | ---: | ---: |
| status_time | **1.77** | 179.10 | 3.28 | 3.31 |
| type_status | **1.69** | 162.05 | 2.87 | 3.23 |
| text | **2.75** | 160.56 | 159.41 | 210.12 |
| keyword | **5.62** | 144.45 | 137.13 | 196.43 |
| keyword_list | **3.55** | 146.34 | 137.78 | 198.93 |
| count | **51.49** | 177.82 | 126.11 | 161.39 |
| group_count | **225.47** | 284.33 | 376.87 | 293.45 |

### Read p95_ms

| Metric | postgres | pdb-cols | pdb-cols-hybrid | pdb-json-hybrid |
| --- | ---: | ---: | ---: | ---: |
| status_time | **7.93** | 577.12 | 11.92 | 11.25 |
| type_status | **7.58** | 501.68 | 11.68 | 11.78 |
| text | **9.86** | 503.51 | 463.06 | 612.90 |
| keyword | **18.45** | 477.20 | 438.13 | 569.20 |
| keyword_list | **12.21** | 482.39 | 433.35 | 591.69 |
| count | **151.65** | 499.34 | 226.21 | 298.63 |
| group_count | 451.01 | 733.09 | 498.14 | **423.17** |

### Read p99_ms

| Metric | postgres | pdb-cols | pdb-cols-hybrid | pdb-json-hybrid |
| --- | ---: | ---: | ---: | ---: |
| status_time | **13.05** | 1133.64 | 19.07 | 16.29 |
| type_status | **13.71** | 921.13 | 17.74 | 17.01 |
| text | **17.04** | 947.61 | 910.26 | 1175.05 |
| keyword | **28.20** | 966.67 | 872.11 | 1203.41 |
| keyword_list | **18.38** | 924.10 | 805.33 | 1165.82 |
| count | **198.15** | 913.93 | 277.11 | 358.46 |
| group_count | 574.33 | 1287.26 | 616.64 | **534.16** |

### Read missed

| Metric | postgres | pdb-cols | pdb-cols-hybrid | pdb-json-hybrid |
| --- | ---: | ---: | ---: | ---: |
| status_time | **0.00** | 13.00 | 1.50 | 0.50 |
| type_status | **0.00** | 15.50 | 0.50 | 0.50 |
| text | **0.00** | 14.00 | **0.00** | 1.00 |
| keyword | **0.00** | 13.00 | 0.50 | 1.50 |
| keyword_list | **0.00** | 18.00 | 0.50 | 1.50 |
| count | **0.00** | 17.00 | 0.50 | 2.50 |
| group_count | **0.00** | 13.00 | **0.00** | 2.00 |

### Read errors

| Metric | postgres | pdb-cols | pdb-cols-hybrid | pdb-json-hybrid |
| --- | ---: | ---: | ---: | ---: |
| status_time | **0.00** | **0.00** | **0.00** | **0.00** |
| type_status | **0.00** | **0.00** | **0.00** | **0.00** |
| text | **0.00** | **0.00** | **0.00** | **0.00** |
| keyword | **0.00** | **0.00** | **0.00** | **0.00** |
| keyword_list | **0.00** | **0.00** | **0.00** | **0.00** |
| count | **0.00** | **0.00** | **0.00** | **0.00** |
| group_count | **0.00** | **0.00** | **0.00** | **0.00** |

### Storage after level

| Metric | postgres | pdb-cols | pdb-cols-hybrid | pdb-json-hybrid |
| --- | ---: | ---: | ---: | ---: |
| Heap MiB | **627.13** | 657.02 | 642.73 | 672.36 |
| Index MiB | 16870.00 | 9103.32 | 10021.29 | **2003.73** |

### Background load

- **postgres r1:** Other containers: 0.0% CPU across 0 containers. Host: `08:10:08 up  3:54,  0 user,  load average: 8.66, 7.77, 7.42`
- **postgres r2:** Other containers: 0.0% CPU across 0 containers. Host: `13:02:48 up  1:31,  0 user,  load average: 8.73, 7.95, 10.24`
- **pdb-cols r1:** Other containers: 0.0% CPU across 0 containers. Host: `08:45:55 up  4:30,  0 user,  load average: 8.10, 7.84, 11.06`
- **pdb-cols r2:** Other containers: 0.0% CPU across 0 containers. Host: `10:51:51 up  1:20,  0 user,  load average: 8.14, 8.82, 15.07`
- **pdb-cols-hybrid r1:** Other containers: 0.0% CPU across 0 containers. Host: `09:48:21 up 16 min,  0 user,  load average: 15.05, 8.76, 4.30`
- **pdb-cols-hybrid r2:** Other containers: 0.0% CPU across 0 containers. Host: `11:48:40 up 17 min,  0 user,  load average: 14.35, 8.85, 4.39`
- **pdb-json-hybrid r1:** Other containers: 0.0% CPU across 0 containers. Host: `10:17:37 up 46 min,  0 user,  load average: 15.19, 14.67, 17.37`
- **pdb-json-hybrid r2:** Other containers: 0.0% CPU across 0 containers. Host: `12:18:12 up 47 min,  0 user,  load average: 7.80, 12.30, 16.52`

## 64 writers

Readers offer a fixed 2 reads/s per query (14/s in total), so every variant faces the same read pressure. Missed reads mean the variant could not keep up.

### Writes

| Metric | postgres | pdb-cols | pdb-cols-hybrid | pdb-json-hybrid |
| --- | ---: | ---: | ---: | ---: |
| Writes/s | 1076.03 | 2402.84 | 2055.07 | **4958.04** |
| Writes/s per round | 1052 / 1100 | 2229 / 2576 | 2205 / 1905 | 5189 / 4727 |

### Write latency and errors

| Metric | postgres | pdb-cols | pdb-cols-hybrid | pdb-json-hybrid |
| --- | ---: | ---: | ---: | ---: |
| p50_ms | 36.24 | 21.79 | 24.02 | **10.81** |
| p95_ms | 203.22 | 72.96 | 92.56 | **32.24** |
| p99_ms | 315.54 | 133.16 | 164.02 | **46.77** |
| errors | **0.00** | **0.00** | **0.00** | **0.00** |

### Reads/s served

| Metric | postgres | pdb-cols | pdb-cols-hybrid | pdb-json-hybrid |
| --- | ---: | ---: | ---: | ---: |
| status_time | **2.00** | 1.38 | 1.57 | 1.29 |
| type_status | **2.00** | 1.38 | 1.54 | 1.24 |
| text | **2.00** | 1.36 | 1.51 | 1.19 |
| keyword | **2.00** | 1.36 | 1.56 | 1.06 |
| keyword_list | **2.00** | 1.45 | 1.81 | 0.90 |
| count | **2.00** | 1.11 | 1.59 | 0.97 |
| group_count | **2.00** | 1.19 | 1.61 | 1.18 |

### Read p50_ms

| Metric | postgres | pdb-cols | pdb-cols-hybrid | pdb-json-hybrid |
| --- | ---: | ---: | ---: | ---: |
| status_time | **2.43** | 428.21 | 2.92 | 3.99 |
| type_status | **2.70** | 389.74 | 2.86 | 3.65 |
| text | **4.05** | 411.95 | 399.50 | 1165.30 |
| keyword | **9.41** | 393.80 | 401.25 | 1097.65 |
| keyword_list | **4.93** | 456.03 | 533.39 | 905.25 |
| count | **122.38** | 416.72 | 369.21 | 466.23 |
| group_count | **378.22** | 877.59 | 1029.69 | 1006.25 |

### Read p95_ms

| Metric | postgres | pdb-cols | pdb-cols-hybrid | pdb-json-hybrid |
| --- | ---: | ---: | ---: | ---: |
| status_time | **16.14** | 2099.18 | 20.41 | 26.75 |
| type_status | **17.37** | 2068.61 | 19.21 | 21.47 |
| text | **24.83** | 2223.84 | 2283.39 | 6581.65 |
| keyword | **57.71** | 1879.70 | 2540.99 | 7624.98 |
| keyword_list | **32.12** | 2075.62 | 3303.15 | 4570.45 |
| count | **390.21** | 1807.98 | 728.05 | 1014.46 |
| group_count | **1056.76** | 2665.33 | 1604.87 | 1664.33 |

### Read p99_ms

| Metric | postgres | pdb-cols | pdb-cols-hybrid | pdb-json-hybrid |
| --- | ---: | ---: | ---: | ---: |
| status_time | **32.78** | 3843.49 | 45.57 | 42.97 |
| type_status | **38.31** | 3294.93 | 41.79 | 40.82 |
| text | **62.13** | 4171.25 | 3587.19 | 11781.89 |
| keyword | **101.34** | 3275.39 | 4197.43 | 10923.35 |
| keyword_list | **68.80** | 3335.71 | 5101.23 | 10875.11 |
| count | **551.75** | 3395.54 | 877.97 | 1280.70 |
| group_count | **1384.31** | 3821.74 | 1801.25 | 1951.14 |

### Read missed

| Metric | postgres | pdb-cols | pdb-cols-hybrid | pdb-json-hybrid |
| --- | ---: | ---: | ---: | ---: |
| status_time | **0.00** | 364.50 | 253.50 | 427.00 |
| type_status | **0.00** | 370.00 | 272.00 | 454.00 |
| text | **0.00** | 383.00 | 294.00 | 484.00 |
| keyword | **0.00** | 380.50 | 260.00 | 561.00 |
| keyword_list | **0.00** | 329.00 | 109.00 | 660.00 |
| count | **0.00** | 529.50 | 246.00 | 614.50 |
| group_count | **0.00** | 483.00 | 231.50 | 489.50 |

### Read errors

| Metric | postgres | pdb-cols | pdb-cols-hybrid | pdb-json-hybrid |
| --- | ---: | ---: | ---: | ---: |
| status_time | **0.00** | **0.00** | **0.00** | **0.00** |
| type_status | **0.00** | **0.00** | **0.00** | **0.00** |
| text | **0.00** | **0.00** | **0.00** | **0.00** |
| keyword | **0.00** | **0.00** | **0.00** | **0.00** |
| keyword_list | **0.00** | **0.00** | **0.00** | **0.00** |
| count | **0.00** | **0.00** | **0.00** | **0.00** |
| group_count | **0.00** | **0.00** | **0.00** | **0.00** |

### Storage after level

| Metric | postgres | pdb-cols | pdb-cols-hybrid | pdb-json-hybrid |
| --- | ---: | ---: | ---: | ---: |
| Heap MiB | 694.45 | 692.45 | **685.96** | 813.04 |
| Index MiB | 18239.55 | 9421.58 | 10262.74 | **2364.02** |

### Background load

- **postgres r1:** Other containers: 0.0% CPU across 0 containers. Host: `08:20:21 up  4:04,  0 user,  load average: 9.29, 10.78, 9.40`
- **postgres r2:** Other containers: 0.0% CPU across 0 containers. Host: `13:13:01 up  1:41,  0 user,  load average: 10.48, 11.29, 10.93`
- **pdb-cols r1:** Other containers: 0.0% CPU across 0 containers. Host: `08:56:07 up  4:40,  0 user,  load average: 10.53, 11.91, 12.11`
- **pdb-cols r2:** Other containers: 0.0% CPU across 0 containers. Host: `11:02:02 up  1:30,  0 user,  load average: 11.44, 13.70, 14.73`
- **pdb-cols-hybrid r1:** Other containers: 0.0% CPU across 0 containers. Host: `09:58:31 up 27 min,  0 user,  load average: 13.07, 12.77, 8.55`
- **pdb-cols-hybrid r2:** Other containers: 0.0% CPU across 0 containers. Host: `11:58:51 up 27 min,  0 user,  load average: 13.97, 13.52, 9.00`
- **pdb-json-hybrid r1:** Other containers: 0.0% CPU across 0 containers. Host: `10:27:48 up 56 min,  0 user,  load average: 14.11, 14.56, 15.94`
- **pdb-json-hybrid r2:** Other containers: 0.0% CPU across 0 containers. Host: `12:28:23 up 57 min,  0 user,  load average: 11.91, 13.54, 15.45`

## Query plans (before timing)

| Query | postgres | pdb-cols | pdb-cols-hybrid | pdb-json-hybrid |
| --- | --- | --- | --- | --- |
| status_time | B-tree index | ParadeDB Base | B-tree index | B-tree index |
| type_status | B-tree index | ParadeDB Base | B-tree index | B-tree index |
| text | B-tree index | ParadeDB Base | ParadeDB Base | ParadeDB Base |
| keyword | B-tree index | ParadeDB Base | ParadeDB Base | ParadeDB Base |
| keyword_list | B-tree index | ParadeDB Base | ParadeDB Base | ParadeDB Base |
| count | B-tree index only | ParadeDB Aggregate | Bitmap heap/index | Bitmap heap/index |
| group_count | B-tree index only | ParadeDB Aggregate | Bitmap heap/index | Heap scan/filter |
