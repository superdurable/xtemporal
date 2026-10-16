#!/usr/bin/env bash
# Dex-profile visibility benchmark driver; see dex-profile-benchmark.md.
# Resumable: each completed step leaves a marker in <output-dir>, and the database volume is kept
# until the run finishes, so rerunning the same command continues where it stopped.
set -euo pipefail

bench_dir=$(cd "$(dirname "$0")" && pwd)
output=${1:?usage: dex-compare.sh <output-dir>}
mkdir -p "$output"
output=$(cd "$output" && pwd)
project=${BENCH_PROJECT:-xtpdbdex}
export BENCH_PORT=${BENCH_PORT:-56650}
export BENCH_IMAGE="paradedb/paradedb:${PARADEDB_TAG:-v0.26.1-pg17}"
compose=(docker compose -f "$bench_dir/compare-compose.yml" -p "$project")
container="$project-db-1"
dsn="postgres://postgres:spike@127.0.0.1:$BENCH_PORT/visibility?sslmode=disable"

namespaces=${BENCH_NAMESPACES:-10000}
rows=${BENCH_ROWS:-10000000}
heavy_rows=${BENCH_HEAVY_ROWS:-2000000}
duration=${BENCH_DURATION:-10m}
browsers=${BENCH_BROWSERS:-200}
pool=${BENCH_POOL:-32}
worst_writers=${BENCH_WORST_WRITERS:-16}
paging_writers=${BENCH_PAGING_WRITERS:-8}
reps=${BENCH_REPS:-20}

(cd "$bench_dir" && go build -o "$output/dex-bench" ./dex)
bench=("$output/dex-bench" -dsn "$dsn")

psql_exec() {
  docker exec "$container" psql -v ON_ERROR_STOP=1 -At -U postgres -d visibility -c "$1"
}

done_step() { [[ -f $output/.done-$1 ]]; }
mark() { touch "$output/.done-$1"; }

ensure_up() {
  "${compose[@]}" up -d --wait > "$output/compose-up.log" 2>&1
  # First start runs initdb on a temporary server, then restarts; wait for the second ready line.
  local fresh=-1 i
  for ((i=0; i<300 && fresh<0; i++)); do
    if docker logs "$container" 2>&1 | grep -q 'PostgreSQL init process complete'; then fresh=1
    elif docker logs "$container" 2>&1 | grep -q 'Skipping initialization'; then fresh=0
    else sleep 2; fi
  done
  for ((i=0; i<120; i++)); do
    if [[ $(docker logs "$container" 2>&1 | grep -c 'database system is ready to accept connections' || true) -ge $((1 + fresh)) ]] \
      && psql_exec 'SELECT 1' > /dev/null 2>&1; then return; fi
    sleep 2
  done
  echo "database did not become ready" >&2
  exit 1
}

host_info() {
  {
    echo "date: $(date -u +%FT%TZ)"
    echo "cpus: $(nproc)"
    grep -m1 'model name' /proc/cpuinfo || true
    free -g | sed -n 1,2p
    df -h / | tail -1
    docker version --format 'docker {{.Server.Version}}'
    echo "image: $BENCH_IMAGE"
    echo "shared_buffers: ${BENCH_SHARED_BUFFERS:-8GB} effective_cache_size: ${BENCH_EFFECTIVE_CACHE_SIZE:-24GB}"
    echo "load generator: same host as the database"
  } > "$output/host.txt"
}

capture_background() {
  local label=$1
  docker stats --no-stream --format '{{.Name}},{{.CPUPerc}},{{.MemUsage}}' > "$output/$label-all-containers.csv"
  uptime > "$output/$label-host-load.txt"
}

set_c3() {
  local want=$1 have
  have=$(psql_exec "SELECT count(*) FROM pg_indexes WHERE indexname = 'by_flow_type_status'")
  if [[ $want == on && $have == 0 ]]; then
    "${bench[@]}" -mode c3 > "$output/c3-build.json"
  elif [[ $want == off && $have != 0 ]]; then
    "${bench[@]}" -mode drop-c3 > /dev/null
  fi
}

mix_step() {
  local label=$1 c3=$2 writers=$3
  done_step "$label" && return
  set_c3 "$c3"
  psql_exec 'CHECKPOINT' > /dev/null
  capture_background "$label"
  : > "$output/$label-docker-stats.csv"
  "${bench[@]}" -mode mix -writers "$writers" -browsers "$browsers" -pool "$pool" -duration "$duration" > "$output/$label.json" &
  local pid=$!
  while kill -0 "$pid" 2>/dev/null; do
    docker stats --no-stream --format '{{.CPUPerc}},{{.MemUsage}}' "$container" >> "$output/$label-docker-stats.csv" || true
    # A full disk crashes the database and invalidates the run, so stop early instead.
    if [[ $(df --output=avail -BM "$output" | tail -1 | tr -dc 0-9) -lt ${BENCH_MIN_FREE_MB:-1500} ]]; then
      kill "$pid" 2>/dev/null || true
      echo "aborting $label: free disk below ${BENCH_MIN_FREE_MB:-1500} MB" >&2
      exit 1
    fi
    sleep 10
  done
  wait "$pid"
  "${bench[@]}" -mode sizes > "$output/$label-sizes.json"
  mark "$label"
}

host_info
ensure_up
if ! done_step load; then
  # A partial load cannot be resumed; start from a fresh volume.
  "${compose[@]}" down -v > /dev/null 2>&1 || true
  ensure_up
  "${bench[@]}" -mode load -schema "$bench_dir/dex-profile-schema.sql" -namespaces "$namespaces" -rows "$rows" \
    -heavy-rows "$heavy_rows" -parallel "$(nproc)" > "$output/load.json"
  mark load
fi
if ! done_step sizes-before; then "${bench[@]}" -mode sizes > "$output/sizes-before.json"; mark sizes-before; fi
if ! done_step explain-no-c3; then set_c3 off; "${bench[@]}" -mode explain > "$output/explain-no-c3.json"; mark explain-no-c3; fi

# Order A16, B16, B64, A64 gives both index variants the same average position in the run.
mix_step mix-no-c3-16 off 16
if ! done_step explain-c3; then set_c3 on; "${bench[@]}" -mode explain > "$output/explain-c3.json"; mark explain-c3; fi
mix_step mix-c3-16 on 16
mix_step mix-c3-64 on 64
mix_step mix-no-c3-64 off 64

if ! done_step worst; then
  set_c3 off
  "${bench[@]}" -mode worst -writers "$worst_writers" -reps "$reps" > "$output/worst.json"
  mark worst
fi
if ! done_step paging; then
  "${bench[@]}" -mode paging -writers "$paging_writers" > "$output/paging.json"
  mark paging
fi
if ! done_step sizes-after; then "${bench[@]}" -mode sizes > "$output/sizes-after.json"; mark sizes-after; fi

docker logs "$container" > "$output/container.log" 2>&1 || true
python3 "$bench_dir/dex-summarize.py" "$output"
if [[ ${BENCH_KEEP_VOLUME:-0} != 1 ]]; then
  "${compose[@]}" down -v > "$output/compose-down.log" 2>&1
fi
