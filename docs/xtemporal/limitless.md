# Aurora PostgreSQL Limitless default store

**Semantics validated on a real `16.13-limitless` cluster; performance not yet benchmarked.** Row-lock fencing, read-after-write across routers, single-shard history transactions, distributed deadlock detection, and fencing across a shard group reboot passed. Only fresh clusters are supported; there is no migration path from vanilla PostgreSQL. Temporal-level end-to-end testing is still pending.

The `postgres12_limitless` SQL plugin is for the **default store only**. Configure visibility as a separate store using `postgres12_pgx` on PostgreSQL or another supported visibility backend. The Limitless plugin rejects visibility-store connections.

## How the schema is distributed

| Table group | Limitless table type and shard key |
| --- | --- |
| Every history table | Sharded and collocated on `shard_id` |
| Matching task tables | Sharded on `range_hash` |
| `task_queue_user_data`, `build_id_to_task_queue` | Sharded and collocated on `task_queue_name`; `namespace_id` is `BYTEA`, which Limitless does not allow as a shard key |
| Queue v2 | Sharded on `(queue_type, queue_name)` |
| `cluster_membership` | Sharded on `membership_partition` |
| `namespaces`, `namespace_metadata`, `cluster_metadata_info`, `nexus_*`, queue v1 | Reference tables |

Collocation keeps transactions for one Temporal history shard on one Limitless shard. The plugin assigns `buffered_events.id` monotonically within each workflow under the execution row lock: Limitless sequences are only monotonic within a router session, so a shared sequence cannot provide this ordering across routers.

## Provision a fresh cluster

Use a private DB subnet group and security group that allow the Temporal hosts to reach the shard group endpoint. Check [AWS's Limitless requirements and unsupported features](https://docs.aws.amazon.com/AmazonRDS/latest/AuroraUserGuide/limitless-reqs-limits.html) and [cluster creation guidance](https://docs.aws.amazon.com/AmazonRDS/latest/AuroraUserGuide/limitless-cluster.html) for the engine version available in your Region. The provisioning checklist is:

- Choose a `16.x-limitless` Aurora PostgreSQL engine version, `aurora-iopt1` storage, and `--cluster-scalability-type limitless`.
- Enable Performance Insights with at least 31 days of retention, Enhanced Monitoring with a monitoring role, and PostgreSQL CloudWatch log export.
- Create a DB shard group. Its `--max-acu` determines the initial number of routers and shards; raising that limit later does not add nodes. Size it for the intended topology.
- Use `--compute-redundancy 1` or higher for production. Redundant compute adds replicas and multiplies compute cost; check the capacity and bill before provisioning.
- Keep the shard group non-public. The tested public shard group was for validation only. At the tested 64-ACU size, each router reported `max_connections = 5000`; verify the limit on your own routers.

For example, after providing private networking, credentials, and an Enhanced Monitoring role:

```sh
aws rds create-db-cluster \
  --db-cluster-identifier temporal-limitless \
  --engine aurora-postgresql --engine-version 16.13-limitless \
  --storage-type aurora-iopt1 --cluster-scalability-type limitless \
  --master-username "$DB_USER" --master-user-password "$DB_PASSWORD" \
  --db-subnet-group-name "$DB_SUBNET_GROUP" \
  --vpc-security-group-ids "$DB_SECURITY_GROUP" \
  --enable-performance-insights --performance-insights-retention-period 31 \
  --monitoring-interval 5 --monitoring-role-arn "$MONITORING_ROLE_ARN" \
  --enable-cloudwatch-logs-exports postgresql

aws rds create-db-shard-group \
  --db-shard-group-identifier temporal-limitless-sg \
  --db-cluster-identifier temporal-limitless \
  --max-acu 64 --min-acu 16 --compute-redundancy 1
```

Select an available `16.x-limitless` version for your Region rather than assuming the example version is offered there. Limitless does not support RDS Proxy, AWS Secrets Manager integration, AWS Backup, Aurora Global Database, cluster cloning, or read replicas. For cross-Region disaster recovery, use Temporal's own replication between clusters; plan backups with supported Aurora cluster snapshots and point-in-time recovery.

## Set up the default-store schema

**Command status: validated on a real cluster: pending.** The schema lives in `schema/limitless/temporal` and is embedded in `temporal-sql-tool` as `--schema-name limitless/temporal`; on PostgreSQL 16 the versioned path was verified up to the first `CALL`, which needs Limitless. Use the shard group endpoint on port 5432 and the built-in `postgres_limitless` database, or create a fresh database first. Export `SQL_PASSWORD` in the shell or supply another supported SQL tool credential source.

From the repository root, build the tool, then choose **one** schema initialization path:

```sh
go build -o /tmp/temporal-sql-tool ./cmd/tools/sql

/tmp/temporal-sql-tool --endpoint "$LIMITLESS_ENDPOINT" --port 5432 \
  --user "$DB_USER" --database postgres_limitless \
  --connect-attributes 'sslmode=require' --plugin postgres12_limitless \
  setup-schema --schema-name limitless/temporal --version 1.19
```

Or initialize only the schema version tables at 0.0 and apply the versioned schema:

```sh
/tmp/temporal-sql-tool --endpoint "$LIMITLESS_ENDPOINT" --port 5432 \
  --user "$DB_USER" --database postgres_limitless \
  --connect-attributes 'sslmode=require' --plugin postgres12_limitless \
  setup-schema -v 0.0

/tmp/temporal-sql-tool --endpoint "$LIMITLESS_ENDPOINT" --port 5432 \
  --user "$DB_USER" --database postgres_limitless \
  --connect-attributes 'sslmode=require' --plugin postgres12_limitless \
  update-schema --schema-name limitless/temporal
```

Do not run `update-schema` after the full 1.19 `setup-schema`. The Limitless schema converts ordinary tables into sharded or reference tables with `CALL rds_aurora.limitless_alter_table_type_*` procedures.

Configure the stores separately (replace the example addresses, user, and password with your deployment's values):

```yaml
persistence:
  defaultStore: limitless-default
  visibilityStore: postgres-visibility
  numHistoryShards: 1024
  datastores:
    limitless-default:
      sql:
        pluginName: postgres12_limitless
        databaseName: postgres_limitless
        connectAddr: limitless-sg.example.amazonaws.com:5432
        connectProtocol: tcp
        user: temporal
        password: example-password
        connectAttributes:
          sslmode: require
        maxConns: 20
        maxIdleConns: 20
        maxConnLifetime: "1h"
    postgres-visibility:
      sql:
        pluginName: postgres12_pgx
        databaseName: temporal_visibility
        connectAddr: postgres-visibility.example.amazonaws.com:5432
        connectProtocol: tcp
        user: temporal
        password: example-password
        connectAttributes:
          sslmode: require
        maxConns: 10
        maxIdleConns: 10
        maxConnLifetime: "1h"
```

Choose connection limits for the total number of Temporal processes and routers. The shard group DNS returned one router per lookup during validation, so long-lived connection pools can concentrate traffic on one router. `maxConnLifetime` periodically recycles connections, allowing new DNS lookups; monitor the resulting distribution. `numHistoryShards` is fixed for the life of a Temporal cluster and determines the granularity of hashing across Limitless shards. Choose enough at cluster creation, for example at least 1024, for even distribution and future shard splits. A single hot Temporal shard remains bounded by one Limitless shard.

## Operations

Inspect router connection distribution and traffic between subclusters with the [Limitless monitoring views](https://docs.aws.amazon.com/AmazonRDS/latest/AuroraUserGuide/limitless-monitoring-views.html):

```sql
SELECT subcluster_id, count(*) AS sessions
FROM rds_aurora.limitless_stat_activity
WHERE subcluster_type = 'router'
GROUP BY subcluster_id
ORDER BY subcluster_id;

SELECT * FROM rds_aurora.limitless_stat_subclusters;
```

Scale by [splitting a shard](https://docs.aws.amazon.com/AmazonRDS/latest/AuroraUserGuide/limitless-shard-split.html) with `rds_aurora.limitless_split_shard` and [adding a router](https://docs.aws.amazon.com/AmazonRDS/latest/AuroraUserGuide/limitless-add-router.html) with `rds_aurora.limitless_add_router`. These SQL functions start asynchronous jobs; track each job before starting another. Shards cannot be merged. Raising `--max-acu` increases the capacity available to existing nodes but does not itself add routers or shards.

During shard or router failover, in-flight transactions abort and Temporal retries. A client can also receive an error after a commit succeeded; Temporal's conditional writes and version checks handle that ambiguous outcome. Keep connection pools able to reconnect and watch for persistent errors after recovery.

Limitless supports IAM database authentication. The server's `sql.passwordCommand` can run a token helper: its `command`, `args`, and optional `timeout` fields supply a password from stdout. It is mutually exclusive with `sql.password`. Set `maxConnLifetime` so connections recycle before an IAM token expires. Keep the command and credentials private to the server environment.

> **WARNING — diagnostic sessions can silently lose writes.** On the tested `16.13-limitless` cluster, if `rds_aurora.limitless_explain_options` includes `shard_plans`, running `EXPLAIN` for a key and then writing that key in the **same session** can report `INSERT 0 1` while storing no row. Temporal does not set this option. Run such diagnostics only in a separate read-only session; do not reuse it for writes.

Minimal reproduction from the real-cluster verification, using a disposable test database with a two-column sharded `shards` table (execute all statements in one session):

```sql
CREATE TABLE shards (shard_id integer PRIMARY KEY, range_id bigint NOT NULL);
CALL rds_aurora.limitless_alter_table_type_sharded('public.shards', ARRAY['shard_id']);
SET rds_aurora.limitless_explain_options = 'shard_plans, single_shard_optimization';
EXPLAIN SELECT * FROM shards WHERE shard_id = 123456;
INSERT INTO shards(shard_id,range_id) VALUES (123456,5);
SELECT * FROM shards WHERE shard_id = 123456;
```

The insert reports `INSERT 0 1`, but the final query returns no row. The original verification used `K` for the key and found the same result with `shard_plans` alone; `EXPLAIN` did not use `ANALYZE`. Do not run this reproduction against Temporal's production `shards` table.

## Tests

There is no Limitless CI: AWS does not provide a local or Docker Limitless service. For Aurora-mode persistence and functional tests, set `LIMITLESS_ADMIN_DATABASE` to an existing database on the shard group endpoint (typically `postgres_limitless`). The harness creates a separate temporary Limitless database for each test cluster or suite and drops it at teardown. Set `LIMITLESS_VISIBILITY_*` to a separate vanilla PostgreSQL endpoint. The SQL-tool verification script uses the coordinator-provided dedicated database in `LIMITLESS_SQL_TOOL_DATABASE`.

The pure-Go plugin tests run with normal unit tests and cover visibility rejection, transaction wrapping, buffered-event ID SQL, and schema-statement rewriting:

```sh
go test -tags test_dep ./x/limitless
```

Static schema tests run with normal unit tests too. They check the table type mapping, shard keys in every PK/UNIQUE, collocation anchors, index ordering, and drift against `schema/postgresql/v12`, so an upstream v12 change fails until it is mirrored:

```sh
go test -tags test_dep ./schema/limitless
```

`x/limitless` (buffered-event inserts) and `schema/limitless` (schema load; on Limitless it also asserts table types and collocation groups) have DSN-gated database tests. Set `XTEMPORAL_LIMITLESS_TEST_DSN` to a disposable test database DSN to run them. The on-demand end-to-end script `scripts/xtemporal/limitless-e2e.sh` is **planned**, not available yet. Real-cluster schema setup and Temporal-level end-to-end tests remain pending.
