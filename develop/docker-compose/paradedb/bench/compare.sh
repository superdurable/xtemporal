#!/usr/bin/env bash
set -euo pipefail

bench_dir=$(cd "$(dirname "$0")" && pwd)
repo_root=$(cd "$bench_dir/../../../.." && pwd)
output=${1:?usage: compare.sh <output-dir>}
mkdir -p "$output"
output=$(cd "$output" && pwd)
project=${BENCH_PROJECT:-xtpdbcmp$(printf '%s' "$output" | cksum | awk '{print $1}')}
export BENCH_PORT=${BENCH_PORT:-56640}
compose=(docker compose -f "$bench_dir/compare-compose.yml" -p "$project")
rows=${BENCH_ROWS:-1000000}
duration=${BENCH_DURATION:-15m}
connections=${BENCH_CONNECTIONS:-"16 64"}
readers=${BENCH_READERS:-8}
variants=${BENCH_VARIANTS:-"postgres pdb-cols pdb-cols-hybrid pdb-json-hybrid"}
container="$project-db-1"
dsn="postgres://postgres:spike@127.0.0.1:$BENCH_PORT/visibility?sslmode=disable"

cleanup() {
  "${compose[@]}" down -v >/dev/null 2>&1 || true
}
trap cleanup EXIT
(cd "$bench_dir" && go build -o "$output/bench" .)

snapshot() {
  local dir=$1 label=$2 variant=$3
  docker exec "$container" psql -v ON_ERROR_STOP=1 -At -U postgres -d visibility -c "SELECT pg_relation_size('executions_visibility'), pg_indexes_size('executions_visibility')" > "$dir/$label-size.txt"
  docker exec "$container" psql -v ON_ERROR_STOP=1 -At -U postgres -d visibility -c "SELECT indexrelname, pg_relation_size(indexrelid) FROM pg_stat_user_indexes WHERE relname='executions_visibility' ORDER BY 1" > "$dir/$label-indexes.txt"
  docker exec "$container" psql -v ON_ERROR_STOP=1 -At -U postgres -d visibility -c "SELECT n_tup_ins, n_tup_upd, n_tup_del, n_dead_tup, autovacuum_count FROM pg_stat_user_tables WHERE relname='executions_visibility'" > "$dir/$label-vacuum.txt"
  if [[ $variant != postgres ]]; then
    docker exec "$container" psql -v ON_ERROR_STOP=1 -At -U postgres -d visibility -c "SELECT * FROM paradedb.index_info('visibility_core_bm25')" > "$dir/$label-segments.txt"
    docker exec "$container" psql -v ON_ERROR_STOP=1 -At -U postgres -d visibility -c "SELECT * FROM paradedb.merge_info('visibility_core_bm25')" > "$dir/$label-merges.txt"
  fi
}

capture_background() {
  local dir=$1 writers=$2
  docker stats --no-stream --format '{{.Name}},{{.CPUPerc}},{{.MemUsage}}' > "$dir/$writers-all-containers.csv"
  uptime > "$dir/$writers-host-load.txt"
  python3 - "$dir/$writers-all-containers.csv" "$container" "$dir/$writers-background.txt" <<'PY'
import sys
rows = [line.strip().split(',', 2) for line in open(sys.argv[1]) if line.strip()]
cpu = sum(float(row[1].rstrip('%')) for row in rows if row[0] != sys.argv[2] and len(row) >= 2)
note = f'Other containers: {cpu:.1f}% CPU across {len(rows)-1} containers.'
if cpu > 100:
    note += ' WARNING: background containers exceed one CPU.'
    print(note, file=sys.stderr)
open(sys.argv[3], 'w').write(note + '\n')
PY
}

for variant in $variants; do
  case "$variant" in
    postgres) export BENCH_IMAGE=postgres:17.11; schema="$repo_root/schema/postgresql/v12/visibility/schema.sql" ;;
    pdb-cols|pdb-cols-hybrid) export BENCH_IMAGE="paradedb/paradedb:${PARADEDB_TAG:-v0.26.1-pg17}"; schema="$bench_dir/../spike/paradedb-schema.sql" ;;
    pdb-json-hybrid) export BENCH_IMAGE="paradedb/paradedb:${PARADEDB_TAG:-v0.26.1-pg17}"; schema="$bench_dir/json-hybrid-schema.sql" ;;
    *) echo "unknown variant: $variant" >&2; exit 1 ;;
  esac
  dir="$output/$variant"
  if [[ -f $dir/done ]]; then echo "Skipping completed $variant"; continue; fi
  rm -rf "$dir"
  mkdir -p "$dir"
  "${compose[@]}" down -v >/dev/null 2>&1
  "${compose[@]}" up -d --wait > "$dir/compose-up.log"
  initialized=0
  for ((attempt=0; attempt<120; attempt++)); do
    if [[ $(docker logs "$container" 2>&1 | grep -c 'database system is ready to accept connections' || true) -ge 2 ]]; then
      initialized=1
      break
    fi
    sleep 2
  done
  if [[ $initialized != 1 ]]; then echo "database initialization timed out: $variant" >&2; exit 1; fi
  "${compose[@]}" up -d --wait >> "$dir/compose-up.log"
  docker exec -i "$container" psql -v ON_ERROR_STOP=1 -U postgres -d visibility < "$schema" > "$dir/schema.log"
  if [[ $variant == pdb-cols-hybrid ]]; then
    docker exec -i "$container" psql -v ON_ERROR_STOP=1 -U postgres -d visibility < "$bench_dir/core-indexes.sql" >> "$dir/schema.log"
  fi
  "$output/bench" -mode preload -variant "$variant" -rows "$rows" -dsn "$dsn" > "$dir/preload.log"
  docker exec "$container" psql -v ON_ERROR_STOP=1 -U postgres -d visibility -c 'ANALYZE executions_visibility' > "$dir/analyze.log"
  snapshot "$dir" before "$variant"
  "$output/bench" -mode check -variant "$variant" -dsn "$dsn" > "$dir/check.json"
  python3 - "$output" "$variant" <<'PY'
import json, pathlib, sys
root, variant = pathlib.Path(sys.argv[1]), sys.argv[2]
actual = json.loads((root / variant / 'check.json').read_text())['results']
reference = root / 'reference-results.json'
if reference.exists():
    expected = json.loads(reference.read_text())
    for name in expected:
        if actual.get(name) != expected[name]:
            raise SystemExit(f'{variant} correctness mismatch for {name}: {actual.get(name)} != {expected[name]}')
else:
    reference.write_text(json.dumps(actual, indent=2, sort_keys=True) + '\n')
PY
  for writers in $connections; do
    capture_background "$dir" "$writers"
    : > "$dir/$writers-docker-stats.csv"
    "$output/bench" -mode run -variant "$variant" -rows "$rows" -writers "$writers" -readers "$readers" -duration "$duration" -dsn "$dsn" > "$dir/$writers.json" &
    bench_pid=$!
    while kill -0 "$bench_pid" 2>/dev/null; do
      docker stats --no-stream --format '{{.CPUPerc}},{{.MemUsage}}' "$container" >> "$dir/$writers-docker-stats.csv"
      sleep 10
    done
    wait "$bench_pid"
    snapshot "$dir" "after-$writers" "$variant"
  done
  if [[ ${BENCH_RUN_CRASH:-0} == 1 && $variant != postgres ]]; then
    "$output/bench" -mode run -variant "$variant" -allow-errors -rows "$rows" -writers 64 -readers "$readers" -duration 2m -dsn "$dsn" > "$dir/crash-load.json" &
    crash_pid=$!
    sleep 30
    docker kill --signal KILL "$container" > /dev/null
    docker start "$container" > /dev/null
    recovered=0
    for ((attempt=0; attempt<900; attempt++)); do
      if docker exec "$container" pg_isready -U postgres -d visibility > /dev/null 2>&1; then recovered=1; break; fi
      sleep 2
    done
    if [[ $recovered != 1 ]]; then echo "crash recovery timed out: $variant" >&2; exit 1; fi
    wait "$crash_pid"
    docker exec "$container" psql -v ON_ERROR_STOP=1 -At -U postgres -d visibility -c 'SELECT count(*) FROM executions_visibility' > "$dir/crash-recovery-count.txt"
    if [[ $variant == pdb-json-hybrid ]]; then
      "$output/bench" -mode check -variant "$variant" -dsn "$dsn" > "$dir/crash-check.json"
    else
      "$output/bench" -mode verify -dsn "$dsn" > "$dir/crash-verify.json"
    fi
  fi
  docker logs "$container" > "$dir/container.log" 2>&1
  "${compose[@]}" down -v > "$dir/compose-down.log"
  touch "$dir/done"
done
python3 "$bench_dir/summarize.py" "$output"
