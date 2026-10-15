#!/usr/bin/env bash
set -euo pipefail

bench_dir=$(cd "$(dirname "$0")" && pwd)
compose=(docker compose -f "$bench_dir/diagnose.yml" -p xtpdbcrash)
output=${1:?usage: diagnose.sh OUTPUT_DIRECTORY}
mkdir -p "$output"
output=$(cd "$output" && pwd)
rows=${DIAG_ROWS:-1000000}
writers=${DIAG_WRITERS:-16}
readers=${DIAG_READERS:-8}
duration=${DIAG_DURATION:-2m}
tag=${PARADEDB_TAG:-v0.26.0-pg17}
label=${DIAG_LABEL:-${tag}-w${writers}-r${readers}}
container=xtpdbcrash-paradedb-1
dsn='postgres://postgres:spike@127.0.0.1:57434/visibility?sslmode=disable'

(cd "$bench_dir" && go build -o "$output/bench" .)
if [[ ${DIAG_REUSE:-0} != 1 ]]; then
  "${compose[@]}" down -v --remove-orphans
  PARADEDB_TAG=$tag "${compose[@]}" up -d --wait
  until docker logs "$container" > "$output/$label-init.log" 2>&1 && grep -q 'PostgreSQL init process complete' "$output/$label-init.log"; do sleep 2; done
  PARADEDB_TAG=$tag "${compose[@]}" up -d --wait
  docker exec -i "$container" psql -v ON_ERROR_STOP=1 -U postgres -d visibility < "$bench_dir/../spike/paradedb-schema.sql" > "$output/$label-schema.log"
  "$output/bench" -mode preload -rows "$rows" -dsn "$dsn" > "$output/$label-preload.log" 2>&1
fi

docker exec "$container" psql -v ON_ERROR_STOP=1 -U postgres -d visibility -c "ALTER SYSTEM SET log_statement = 'all'" > "$output/$label-logging.log"
docker exec "$container" psql -v ON_ERROR_STOP=1 -U postgres -d visibility -c 'SELECT pg_reload_conf()' >> "$output/$label-logging.log"
docker exec "$container" psql -At -U postgres -d visibility -c 'SELECT count(*) FROM executions_visibility' > "$output/$label-rows.txt"
docker exec "$container" psql -At -U postgres -d visibility -c "SELECT extversion FROM pg_extension WHERE extname='pg_search'" > "$output/$label-extension.txt"
start=$(date -u +%Y-%m-%dT%H:%M:%SZ)
set +e
"$output/bench" -mode run -rows "$rows" -writers "$writers" -readers "$readers" -duration "$duration" -write-op "${DIAG_WRITE_OP:-mixed}" -read-query "${DIAG_READ_QUERY:-all}" -dsn "$dsn" > "$output/$label.json" 2> "$output/$label.stderr"
status=$?
set -e
docker logs --since "$start" "$container" > "$output/$label-postgres.log" 2>&1
docker inspect --format '{{json .State}}' "$container" > "$output/$label-container.json"
docker exec "$container" psql -At -U postgres -d visibility -c "SELECT * FROM paradedb.index_info('visibility_core_bm25')" > "$output/$label-segments.txt" || true
docker exec "$container" psql -At -U postgres -d visibility -c "SELECT * FROM paradedb.merge_info('visibility_core_bm25')" > "$output/$label-merges.txt" || true
echo "workload exit status: $status; logs: $output/$label-postgres.log"
exit "$status"
