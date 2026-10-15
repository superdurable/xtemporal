#!/usr/bin/env bash
set -euo pipefail

ref=$1
revision=$2
output=$3

if [[ ! $revision =~ ^[0-9a-f]{40}$ ]]; then
  echo "Invalid revision: $revision" >&2
  exit 1
fi

upstream_version=$(sed -n 's/^[[:space:]]*ServerVersion = "\([0-9][^"]*\)"/\1/p' common/headers/version_checker.go)
if [[ ! $upstream_version =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "Cannot determine upstream server version" >&2
  exit 1
fi

version_tag=
minor_tag=
latest=false
main=false
image_version=$upstream_version

case "$ref" in
  refs/heads/main)
    main=true
    ;;
  refs/tags/v*)
    tag=${ref#refs/tags/v}
    if [[ ! $tag =~ ^[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$ ]]; then
      echo "Invalid release tag: $tag" >&2
      exit 1
    fi
    image_version=$tag
    version_tag=$tag
    if [[ $tag =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
      current_tag="v$tag"
      newest_tag=$(git tag --list 'v*' | grep -E '^v[0-9]+\.[0-9]+\.[0-9]+$' | sort -V | tail -1 || true)
      if [[ $current_tag == "$newest_tag" ]]; then
        minor_tag=${tag%.*}
        latest=true
      fi
    fi
    ;;
  refs/pull/*|refs/heads/*)
    ;;
  *)
    echo "Unsupported ref: $ref" >&2
    exit 1
    ;;
esac

{
  echo "image_sha_tag=sha-$revision"
  echo "revision=$revision"
  echo "image_version_tag=$version_tag"
  echo "image_minor_tag=$minor_tag"
  echo "tag_latest=$latest"
  echo "tag_main=$main"
  echo "image_version=$image_version"
  echo "upstream_version=$upstream_version"
  echo "release_tag=${ref#refs/tags/}"
} >> "$output"
