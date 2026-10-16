# Maintaining xtemporal

xtemporal tracks upstream [Temporal Server](https://github.com/temporalio/temporal) while keeping fork specific extensions reviewable.

## Extension layout

| Path | Purpose |
| --- | --- |
| `x/` | Fork specific extensions: `x/limitless` (Aurora Limitless default store), `x/limitless-verify` (its on-demand end-to-end script and verification tools), and `x/paradedb` (ParadeDB visibility). |
| `schema/limitless/` | Aurora Limitless default-store schema; must mirror `schema/postgresql/v12` (a drift test enforces this). |
| `schema/paradedb/` | ParadeDB visibility schema. |
| `scripts/xtemporal/` | Upstream rewrite, sync, and release tooling. |
| `docs/xtemporal/` | Fork maintenance and extension documentation. |

When an extension must touch upstream owned code, keep the hook small and mark it with an `xtemporal:` comment so it can be found during a sync. When an upstream release changes `schema/postgresql/v12/temporal`, mirror the change in `schema/limitless/temporal` (including a conversion `CALL` for any new table) and run `x/limitless-verify/limitless-e2e.sh`.

## Upstream sync

`upstream-snapshots` contains successive upstream release trees rewritten to use the `github.com/superdurable/xtemporal` module path. It starts at upstream `v1.32.1`. The rewrite changes references to `go.temporal.io/server`, while leaving `go.temporal.io/api` and `go.temporal.io/sdk` intact. The `main` branch carries the fork's extensions and merges each new rewritten snapshot. The previous snapshot is the three way merge base, so conflicts should be concentrated in files changed by both upstream and xtemporal.

The daily sync (`.github/workflows/upstream-sync.yml`) selects upstream's **Latest** release, prepares its rewritten snapshot, and opens a sync pull request when a newer release is available. A failed rewrite or unresolved merge conflict must be fixed before release; the sync does not publish a partial update.

To resolve a sync conflict:

1. Fetch the latest `main` and `upstream-snapshots`, then work on the sync branch.
2. Inspect each conflict against the previous snapshot, the new snapshot, and the `xtemporal:` hook points. Preserve upstream behavior and the intended fork extension.
3. Resolve the files, run the relevant build and tests, and update the sync pull request. Check the extension's functional tests when a hook point changed.
4. Merge the sync pull request only after the conflict resolution and required checks pass.

## Release flow

xtemporal uses upstream version numbers. After a sync pull request is merged and validated, the release workflow creates an immutable Go tag such as `v1.32.1` and publishes `superdurable/xtemporal:1.32.1` with `temporal-server` and `temporal-sql-tool`.

To release fork changes on the same upstream version, push an annotated tag `vX.Y.Z-x.N` (for example `v1.32.1-x.2`) on `main`. The build workflow publishes image `X.Y.Z-x.N` and creates a GitHub pre-release (semver pre-release); the `X.Y.Z`, minor, and `latest` image tags and the `vX.Y.Z` Go tag do not move. Go consumers pin the `-x.N` version explicitly, because minimal version selection prefers plain `vX.Y.Z` when another module requires it.

## Authentication

- [Revocable JWT API keys](jti-denylist.md)
