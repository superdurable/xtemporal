#!/usr/bin/env bash
set -euo pipefail

script_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
repo="$tmp/repo"
upstream="$tmp/upstream"
origin="$tmp/origin.git"
mkdir -p "$repo/scripts/xtemporal" "$tmp/bin"
cp "$script_dir/sync.sh" "$repo/scripts/xtemporal/sync.sh"
cp "$script_dir/fork-owned.txt" "$repo/scripts/xtemporal/fork-owned.txt"
cp "$script_dir/../../.gitattributes" "$repo/.gitattributes"
while IFS= read -r path; do
  [[ -z $path ]] && continue
  grep -Fxq "$path merge=ours" "$repo/.gitattributes"
done < "$repo/scripts/xtemporal/fork-owned.txt"
printf 'base\n' > "$repo/README.md"
printf 'base\n' > "$repo/CONTRIBUTING.md"
printf 'base\n' > "$repo/version.txt"
git -C "$repo" init -q -b main
git -C "$repo" config user.name 'Sync test'
git -C "$repo" config user.email 'sync@example.com'
git -C "$repo" add -A
git -C "$repo" commit -qm base
git -C "$repo" tag v1.32.1
git init -q --bare "$origin"
git -C "$repo" remote add origin "$origin"
git -C "$repo" push -q origin main:main main:upstream-snapshots --tags
git -C "$repo" fetch -q origin main upstream-snapshots
cp -R "$repo" "$upstream"
git -C "$upstream" remote remove origin
printf 'upstream\n' > "$upstream/README.md"
printf 'upstream\n' > "$upstream/CONTRIBUTING.md"
printf 'new\n' > "$upstream/version.txt"
git -C "$upstream" add -A
git -C "$upstream" commit -qm upstream
git -C "$upstream" tag v1.32.2
printf 'ours\n' > "$repo/README.md"
printf 'ours\n' > "$repo/CONTRIBUTING.md"
git -C "$repo" add -A
git -C "$repo" commit -qm overlay
git -C "$repo" push -q origin main
cat > "$tmp/bin/gh" <<'GH'
#!/usr/bin/env bash
if [[ $1 == pr && $2 == list ]]; then echo 0; exit; fi
if [[ $1 == pr && $2 == create ]]; then echo https://example.com/pr/1; exit; fi
if [[ $1 == label && $2 == create ]]; then exit; fi
echo "unexpected gh invocation: $*" >&2
exit 1
GH
cat > "$tmp/rewrite.sh" <<'REWRITE'
#!/usr/bin/env bash
set -euo pipefail
git worktree add --detach "$2" "refs/upstream/$1" >/dev/null
REWRITE
chmod +x "$tmp/bin/gh" "$tmp/rewrite.sh"
cd "$repo"
output=$(bash scripts/xtemporal/sync.sh --tag v1.32.1 --dry-run)
[[ $output == *'already up to date'* ]]
output=$(bash scripts/xtemporal/sync.sh --tag v1.31.4 --dry-run)
[[ $output == *'refusing older release'* ]]
PATH="$tmp/bin:$PATH" GH_TOKEN=dummy SYNC_UPSTREAM_SOURCE="$upstream" SYNC_REWRITE_TOOL="$tmp/rewrite.sh" bash scripts/xtemporal/sync.sh --tag v1.32.2
snapshot=$(git -C "$origin" rev-parse refs/heads/upstream-snapshots)
merge=$(git -C "$origin" rev-parse refs/heads/sync/v1.32.2)
parents=$(git -C "$origin" rev-list --parents -n1 "$merge")
[[ $parents == "$merge "*" $snapshot" ]]
[[ $(git -C "$origin" show "$merge:README.md") == ours ]]
[[ $(git -C "$origin" show "$merge:CONTRIBUTING.md") == ours ]]
[[ $(git -C "$origin" show "$merge:version.txt") == new ]]
output=$(PATH="$tmp/bin:$PATH" GH_TOKEN=dummy bash scripts/xtemporal/sync.sh --tag v1.32.2)
[[ $output == *'already exists'* ]]
echo 'sync: PASS (semver, idempotency, ours merge, snapshot second parent)'
