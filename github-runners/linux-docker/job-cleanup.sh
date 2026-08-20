#!/usr/bin/env bash

set -euo pipefail

container_ids="$(docker ps --all --quiet)"
if [[ -n "$container_ids" ]]; then
  docker rm --force $container_ids >/dev/null
fi
docker network prune --force >/dev/null
docker volume prune --force >/dev/null
rm -rf /home/runner/go/pkg/mod /home/runner/.cache/go-build 2>/dev/null || true
