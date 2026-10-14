#!/usr/bin/env bash
set -euo pipefail

bench_dir=$(cd "$(dirname "$0")" && pwd)
compose=(docker compose -f "$bench_dir/docker-compose.yml" -p xtpdbbench)
output=${1:?usage: crash.sh OUTPUT_DIRECTORY (with one million preloaded ParadeDB rows)}
mkdir -p "$output"
output=$(cd "$output" && pwd)
(cd "$bench_dir" && go build -o "$output/bench" .)

cleanup() {
  if [[ $1 == 0 && ${BENCH_KEEP_CONTAINERS:-0} != 1 ]]; then
    "${compose[@]}" down -v
  elif [[ $1 != 0 ]]; then
    echo 'Crash check failed; retaining containers and volumes for diagnosis' >&2
  fi
}
trap 'cleanup $?' EXIT

rows=$(docker exec xtpdbbench-paradedb-1 psql -At -U postgres -d visibility -c 'SELECT count(*) FROM executions_visibility')
if [[ $rows != 1000000 ]]; then echo "expected one million rows, found $rows" >&2; exit 1; fi

crash_start=$(date -u +%Y-%m-%dT%H:%M:%SZ)
"$output/bench" -mode run -allow-errors -rows 1000000 -writers 16 -readers 8 -duration 2m -paradedb=true -dsn 'postgres://postgres:spike@127.0.0.1:56432/visibility?sslmode=disable' > "$output/paradedb-crash-load.json" &
load_pid=$!
sleep 10
if ! kill -0 "$load_pid" 2>/dev/null || ! docker exec xtpdbbench-paradedb-1 pg_isready -U postgres -d visibility >/dev/null 2>&1; then
  echo 'ParadeDB stopped before the deliberate kill' >&2
  exit 1
fi
docker kill --signal KILL xtpdbbench-paradedb-1
wait "$load_pid"
docker start xtpdbbench-paradedb-1
ready=0
for ((attempt=0; attempt<900; attempt++)); do
  if docker exec xtpdbbench-paradedb-1 pg_isready -U postgres -d visibility >/dev/null 2>&1; then ready=1; break; fi
  sleep 2
done
if [[ $ready != 1 ]]; then echo 'ParadeDB did not recover within 30 minutes' >&2; exit 1; fi
ready_time=$(date -u +%Y-%m-%dT%H:%M:%SZ)
"$output/bench" -mode verify -dsn 'postgres://postgres:spike@127.0.0.1:56432/visibility?sslmode=disable' > "$output/paradedb-crash-verify.json"
docker logs --since "$crash_start" xtpdbbench-paradedb-1 > "$output/paradedb-crash-recovery.log" 2>&1
docker logs --since "$ready_time" xtpdbbench-paradedb-1 > "$output/paradedb-crash-post-ready.log" 2>&1
if grep -En 'ERROR:|PANIC:|FATAL:' "$output/paradedb-crash-post-ready.log"; then
  echo 'ParadeDB logged an error after recovery' >&2
  exit 1
fi
