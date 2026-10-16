#!/usr/bin/env bash
# Reproduces the evidence for the functional tests that limitless-verify/limitless-e2e.sh skips by default.
# Those tests pass with postgres12_limitless at LAN latency, and fail the same way on vanilla PostgreSQL with
# the stock postgres12_pgx plugin once a toxiproxy adds round-trip latency. So their failures over the
# internet come from latency, not from Limitless or the plugin.
#
# Usage, from the repository root: limitless-verify/latency-compare.sh
# Optional env: LATENCY_MS (one-way, default 15), PG_PORT (default 55441), PROXY_PORT (default 55442).
set -euo pipefail

latency_ms=${LATENCY_MS:-15}
pg_port=${PG_PORT:-55441}
proxy_port=${PROXY_PORT:-55442}
api_port=${TOXIPROXY_API_PORT:-8475}
run_id=xt-latency-compare-$$
log_dir=${LOG_DIR:-/tmp/$run_id}
tests='^(TestMaxBufferedEventSuite|TestChildWorkflowSuite|TestContinueAsNewTestSuite)$/^(TestMaxBufferedEventsLimit|TestCronChildWorkflowExecution|TestChildWorkflowWithContinueAsNewParentTerminate)$'
mkdir -p "$log_dir"
cd "$(git rev-parse --show-toplevel)"

cleanup() { docker rm -f "$run_id-pg" "$run_id-proxy" >/dev/null 2>&1 || true; }
trap cleanup EXIT

docker run -d --name "$run_id-pg" -p "127.0.0.1:$pg_port:5432" \
  -e POSTGRES_USER=temporal -e POSTGRES_PASSWORD=temporal postgres:16 -c max_connections=500 >/dev/null
docker run -d --name "$run_id-proxy" --add-host host.docker.internal:host-gateway \
  -p "127.0.0.1:$api_port:8474" -p "127.0.0.1:$proxy_port:$proxy_port" ghcr.io/shopify/toxiproxy:2.9.0 >/dev/null
until docker exec "$run_id-pg" pg_isready -U temporal >/dev/null 2>&1; do sleep 1; done
until curl -fs "127.0.0.1:$api_port/version" >/dev/null; do sleep 1; done

api="127.0.0.1:$api_port"
curl -fs -X POST "$api/proxies" \
  -d "{\"name\":\"pg\",\"listen\":\"0.0.0.0:$proxy_port\",\"upstream\":\"host.docker.internal:$pg_port\"}" >/dev/null
for stream in upstream downstream; do
  curl -fs -X POST "$api/proxies/pg/toxics" \
    -d "{\"name\":\"$stream\",\"type\":\"latency\",\"stream\":\"$stream\",\"attributes\":{\"latency\":$latency_ms}}" >/dev/null
done

summarize() { grep -E '^\s*--- (PASS|FAIL)' "$1" || true; }

echo "== postgres12_limitless, Limitless schema on vanilla PostgreSQL, no added latency"
status_direct=0
POSTGRES_SEEDS=127.0.0.1 POSTGRES_PORT=$pg_port LIMITLESS_TEST=pg \
  go test -tags test_dep -count=1 -p 1 -parallel 1 -timeout 30m -v -run "$tests" ./tests/ \
  -persistenceType=sql -persistenceDriver=postgres12_limitless >"$log_dir/direct.log" 2>&1 || status_direct=$?
summarize "$log_dir/direct.log"

echo "== postgres12_pgx, vanilla PostgreSQL behind $((2 * latency_ms))ms RTT"
status_proxied=0
POSTGRES_SEEDS=127.0.0.1 POSTGRES_PORT=$proxy_port \
  go test -tags test_dep -count=1 -p 1 -parallel 1 -timeout 40m -v -run "$tests" ./tests/ \
  -persistenceType=sql -persistenceDriver=postgres12_pgx >"$log_dir/proxied.log" 2>&1 || status_proxied=$?
summarize "$log_dir/proxied.log"

echo "logs: $log_dir"
if [[ $status_direct == 0 && $status_proxied != 0 ]]; then
  echo "result: latency alone reproduces the failures"
else
  echo "result: unexpected (direct exit $status_direct, proxied exit $status_proxied); inspect the logs"
  exit 1
fi
