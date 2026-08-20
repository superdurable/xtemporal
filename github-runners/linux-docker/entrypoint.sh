#!/usr/bin/env bash

set -euo pipefail

runner_url="${RUNNER_URL:?RUNNER_URL is required}"
runner_name="${RUNNER_NAME:?RUNNER_NAME is required}"
runner_labels="${RUNNER_LABELS:?RUNNER_LABELS is required}"
token_file="${RUNNER_TOKEN_FILE:-/run/secrets/runner-token}"

install -d -o runner -g runner /opt/actions-runner/_work

dockerd --host=unix:///var/run/docker.sock --storage-driver=overlay2 >/var/log/dockerd.log 2>&1 &
dockerd_pid=$!

for attempt in $(seq 1 60); do
  if docker info >/dev/null 2>&1; then
    break
  fi
  if ! kill -0 "$dockerd_pid" 2>/dev/null || [[ "$attempt" == 60 ]]; then
    cat /var/log/dockerd.log >&2
    exit 1
  fi
  sleep 1
done

if [[ ! -f .runner ]]; then
  if [[ ! -s "$token_file" ]]; then
    echo "Runner registration token is missing." >&2
    exit 1
  fi
  runner_token="$(<"$token_file")"
  : > "$token_file"
  runuser --user runner -- env HOME=/home/runner USER=runner LOGNAME=runner ./config.sh \
    --unattended \
    --replace \
    --url "$runner_url" \
    --token "$runner_token" \
    --name "$runner_name" \
    --labels "$runner_labels" \
    --work _work
  unset runner_token
fi

runuser --user runner -- env HOME=/home/runner USER=runner LOGNAME=runner ./run.sh &
runner_pid=$!

terminate() {
  kill -TERM "$runner_pid" "$dockerd_pid" 2>/dev/null || true
  wait "$runner_pid" 2>/dev/null || true
  wait "$dockerd_pid" 2>/dev/null || true
}

trap terminate INT TERM EXIT
wait "$runner_pid"
