# Maintaining xtemporal

xtemporal tracks upstream [Temporal Server](https://github.com/temporalio/temporal) while keeping fork specific extensions reviewable. Some paths below are reserved for planned work and may not exist until their respective changes land.

## Extension layout

| Path | Purpose |
| --- | --- |
| `x/` | Fork specific Go packages, including the planned ParadeDB visibility plugin. |
| `schema/paradedb/` | Planned ParadeDB visibility schema. |
| `scripts/xtemporal/` | Upstream rewrite and sync tooling. |
| `docs/xtemporal/` | Fork maintenance and extension documentation. |

When an extension must touch upstream owned code, keep the hook small and mark it with an `xtemporal:` comment so it can be found during a sync. The planned ParadeDB plugin needs hook points for registration, visibility plugin validation, and tests. It has not shipped yet.

## Upstream sync

`upstream-snapshots` contains successive upstream release trees rewritten to use the `github.com/superdurable/xtemporal` module path. It starts at upstream `v1.32.1`. The rewrite changes references to `go.temporal.io/server`, while leaving `go.temporal.io/api` and `go.temporal.io/sdk` intact. The `main` branch carries the fork's extensions and merges each new rewritten snapshot. The previous snapshot is the three way merge base, so conflicts should be concentrated in files changed by both upstream and xtemporal.

The planned daily sync selects upstream's **Latest** release, prepares its rewritten snapshot, and opens a sync pull request when a newer release is available. A failed rewrite or unresolved merge conflict must be fixed before release; the sync does not publish a partial update.

To resolve a sync conflict:

1. Fetch the latest `main` and `upstream-snapshots`, then work on the sync branch.
2. Inspect each conflict against the previous snapshot, the new snapshot, and the `xtemporal:` hook points. Preserve upstream behavior and the intended fork extension.
3. Resolve the files, run the relevant build and tests, and update the sync pull request. Check the extension's functional tests when a hook point changed.
4. Merge the sync pull request only after the conflict resolution and required checks pass.

## Release flow

xtemporal uses upstream version numbers. After a sync pull request is merged and validated, the release process creates an immutable Go tag such as `v1.32.1` and publishes `superdurable/xtemporal:1.32.1` with `temporal-server` and `temporal-sql-tool`. The daily sync and release automation are planned; consult the repository workflows for their current status.

For a fork hotfix on the same upstream version, publish an image tag such as `1.32.1-x.1` and move the `1.32.1` image tag to that build. Do not move the Go tag. Go consumers use a pseudo-version for the hotfix commit.
