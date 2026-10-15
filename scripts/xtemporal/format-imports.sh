#!/usr/bin/env bash
set -euo pipefail

root=${1:?usage: format-imports.sh TREE}
gci_dir=$(mktemp -d)
trap 'rm -rf "$gci_dir"' EXIT
GOBIN=$gci_dir go install github.com/daixiang0/gci@v0.13.6
(cd "$root" && "$gci_dir/gci" write --skip-generated -s standard -s default ./*)
