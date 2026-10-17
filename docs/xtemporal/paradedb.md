# ParadeDB Dex visibility

The [design of record](paradedb-dex-store.md) defines a fresh-only v2.0 visibility schema for ParadeDB 0.26.1. The `paradedb` SQL plugin is registered in `temporal-sql-tool` for schema administration only. Server configuration with `sql.pluginName: paradedb` fails at startup. The dedicated `customDatastore.name: paradedb-dex` store is introduced in P2–P5; until then, do not configure ParadeDB as a runtime visibility store.

From the repository root, start the local ParadeDB service and build the SQL tool:

```sh
docker compose -f develop/docker-compose/paradedb/docker-compose.yml up -d --wait paradedb
go build -o /tmp/temporal-sql-tool ./cmd/tools/sql
```

Create a fresh visibility database, then choose one of the two equivalent schema paths:

```sh
/tmp/temporal-sql-tool --endpoint 127.0.0.1 --port 55432 --user temporal --password temporal --database visibility_v2 --plugin paradedb create-database
/tmp/temporal-sql-tool --endpoint 127.0.0.1 --port 55432 --user temporal --password temporal --database visibility_v2 --plugin paradedb setup-schema --schema-name paradedb/visibility --version 2.0
```

Alternatively, run `setup-schema --version 0.0` without a schema name on another fresh database, followed by `update-schema --schema-name paradedb/visibility`. The CI schema job compares their full catalogs and checks both versions. There is no v1.x migration. The [schema decision](paradedb-schema-decision.md) and [benchmark record](paradedb-benchmark.md) provide earlier context; benchmark results and their historical schemas are not deployment instructions.
