#!/usr/bin/env bash

set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
pool_dir="$script_dir/linux-docker"

for command_name in orbctl docker gh; do
  if ! command -v "$command_name" >/dev/null 2>&1; then
    echo "Required command is missing: $command_name" >&2
    exit 1
  fi
done

orbctl start
docker context inspect orbstack >/dev/null
docker context use orbstack >/dev/null

for attempt in $(seq 1 90); do
  if docker info >/dev/null 2>&1; then
    "$pool_dir/scale.sh" up
    exit 0
  fi
  if [[ "$attempt" == 90 ]]; then
    echo "OrbStack Docker did not become ready within three minutes." >&2
    exit 1
  fi
  sleep 2
done
