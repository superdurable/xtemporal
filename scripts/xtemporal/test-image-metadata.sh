#!/usr/bin/env bash
set -euo pipefail

script_dir=$(cd "$(dirname "$0")" && pwd)
test_dir=$(mktemp -d)
trap 'rm -rf "$test_dir"' EXIT

cd "$test_dir"
git init -q
git config user.email test@example.com
git config user.name Test
mkdir -p common/headers
echo 'ServerVersion = "1.32.1"' > common/headers/version_checker.go
git add .
git commit -qm 'Import upstream temporalio/temporal v1.32.1'
git tag v1.32.0
git tag v1.32.1
revision=$(git rev-parse HEAD)

check() {
  local ref=$1
  shift
  local output="$test_dir/output"
  : > "$output"
  bash "$script_dir/image-metadata.sh" "$ref" "$revision" "$output"
  for expected in "$@"; do
    if ! grep -Fxq "$expected" "$output"; then
      echo "Missing $expected for $ref" >&2
      exit 1
    fi
  done
}

check refs/tags/v1.32.1 image_version_tag=1.32.1 image_minor_tag=1.32 tag_latest=true tag_main=false
check refs/tags/v1.32.0 image_version_tag=1.32.0 image_minor_tag= tag_latest=false
check refs/tags/v1.32.1-x.1 image_version_tag=1.32.1-x.1 image_minor_tag= tag_latest=false
check refs/heads/main image_version_tag= tag_latest=false tag_main=true
check refs/pull/1/merge image_version_tag= tag_latest=false tag_main=false

if bash "$script_dir/image-metadata.sh" refs/tags/vbad "$revision" "$test_dir/output" >/dev/null 2>&1; then
  echo 'Invalid release tag was accepted' >&2
  exit 1
fi
