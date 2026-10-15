# ParadeDB visibility

The `paradedb` SQL plugin stores visibility data in ParadeDB while the default store uses `postgres12_pgx`. It targets `paradedb/paradedb:v0.26.0-pg17` (PostgreSQL 17, `pg_search` 0.26.0). Use a fresh visibility database: its generated `TEXT` search columns differ from the PostgreSQL visibility schema. The plugin always uses the unified query converter, including when `system.visibilityEnableUnifiedQueryConverter` is disabled.

## Local databases and schema

From the repository root:

```sh
docker compose -f develop/docker-compose/paradedb/docker-compose.yml -p xtpdb up -d --wait
go build -o /tmp/temporal-sql-tool ./cmd/tools/sql

/tmp/temporal-sql-tool --endpoint 127.0.0.1 --port 55433 --user temporal --password temporal --database temporal --plugin postgres12_pgx create-database
/tmp/temporal-sql-tool --endpoint 127.0.0.1 --port 55433 --user temporal --password temporal --database temporal --plugin postgres12_pgx setup-schema --schema-name postgresql/v12/temporal --version 1.19

/tmp/temporal-sql-tool --endpoint 127.0.0.1 --port 55432 --user temporal --password temporal --database temporal_visibility --plugin paradedb create-database
/tmp/temporal-sql-tool --endpoint 127.0.0.1 --port 55432 --user temporal --password temporal --database temporal_visibility --plugin paradedb setup-schema --schema-name paradedb/visibility --version 1.14
```

For an empty visibility database with version tables at 0.0, `update-schema --schema-name paradedb/visibility` applies the versioned migrations through 1.14. Do not run it after the full 1.14 `setup-schema` command. The Compose file creates a superuser `temporal` test role on first initialization so tests can terminate ParadeDB background sessions while dropping temporary databases; use a limited role for deployments.

Configure the two stores separately:

```yaml
persistence:
  defaultStore: postgres-default
  visibilityStore: paradedb-visibility
  numHistoryShards: 4
  datastores:
    postgres-default:
      sql:
        pluginName: postgres12_pgx
        databaseName: temporal
        connectAddr: 127.0.0.1:55433
        connectProtocol: tcp
        user: temporal
        password: temporal
    paradedb-visibility:
      sql:
        pluginName: paradedb
        databaseName: temporal_visibility
        connectAddr: 127.0.0.1:55432
        connectProtocol: tcp
        user: temporal
        password: temporal
```

The `temporal-server` and `temporal-sql-tool` binaries in the xtemporal image include the plugin.

## Tests

```sh
go test -tags test_dep ./x/paradedb
PARADEDB_TEST=1 POSTGRES_PORT=55432 go test -tags test_dep ./common/persistence/tests -run '^TestParadeDBVisibilityPersistence$' -count=1
POSTGRES_PORT=55432 go test -tags test_dep,integration ./tests -run '^TestWorkflowVisibilityTestSuite$' -count=1 -persistenceType=sql -persistenceDriver=paradedb
POSTGRES_PORT=55432 go test -tags test_dep,integration ./tests -run '^TestAdvancedVisibilitySuite' -parallel 1 -timeout 65m -count=1 -persistenceType=sql -persistenceDriver=paradedb
```

The test harness uses `postgres12_pgx` for its default store and `paradedb` for visibility. The Compose example sets `max_connections=500`; CI runs the complete advanced visibility suites serially to bound connection and memory use. CI also runs the full functional package in three shards against ParadeDB on each pull request and push to main. The advanced suite skips `TestListWorkflow_OrderBy` for all SQL visibility stores because that test covers Elasticsearch-only ordering. ParadeDB's `pg_search` source is AGPL-3.0; review distribution obligations before bundling ParadeDB or `pg_search` in a product image.
