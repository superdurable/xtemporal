# Temporal Server GitHub runners on OrbStack

This directory provisions a repository-scoped Linux/amd64 GitHub Actions runner pool for `superdurable/temporal-server`. Each runner has its own Docker daemon, Docker data volume, and Actions work volume.

The pool contains two `linux-dind-heavy` runners for PostgreSQL and unit-test jobs, plus one `linux-dind-light` runner for the test gate. These are the labels used by `.github/workflows/run-tests.yml`.

## Start the pool

GitHub CLI must be authenticated with repository administration access for `superdurable/temporal-server`.

```bash
cd github-runners
./start-linux-runners.sh
```

The script starts OrbStack, selects its Docker context, builds the runner image if necessary, and registers three runners. Containers restart automatically with OrbStack.

## Operations

```bash
cd github-runners/linux-docker

./scale.sh status
./scale.sh logs 1
./scale.sh stop
./scale.sh reset
```

`reset` removes only the `dex-engine-linux-dind*` registrations and containers, then creates fresh registrations. Docker and work volumes are preserved. Registration tokens are short-lived and intentionally ignored by Git.
