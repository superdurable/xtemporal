#!/usr/bin/env bash
set -euo pipefail

script_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
fixture=$(mktemp -d)
trap 'rm -rf "$fixture"' EXIT
mkdir -p "$fixture/.github/workflows" "$fixture/.github/actions" "$fixture/pkg"
cat > "$fixture/pkg/paths.go" <<'GO'
package pkg
const paths = "go.temporal.io/server go.temporal.io/server/x go.temporal.io/api go.temporal.io/sdk go.temporal.io/serverless"
GO
printf 'module go.temporal.io/server\n' > "$fixture/go.mod"
printf 'remove\n' > "$fixture/.github/workflows/ci.yml"
printf 'remove\n' > "$fixture/.github/actions/action.yml"
printf 'keep\n' > "$fixture/.github/.golangci.yml"
cat > "$fixture/pkg/descriptor.pb.go" <<'GO'
package pkg
const rawDesc = "B*Z(go.temporal.io/server/api/clock/v1;clockb\x06proto3"
GO
"$script_dir/apply-tree.sh" "$fixture"
python3 "$script_dir/fix-descriptor-lengths.py" "$fixture"
expected='github.com/superdurable/xtemporal github.com/superdurable/xtemporal/x go.temporal.io/api go.temporal.io/sdk go.temporal.io/serverless'
rg -Fq "$expected" "$fixture/pkg/paths.go"
[[ ! -e "$fixture/.github/workflows" && ! -e "$fixture/.github/actions" ]]
[[ -f "$fixture/.github/.golangci.yml" ]]
rg -Fq 'B6Z4github.com/superdurable/xtemporal/api/clock/v1;clockb' "$fixture/pkg/descriptor.pb.go"
first=$(find "$fixture" -type f -print0 | sort -z | xargs -0 shasum -a 256 | shasum -a 256)
"$script_dir/apply-tree.sh" "$fixture"
python3 "$script_dir/fix-descriptor-lengths.py" "$fixture"
second=$(find "$fixture" -type f -print0 | sort -z | xargs -0 shasum -a 256 | shasum -a 256)
[[ $first == "$second" ]]
echo 'fixture rewrite: PASS (exact prefix, denylist, idempotency)'
