#!/usr/bin/env bash
set -euo pipefail

script_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
repo="$tmp/repo"
mkdir -p "$repo" "$tmp/bin"
git -C "$repo" init -q -b main
git -C "$repo" config user.name 'Release test'
git -C "$repo" config user.email 'release@example.com'
printf 'base\n' > "$repo/version.txt"
git -C "$repo" add version.txt
git -C "$repo" commit -qm 'Import upstream temporalio/temporal v1.32.1'
git -C "$repo" tag v1.32.1

git -C "$repo" switch -qc pr-one
printf 'one\n' >> "$repo/version.txt"
git -C "$repo" commit -qam 'Add feature'
git -C "$repo" switch -q main
git -C "$repo" merge -q --no-ff -m 'Merge pull request #42 from superdurable/pr-one' pr-one
git -C "$repo" switch -qc pr-two
printf 'two\n' >> "$repo/version.txt"
git -C "$repo" commit -qam 'Fix bug'
git -C "$repo" switch -q main
git -C "$repo" merge -q --no-ff -m "Merge branch 'main' into pr-two" pr-two
git -C "$repo" switch -qc pr-three
printf 'three\n' >> "$repo/version.txt"
git -C "$repo" commit -qam 'Improve docs'
git -C "$repo" switch -q main
git -C "$repo" merge -q --no-ff -m 'Merge pull request #43 from superdurable/pr-three' pr-three
git -C "$repo" tag v1.32.2
git -C "$repo" switch -qc pr-four
printf 'four\n' >> "$repo/version.txt"
git -C "$repo" commit -qam 'Add plugin'
git -C "$repo" switch -q main
git -C "$repo" merge -q --no-ff -m 'Merge pull request #44 from superdurable/pr-four' pr-four
git -C "$repo" tag v1.32.2-x.1

cat > "$tmp/bin/gh" <<'GH'
#!/usr/bin/env bash
[[ $1 == pr && $2 == view && $4 == --json && $5 == title && $6 == --jq && $7 == .title ]]
case $3 in
  42) echo 'Add useful feature' ;;
  43) echo 'Improve documentation' ;;
  44) echo 'Add experimental plugin' ;;
  *) exit 1 ;;
esac
GH
chmod +x "$tmp/bin/gh"
cd "$repo"
output=$(PATH="$tmp/bin:$PATH" bash "$script_dir/release-notes.sh" v1.32.2)
[[ $output == *'https://github.com/temporalio/temporal/releases/tag/v1.32.2'* ]]
[[ $output == *'- Add useful feature (#42)'* ]]
[[ $output == *'- Improve documentation (#43)'* ]]
[[ $output != *'Merge branch'* && $output != *'Import upstream'* && $output != *'Fix bug'* ]]
hotfix=$(PATH="$tmp/bin:$PATH" bash "$script_dir/release-notes.sh" v1.32.2-x.1)
[[ $hotfix == *'https://github.com/temporalio/temporal/releases/tag/v1.32.2'* && $hotfix != *'tag/v1.32.2-x.1'* ]]
[[ $hotfix == *'- Add experimental plugin (#44)'* && $hotfix != *'(#43)'* ]]
if bash "$script_dir/release-notes.sh" v1.32.2-rc.1 >/dev/null 2>&1; then exit 1; fi
first=$(PATH="$tmp/bin:$PATH" bash "$script_dir/release-notes.sh" v1.32.1)
[[ $first == *'- No merged pull requests.'* ]]
echo 'release-notes: PASS (upstream link, PR titles, merge filtering, first release, hotfix release)'
