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
printf 'base\n' > "$repo/extra.txt"
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
#!/usr/bin/env python3
import json
import os
import pathlib
import sys

args = sys.argv[1:]
root = pathlib.Path(os.environ["GH_STUB_DIR"])
state_path = root / "issues.json"
issues = json.loads(state_path.read_text()) if state_path.exists() else []
if args[:2] == ["pr", "list"]:
    print(0)
elif args[:2] == ["pr", "create"]:
    print("https://example.com/pr/1")
elif args[:2] == ["label", "create"]:
    pass
elif args[:2] == ["issue", "list"]:
    print(json.dumps([{"number": issue["number"], "title": issue["title"]} for issue in issues]))
elif args[:2] == ["issue", "view"]:
    issue = next(issue for issue in issues if str(issue["number"]) == args[2])
    print(json.dumps({"body": issue["body"], "comments": [{"body": body} for body in issue["comments"]]}))
elif args[:2] == ["issue", "create"]:
    issue = {"number": len(issues) + 1, "title": args[args.index("--title") + 1], "body": args[args.index("--body") + 1], "comments": []}
    issues.append(issue)
    state_path.write_text(json.dumps(issues))
    print(f"https://example.com/issues/{issue['number']}")
elif args[:2] == ["issue", "comment"]:
    number = int(args[2].rsplit("/", 1)[-1])
    issue = next(issue for issue in issues if issue["number"] == number)
    issue["comments"].append(args[args.index("--body") + 1])
    state_path.write_text(json.dumps(issues))
else:
    raise SystemExit(f"unexpected gh invocation: {args}")
GH
cat > "$tmp/rewrite.sh" <<'REWRITE'
#!/usr/bin/env bash
set -euo pipefail
git worktree add --detach "$2" "refs/upstream/$1" >/dev/null
REWRITE
chmod +x "$tmp/bin/gh" "$tmp/rewrite.sh"
export GH_STUB_DIR="$tmp"
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

printf 'upstream next\n' > "$upstream/version.txt"
printf 'upstream extra\n' > "$upstream/extra.txt"
git -C "$upstream" add -A
git -C "$upstream" commit -qm 'next upstream'
git -C "$upstream" tag v1.32.3
printf 'fork version\n' > "$repo/version.txt"
git -C "$repo" add -A
git -C "$repo" commit -qm 'fork version'
git -C "$repo" push -q origin main
if output=$(PATH="$tmp/bin:$PATH" GH_TOKEN=dummy SYNC_UPSTREAM_SOURCE="$upstream" SYNC_REWRITE_TOOL="$tmp/rewrite.sh" bash scripts/xtemporal/sync.sh --tag v1.32.3 2>&1); then
  echo 'expected conflict' >&2
  exit 1
fi
[[ $(python3 -c 'import json,sys; print(len(json.load(open(sys.argv[1]))))' "$tmp/issues.json") == 1 ]]
[[ $(python3 -c 'import json,sys; print(len(json.load(open(sys.argv[1]))[0]["comments"]))' "$tmp/issues.json") == 0 ]]
python3 - "$tmp/issues.json" <<'PY'
import json
import sys
issue = json.load(open(sys.argv[1]))[0]
body = issue["body"]
for expected in ("Snapshot commit:", "Main commit:", "version.txt", "git merge --no-ff", "xtemporal:", "fork-owned.txt", "README.md", "CONTRIBUTING.md", "required-checks", "go build ./...", "go.temporal.io/server", "Never squash"):
    assert expected in body, expected
PY
if output=$(PATH="$tmp/bin:$PATH" GH_TOKEN=dummy bash scripts/xtemporal/sync.sh --tag v1.32.3 2>&1); then
  echo 'expected repeated conflict' >&2
  exit 1
fi
[[ $output == *'conflict already tracked in #1'* ]]
[[ $(python3 -c 'import json,sys; x=json.load(open(sys.argv[1])); print(len(x), len(x[0]["comments"]))' "$tmp/issues.json") == '1 0' ]]

printf 'fork extra\n' > "$repo/extra.txt"
git -C "$repo" add -A
git -C "$repo" commit -qm 'fork extra'
git -C "$repo" push -q origin main
if output=$(PATH="$tmp/bin:$PATH" GH_TOKEN=dummy bash scripts/xtemporal/sync.sh --tag v1.32.3 2>&1); then
  echo 'expected changed conflict' >&2
  exit 1
fi
[[ $output == *'conflict already tracked in #1'* ]]
[[ $(python3 -c 'import json,sys; x=json.load(open(sys.argv[1])); print(len(x), len(x[0]["comments"]), "extra.txt" in x[0]["comments"][0])' "$tmp/issues.json") == '1 1 True' ]]
if output=$(PATH="$tmp/bin:$PATH" GH_TOKEN=dummy bash scripts/xtemporal/sync.sh --tag v1.32.3 2>&1); then
  echo 'expected repeated changed conflict' >&2
  exit 1
fi
[[ $(python3 -c 'import json,sys; print(len(json.load(open(sys.argv[1]))[0]["comments"]))' "$tmp/issues.json") == 1 ]]

printf 'upstream newest\n' > "$upstream/version.txt"
git -C "$upstream" add -A
git -C "$upstream" commit -qm 'newest upstream'
git -C "$upstream" tag v1.32.4
if output=$(PATH="$tmp/bin:$PATH" GH_TOKEN=dummy SYNC_CONFLICT_AGENT_MENTION=@codex SYNC_UPSTREAM_SOURCE="$upstream" SYNC_REWRITE_TOOL="$tmp/rewrite.sh" bash scripts/xtemporal/sync.sh --tag v1.32.4 2>&1); then
  echo 'expected second conflict' >&2
  exit 1
fi
[[ $(python3 -c 'import json,sys; x=json.load(open(sys.argv[1])); print(len(x), x[1]["comments"])' "$tmp/issues.json") == "2 ['@codex please resolve this upstream sync conflict following the instructions in this issue.']" ]]
if output=$(PATH="$tmp/bin:$PATH" GH_TOKEN=dummy SYNC_CONFLICT_AGENT_MENTION=@codex bash scripts/xtemporal/sync.sh --tag v1.32.4 2>&1); then
  echo 'expected repeated second conflict' >&2
  exit 1
fi
[[ $(python3 -c 'import json,sys; print(len(json.load(open(sys.argv[1]))[1]["comments"]))' "$tmp/issues.json") == 1 ]]
python3 - "$tmp/issues.json" <<'PY'
import json
import sys
issues = json.load(open(sys.argv[1]))
assert len(issues) == 2
assert issues[0]["title"] == "Upstream sync conflict: v1.32.3"
assert len(issues[0]["comments"]) == 1
assert "extra.txt" in issues[0]["comments"][0]
assert issues[1]["title"] == "Upstream sync conflict: v1.32.4"
assert issues[1]["comments"] == ["@codex please resolve this upstream sync conflict following the instructions in this issue."]
PY
echo 'sync: PASS (semver, idempotency, ours merge, conflict dedupe, agent mention)'
