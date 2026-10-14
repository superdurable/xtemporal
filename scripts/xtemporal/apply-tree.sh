#!/usr/bin/env bash
set -euo pipefail

root=${1:?usage: apply-tree.sh TREE}
script_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)

while IFS= read -r pattern || [[ -n $pattern ]]; do
  [[ -z $pattern || $pattern == \#* ]] && continue
  find "$root" -path "$root/.git" -prune -o -path "$root/$pattern" -print0 |
    while IFS= read -r -d '' path; do rm -rf -- "$path"; done
done < "$script_dir/upstream-denylist.txt"

# Restrict the match to the module itself and its import-path descendants.
while IFS= read -r -d '' path; do
  perl -pi -e 's{go\.temporal\.io/server(?![A-Za-z0-9_.-])}{github.com/superdurable/xtemporal}g; s{go\.temporal\.io/auto-scaled-workers(?![A-Za-z0-9_.-])}{github.com/superdurable/xtemporal/components/autoscaledworkers}g' "$path"
done < <(rg -l -0 --hidden --no-ignore -F -e 'go.temporal.io/server' -e 'go.temporal.io/auto-scaled-workers' "$root" -g '!.git/**' -g '!go.sum' || true)
