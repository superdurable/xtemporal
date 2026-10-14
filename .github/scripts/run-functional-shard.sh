#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 1 ]]; then
  echo "usage: $0 <persistence-driver>" >&2
  exit 2
fi

xt_driver="$1"
xt_parallelism="${XT_TEST_PARALLELISM:-1}"
xt_list="$(mktemp "${RUNNER_TEMP:-/tmp}/xt-functional-list.XXXXXX")"
trap 'rm -f "$xt_list"' EXIT

make prepare-coverage-test
go test -list '^Test' -tags disable_grpc_modules,test_dep -p 1 ./tests > "$xt_list"
mapfile -t xt_suites < <(grep '^Test' "$xt_list")
if [[ ${#xt_suites[@]} -eq 0 ]]; then
  echo "No functional test suites found." >&2
  exit 1
fi

xt_batch_size=10
for ((xt_start=0; xt_start<${#xt_suites[@]}; xt_start+=xt_batch_size)); do
  xt_batch=("${xt_suites[@]:xt_start:xt_batch_size}")
  xt_pattern="$(IFS='|'; echo "${xt_batch[*]}")"
  echo "Running functional suites $((xt_start + 1))-$((xt_start + ${#xt_batch[@]})) of ${#xt_suites[@]}"
  xt_report=".testoutput/junit.functional.${TEST_SHARD_INDEX:-0}.${xt_start}.xml"
  xt_coverage=".testoutput/coverage.functional.${TEST_SHARD_INDEX:-0}.${xt_start}.out"
  go run ./cmd/tools/test-runner test --gotestsum-path=.bin/gotestsum-v1.12.3 --max-attempts=3 --junitfile="$xt_report" -- \
    -tags disable_grpc_modules,test_dep -p 1 -timeout=25m -parallel="$xt_parallelism" -shuffle=on \
    -run="^(${xt_pattern})$" -coverprofile="$xt_coverage" ./tests \
    -args -persistenceType=sql -persistenceDriver="$xt_driver"
done
