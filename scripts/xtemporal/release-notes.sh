#!/usr/bin/env bash
set -euo pipefail

tag=${1:?usage: release-notes.sh vX.Y.Z[-x.N]}
release_pattern='^v[0-9]+\.[0-9]+\.[0-9]+(-x\.[0-9]+)?$'
[[ $tag =~ $release_pattern ]] || { echo "invalid release tag: $tag" >&2; exit 1; }
# Hotfix releases (vX.Y.Z-x.N) are built on upstream vX.Y.Z.
upstream_tag=${tag%%-*}
git rev-parse --verify "refs/tags/$tag^{commit}" >/dev/null

previous=$(git tag --list 'v*' | grep -E "$release_pattern" | sort -V | awk -v current="$tag" '$0 == current { print previous; exit } { previous = $0 }')
range=$tag
if [[ -n $previous ]] && git merge-base --is-ancestor "$previous" "$tag"; then
  range="$previous..$tag"
fi

printf 'Upstream release: https://github.com/temporalio/temporal/releases/tag/%s\n\n## xtemporal changes\n' "$upstream_tag"
found=false
while IFS= read -r subject; do
  if [[ $subject =~ ^Merge\ pull\ request\ \#([0-9]+)\  ]]; then
    number=${BASH_REMATCH[1]}
    title=$(gh pr view "$number" --json title --jq .title)
    printf -- '- %s (#%s)\n' "$title" "$number"
    found=true
  fi
done < <(git log --first-parent --merges --reverse --format=%s "$range")
if [[ $found == false ]]; then
  echo '- No merged pull requests.'
fi
