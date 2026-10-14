#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 1 ]]; then
  echo "usage: $0 <persistence-driver>" >&2
  exit 2
fi

xt_driver="$1"
xt_binary="$(mktemp "${RUNNER_TEMP:-/tmp}/xt-functional.XXXXXX")"
xt_list="$(mktemp "${RUNNER_TEMP:-/tmp}/xt-functional-list.XXXXXX")"
trap 'rm -f "$xt_binary" "$xt_list"' EXIT

go test -c -tags disable_grpc_modules,test_dep -p 1 -o "$xt_binary" ./tests
"$xt_binary" -test.list '^Test' > "$xt_list"
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
  ./develop/github/monitor_test.sh "$xt_binary" -test.timeout=25m -test.parallel=1 -test.shuffle=on -test.run="^(${xt_pattern})$" -persistenceType=sql -persistenceDriver="$xt_driver"
done
