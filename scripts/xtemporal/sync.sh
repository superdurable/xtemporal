#!/usr/bin/env bash
set -euo pipefail

usage() { echo "usage: $0 [--tag vX.Y.Z] [--dry-run]" >&2; exit 2; }
tag=
dry_run=false
while (($#)); do
  case $1 in
    --tag) (($# >= 2)) || usage; tag=$2; shift 2 ;;
    --dry-run) dry_run=true; shift ;;
    *) usage ;;
  esac
done
script_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
repo=$(git -C "$script_dir" rev-parse --show-toplevel)
cd "$repo"
repo_name=${GITHUB_REPOSITORY:-superdurable/xtemporal}
if [[ -z $tag ]]; then
  release=$(gh api "repos/temporalio/temporal/releases/latest")
  tag=$(python3 -c 'import json,sys; r=json.load(sys.stdin); print(r["tag_name"] if not r["draft"] and not r["prerelease"] else "")' <<< "$release")
  [[ -n $tag ]] || { echo 'upstream Latest is a draft or prerelease; skipping'; exit 0; }
fi
[[ $tag =~ ^v([0-9]+)\.([0-9]+)\.([0-9]+)$ ]] || { echo "invalid release tag: $tag" >&2; exit 1; }

git fetch origin --tags
highest=$(git tag --list | awk '/^v[0-9]+\.[0-9]+\.[0-9]+$/ {print}' | sort -V | tail -1)
if [[ -n $highest && $(printf '%s\n%s\n' "$tag" "$highest" | sort -V | tail -1) == "$highest" ]]; then
  if [[ $tag == "$highest" ]]; then
    echo "already up to date: $tag"
  else
    echo "refusing older release $tag; latest local tag is $highest"
  fi
  exit 0
fi
branch="sync/$tag"
if git ls-remote --exit-code --heads origin "$branch" >/dev/null 2>&1; then
  echo "already exists: $branch"
  exit 0
fi
if [[ -n ${GH_TOKEN:-} ]] && [[ $(gh pr list --repo "$repo_name" --head "$branch" --state open --json number --jq length) != 0 ]]; then
  echo "open PR already exists: $branch"
  exit 0
fi
if [[ $dry_run == true ]]; then
  echo "dry run: would sync $tag into $branch"
  exit 0
fi
[[ -n ${GH_TOKEN:-} ]] || { echo 'XTEMPORAL_BOT_TOKEN is required for non-dry-run sync' >&2; exit 1; }

snapshot=$(git rev-parse refs/remotes/origin/upstream-snapshots)
main=$(git rev-parse refs/remotes/origin/main)
if [[ $(git log -1 --format=%s "$snapshot") == "Import upstream temporalio/temporal $tag" ]]; then
  echo "reusing existing snapshot for $tag"
  new_snapshot=$snapshot
else
  git fetch --no-tags "${SYNC_UPSTREAM_SOURCE:-https://github.com/temporalio/temporal.git}" "refs/tags/$tag:refs/upstream/$tag"
fi
tmp=$(mktemp -d)
cleanup() {
  git worktree remove --force "$tmp/rewrite" 2>/dev/null || true
  git worktree remove --force "$tmp/merge" 2>/dev/null || true
  rm -rf "$tmp"
}
trap cleanup EXIT
if [[ -z ${new_snapshot:-} ]]; then
  "${SYNC_REWRITE_TOOL:-$script_dir/rewrite-upstream.sh}" "$tag" "$tmp/rewrite"
  git -C "$tmp/rewrite" add -A
  tree=$(git -C "$tmp/rewrite" write-tree)
  new_snapshot=$(printf 'Import upstream temporalio/temporal %s\n' "$tag" | git commit-tree "$tree" -p "$snapshot")
  git worktree remove --force "$tmp/rewrite"
  git push origin "$new_snapshot:refs/heads/upstream-snapshots"
fi
git worktree add --detach "$tmp/merge" "$main"
git -C "$tmp/merge" switch -c "$branch"
git -C "$tmp/merge" config merge.ours.driver true
if ! git -C "$tmp/merge" merge --no-ff -m "Merge upstream temporalio/temporal $tag" "$new_snapshot"; then
  files=$(git -C "$tmp/merge" diff --name-only --diff-filter=U)
  git -C "$tmp/merge" merge --abort
  gh label create upstream-sync-conflict --repo "$repo_name" --color D93F0B --description 'Upstream snapshot merge needs resolution' --force
  body=$(printf 'The %s snapshot was pushed to upstream-snapshots, but its merge into main conflicts.\n\nConflicted files:\n```\n%s\n```\n\nReproduce:\n```sh\ngit fetch origin main upstream-snapshots\ngit switch -c %s origin/main\ngit config merge.ours.driver true\ngit merge --no-ff origin/upstream-snapshots\n```\n' "$tag" "$files" "$branch")
  gh issue create --repo "$repo_name" --title "Upstream sync conflict: $tag" --body "$body" --label upstream-sync-conflict
  exit 1
fi
git -C "$tmp/merge" push origin "HEAD:refs/heads/$branch"
gh label create upstream-sync --repo "$repo_name" --color 0E8A16 --description 'Upstream release sync' --force
gh label create full-ci --repo "$repo_name" --color 5319E7 --description 'Run full CI' --force
pr_body=$(printf 'Imports the rewritten upstream %s snapshot and merges it with fork changes.\n\nUpstream release: https://github.com/temporalio/temporal/releases/tag/%s\n' "$tag" "$tag")
pr=$(gh pr create --repo "$repo_name" --base main --head "$branch" --title "Sync upstream $tag" --body "$pr_body" --label upstream-sync --label full-ci)
echo "$pr"
if [[ ${AUTO_MERGE_SYNC:-false} == true ]]; then
  gh pr merge "$pr" --auto --merge --repo "$repo_name"
fi
