#!/usr/bin/env bash
set -euo pipefail

bench_dir=$(cd "$(dirname "$0")" && pwd)
repo_root=$(cd "$bench_dir/../../../.." && pwd)
compose=(docker compose -f "$bench_dir/docker-compose.yml" -p xtpdbbench)
output=${1:-"$bench_dir/results"}
rows=${BENCH_ROWS:-1000000}
duration=${BENCH_DURATION:-15m}
connections=${BENCH_CONNECTIONS:-"16 64 128"}
mkdir -p "$output"
output=$(cd "$output" && pwd)

cleanup() {
  if [[ $1 == 0 && ${BENCH_KEEP_CONTAINERS:-0} != 1 ]]; then
    "${compose[@]}" down -v
  elif [[ $1 != 0 ]]; then
    echo 'Benchmark failed; retaining containers and volumes for diagnosis' >&2
  fi
}
trap 'cleanup $?' EXIT

(cd "$bench_dir" && go build -o "$output/bench" .)
if [[ ${BENCH_SKIP_SETUP:-0} != 1 ]]; then
  "${compose[@]}" up -d --wait
  until docker logs xtpdbbench-paradedb-1 2>&1 | grep -q 'PostgreSQL init process complete'; do sleep 2; done
  "${compose[@]}" up -d --wait
  docker exec -i xtpdbbench-paradedb-1 psql -v ON_ERROR_STOP=1 -U postgres -d visibility < "$bench_dir/../spike/paradedb-schema.sql" > "$output/schema-paradedb.log"
  docker exec -i xtpdbbench-postgres-1 psql -v ON_ERROR_STOP=1 -U postgres -d visibility < "$repo_root/schema/postgresql/v12/visibility/schema.sql" > "$output/schema-postgres.log"
  for database in ${BENCH_DATABASES:-postgres paradedb}; do
    port=56433
    if [[ $database == paradedb ]]; then port=56432; fi
    "$output/bench" -mode preload -rows "$rows" -dsn "postgres://postgres:spike@127.0.0.1:$port/visibility?sslmode=disable" > "$output/preload-$database.log" 2>&1
  done
fi

snapshot() {
  local database=$1 label=$2
  local container="xtpdbbench-$database-1"
  docker exec "$container" psql -At -U postgres -d visibility -c "SELECT now(), count(*), pg_size_pretty(pg_relation_size('executions_visibility')), pg_size_pretty(pg_indexes_size('executions_visibility')) FROM executions_visibility" > "$output/$database-$label-size.txt"
  docker exec "$container" psql -At -U postgres -d visibility -c "SELECT indexrelname, pg_relation_size(indexrelid) FROM pg_stat_user_indexes WHERE relname='executions_visibility' ORDER BY 1" > "$output/$database-$label-indexes.txt"
  docker exec "$container" psql -At -U postgres -d visibility -c "SELECT relname, n_tup_ins, n_tup_upd, n_tup_del, n_dead_tup, autovacuum_count, last_autovacuum FROM pg_stat_user_tables WHERE relname='executions_visibility'" > "$output/$database-$label-vacuum.txt"
  if [[ $database == paradedb ]]; then
    docker exec "$container" psql -At -U postgres -d visibility -c "SELECT extversion FROM pg_extension WHERE extname='pg_search'" > "$output/paradedb-extension.txt"
    docker exec "$container" psql -At -U postgres -d visibility -c "SELECT * FROM paradedb.index_info('visibility_core_bm25')" > "$output/paradedb-$label-segments.txt"
    docker exec "$container" psql -At -U postgres -d visibility -c "SELECT * FROM paradedb.merge_info('visibility_core_bm25')" > "$output/paradedb-$label-merges.txt"
  fi
}

for database in ${BENCH_DATABASES:-postgres paradedb}; do
  if [[ ! -f $output/$database-before-size.txt ]]; then snapshot "$database" before; fi
  port=56433
  parade=false
  if [[ $database == paradedb ]]; then port=56432; parade=true; fi
  database_connections=$connections
  if [[ $database == postgres ]]; then database_connections=${BENCH_POSTGRES_CONNECTIONS:-$connections}; fi
  if [[ $database == paradedb ]]; then database_connections=${BENCH_PARADEDB_CONNECTIONS:-$connections}; fi
  for writers in $database_connections; do
    : > "$output/$database-$writers-docker-stats.csv"
    "$output/bench" -mode run -rows "$rows" -writers "$writers" -readers 8 -duration "$duration" -paradedb="$parade" -dsn "postgres://postgres:spike@127.0.0.1:$port/visibility?sslmode=disable" > "$output/$database-$writers.json" &
    bench_pid=$!
    while kill -0 "$bench_pid" 2>/dev/null; do
      docker stats --no-stream --format '{{.CPUPerc}},{{.MemUsage}}' "xtpdbbench-$database-1" >> "$output/$database-$writers-docker-stats.csv"
      sleep 10
    done
    wait "$bench_pid"
    snapshot "$database" "after-$writers"
  done
done

if [[ ${BENCH_RUN_CRASH:-1} != 1 ]]; then exit 0; fi

crash_start=$(date -u +%Y-%m-%dT%H:%M:%SZ)
"$output/bench" -mode run -allow-errors -rows "$rows" -writers 64 -readers 8 -duration 2m -paradedb=true -dsn 'postgres://postgres:spike@127.0.0.1:56432/visibility?sslmode=disable' > "$output/paradedb-crash-load.json" &
crash_pid=$!
sleep 30
docker kill --signal KILL xtpdbbench-paradedb-1
docker start xtpdbbench-paradedb-1
ready=0
for ((attempt=0; attempt<900; attempt++)); do
  if docker exec xtpdbbench-paradedb-1 pg_isready -U postgres -d visibility >/dev/null 2>&1; then ready=1; break; fi
  sleep 2
done
if [[ $ready != 1 ]]; then echo 'ParadeDB did not recover within 30 minutes' >&2; exit 1; fi
wait "$crash_pid"
"$output/bench" -mode verify -dsn 'postgres://postgres:spike@127.0.0.1:56432/visibility?sslmode=disable' > "$output/paradedb-crash-verify.json"
docker logs --since "$crash_start" xtpdbbench-paradedb-1 > "$output/paradedb-after-crash.log" 2>&1
snapshot paradedb after-crash
