# Upstream snapshot rewrite

`rewrite-upstream.sh <upstream-tag> <output-dir>` checks out a fetched `refs/upstream/<tag>` into a new Git worktree, vendors the pinned `auto-scaled-workers` module, changes server import paths and generated descriptor lengths, applies `upstream-denylist.txt`, tidies the module, and checks for the old server module. It leaves the output worktree available for inspection and committing. The tool requires Git, Go, Perl, Python 3, and ripgrep.

Run the fixture test with `scripts/xtemporal/tests/rewrite-fixture.sh`.

To check reproducibility for v1.32.1, run the tool twice in fresh output directories, then run `git -C <output-dir> add -A && git -C <output-dir> write-tree` in each. Both tree hashes must equal `git rev-parse upstream-snapshots^{tree}`. Each output is a Git worktree; remove it with `git worktree remove <output-dir>` when finished.
