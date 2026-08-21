#!/usr/bin/env bash

set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
image="dex-engine-actions-runner:2.336.0"
platform="linux/arm64"
container_prefix="dex-engine-linux-dind-runner"
runner_prefix="dex-engine-linux-dind"
repository="superdurable/temporal-server"
runner_count=6

require_orbstack() {
  if [[ "$(docker context show)" != orbstack ]]; then
    echo "Docker context must be orbstack." >&2
    exit 1
  fi
}

suffix_for() {
  printf '%02d' "$1"
}

container_for() {
  if (( $1 == 1 )); then
    printf '%s' "$container_prefix"
  else
    printf '%s-%s' "$container_prefix" "$(suffix_for "$1")"
  fi
}

runner_for() {
  if (( $1 == 1 )); then
    printf '%s' "$runner_prefix"
  else
    printf '%s-%s' "$runner_prefix" "$(suffix_for "$1")"
  fi
}

role_for() {
  if (( $1 <= 2 )); then
    printf '%s' heavy
  else
    printf '%s' light
  fi
}

build() {
  require_orbstack
  docker build --platform "$platform" --tag "$image" "$script_dir"
}

create_runner() {
  index="$1"
  container="$(container_for "$index")"
  if docker container inspect "$container" >/dev/null 2>&1; then
    docker start "$container" >/dev/null 2>&1 || true
    return
  fi

  suffix="$(suffix_for "$index")"
  token_file="$script_dir/.runner-token-${suffix}"
  umask 077
  gh api --method POST "repos/${repository}/actions/runners/registration-token" --jq .token > "$token_file"

  docker run \
    --detach \
    --platform "$platform" \
    --privileged \
    --restart unless-stopped \
    --label com.superdurable.dex-engine.linux-runner=true \
    --name "$container" \
    --hostname "$(runner_for "$index")" \
    --env "RUNNER_URL=https://github.com/${repository}" \
    --env "RUNNER_NAME=$(runner_for "$index")" \
    --env "RUNNER_LABELS=linux-dind,linux-dind-${suffix},linux-dind-$(role_for "$index")" \
    --mount "type=bind,source=${token_file},target=/run/secrets/runner-token" \
    --mount "type=volume,source=dex-engine-runner-docker-${suffix},target=/var/lib/docker" \
    --mount "type=volume,source=dex-engine-runner-work-${suffix},target=/opt/actions-runner/_work" \
    "$image" >/dev/null
}

wait_for_runners() {
  for attempt in $(seq 1 90); do
    online="$(gh api "repos/${repository}/actions/runners?per_page=100" \
      --jq "[.runners[] | select(.name == \"${runner_prefix}\" or (.name | startswith(\"${runner_prefix}-\"))) | select(.status == \"online\")] | length")"
    if (( online >= runner_count )); then
      return
    fi
    sleep 2
  done
  echo "Not all runners became online in GitHub." >&2
  exit 1
}

up() {
  require_orbstack
  if ! docker image inspect "$image" >/dev/null 2>&1; then
    build
  fi
  for index in $(seq 1 "$runner_count"); do
    create_runner "$index"
  done
  wait_for_runners
  status
}

status() {
  require_orbstack
  docker ps --all --filter "name=${container_prefix}" --format 'table {{.Names}}\t{{.Status}}'
  gh api "repos/${repository}/actions/runners?per_page=100" \
    --jq ".runners[] | select(.name == \"${runner_prefix}\" or (.name | startswith(\"${runner_prefix}-\"))) | {name,status,busy,labels:[.labels[].name]}"
}

stop() {
  require_orbstack
  for index in $(seq 1 "$runner_count"); do
    container="$(container_for "$index")"
    if docker container inspect "$container" >/dev/null 2>&1; then
      docker stop "$container" >/dev/null
    fi
  done
}

reset() {
  require_orbstack
  runners_json="$(gh api "repos/${repository}/actions/runners?per_page=100")"
  busy=0
  for index in $(seq 1 "$runner_count"); do
    runner="$(runner_for "$index")"
    if [[ "$(jq --arg runner "$runner" '[.runners[] | select(.name == $runner and .busy)] | length' <<<"$runners_json")" != 0 ]]; then
      ((busy += 1))
    fi
  done
  if (( busy > 0 )); then
    echo "Refusing to reset while ${busy} runners are busy." >&2
    exit 1
  fi
  for index in $(seq 1 "$runner_count"); do
    container="$(container_for "$index")"
    if docker container inspect "$container" >/dev/null 2>&1; then
      docker rm --force "$container" >/dev/null
    fi
  done
  for index in $(seq 1 "$runner_count"); do
    runner="$(runner_for "$index")"
    while IFS= read -r runner_id; do
      if [[ -n "$runner_id" ]]; then
        gh api --method DELETE "repos/${repository}/actions/runners/${runner_id}" >/dev/null
      fi
    done < <(jq --raw-output --arg runner "$runner" '.runners[] | select(.name == $runner) | .id' <<<"$runners_json")
  done
  rm -f "$script_dir"/.runner-token-*
  up
}

case "${1:-}" in
  build) build ;;
  up) up ;;
  status) status ;;
  stop) stop ;;
  reset) reset ;;
  logs) docker logs --follow "$(container_for "${2:?runner index is required}")" ;;
  *)
    echo "Usage: $0 {build|up|status|stop|reset|logs index}" >&2
    exit 1
    ;;
esac
