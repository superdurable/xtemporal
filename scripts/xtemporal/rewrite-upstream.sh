#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 2 ]]; then
  echo "usage: $0 <upstream-tag> <output-dir>" >&2
  exit 2
fi

tag=$1
output=$2
script_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
repo=$(git -C "$script_dir" rev-parse --show-toplevel)
ref="refs/upstream/$tag"
git -C "$repo" rev-parse --verify "$ref^{commit}" >/dev/null || {
  echo "missing upstream ref $ref" >&2
  exit 1
}
[[ ! -e $output ]] || { echo "output directory already exists: $output" >&2; exit 1; }
output=$(python3 -c 'import os,sys; print(os.path.abspath(sys.argv[1]))' "$output")
git -C "$repo" worktree add --detach "$output" "$ref"

version=$(sed -nE 's/^[[:space:]]*go\.temporal\.io\/auto-scaled-workers[[:space:]]+([^[:space:]]+).*/\1/p' "$output/go.mod")
[[ -n $version ]] || { echo "auto-scaled-workers version missing from upstream go.mod" >&2; exit 1; }
module_json=$(cd "$output" && go mod download -json "go.temporal.io/auto-scaled-workers@$version")
module_dir=$(printf '%s\n' "$module_json" | python3 -c 'import json,sys; print(json.load(sys.stdin)["Dir"])')
mkdir -p "$output/components/autoscaledworkers"
cp -R "$module_dir"/. "$output/components/autoscaledworkers/"
chmod -R u+w "$output/components/autoscaledworkers"
rm -f "$output/components/autoscaledworkers/go.mod" "$output/components/autoscaledworkers/go.sum"
(cd "$output" && go mod edit -droprequire=go.temporal.io/auto-scaled-workers)
"$script_dir/apply-tree.sh" "$output"
python3 "$script_dir/fix-descriptor-lengths.py" "$output"
(cd "$output" && go mod tidy)
if (cd "$output" && go list -m all) | grep -q '^go.temporal.io/server\([[:space:]]\|$\)'; then
  echo 'safety check failed: go.temporal.io/server remains in module graph' >&2
  exit 1
fi
if rg -n --hidden --no-ignore -F 'go.temporal.io/server' "$output" -g '!.git/**'; then
  echo 'safety check failed: go.temporal.io/server remains in tree' >&2
  exit 1
fi
