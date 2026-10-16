-- DML semantics checks for the Limitless schema. Load schema/limitless/temporal/schema.sql into a scratch
-- database first (on Aurora Limitless, or vanilla PostgreSQL with the CALL statements removed), then run:
--   psql "$DSN" -X -v ON_ERROR_STOP=0 -f develop/limitless/sql/semantics.sql
-- T7 and T8 are expected to fail (UNIQUE and CHECK enforcement on reference tables).
\echo == T1 namespace_metadata seed survived reference conversion
SELECT partition_id, notification_version FROM namespace_metadata;
\echo == T2 activity_info_maps upsert exactly as setKeyInMapQueryTemplate (SET includes shard key)
INSERT INTO activity_info_maps (shard_id, namespace_id, workflow_id, run_id, schedule_id, data, data_encoding) VALUES (1, '\x01', 'wf', '\x02', 7, '\x0a', 'proto3')
ON CONFLICT (shard_id, namespace_id, workflow_id, run_id, schedule_id) DO UPDATE
SET (shard_id, namespace_id, workflow_id, run_id, schedule_id, data, data_encoding) = (excluded.shard_id, excluded.namespace_id, excluded.workflow_id, excluded.run_id, excluded.schedule_id, excluded.data, excluded.data_encoding);
INSERT INTO activity_info_maps (shard_id, namespace_id, workflow_id, run_id, schedule_id, data, data_encoding) VALUES (1, '\x01', 'wf', '\x02', 7, '\x0b', 'proto3')
ON CONFLICT (shard_id, namespace_id, workflow_id, run_id, schedule_id) DO UPDATE
SET (shard_id, namespace_id, workflow_id, run_id, schedule_id, data, data_encoding) = (excluded.shard_id, excluded.namespace_id, excluded.workflow_id, excluded.run_id, excluded.schedule_id, excluded.data, excluded.data_encoding);
\echo == T3 activity_info_maps upsert with SET excluding key columns
INSERT INTO activity_info_maps (shard_id, namespace_id, workflow_id, run_id, schedule_id, data, data_encoding) VALUES (1, '\x01', 'wf', '\x02', 7, '\x0c', 'proto3')
ON CONFLICT (shard_id, namespace_id, workflow_id, run_id, schedule_id) DO UPDATE
SET (data, data_encoding) = (excluded.data, excluded.data_encoding);
SELECT schedule_id, data FROM activity_info_maps;
\echo == T4 cluster_membership upsert exactly as template (SET includes shard key)
INSERT INTO cluster_membership (membership_partition, host_id, rpc_address, rpc_port, role, session_start, last_heartbeat, record_expiry) VALUES (0, '\x09', '10.0.0.1', 7233, 1, now(), now(), now())
ON CONFLICT(membership_partition, host_id) DO UPDATE SET membership_partition = 0, host_id = '\x09', rpc_address = '10.0.0.1', rpc_port = 7233, role = 1, session_start = now(), last_heartbeat = now(), record_expiry = now();
INSERT INTO cluster_membership (membership_partition, host_id, rpc_address, rpc_port, role, session_start, last_heartbeat, record_expiry) VALUES (0, '\x09', '10.0.0.1', 7233, 1, now(), now(), now())
ON CONFLICT(membership_partition, host_id) DO UPDATE SET membership_partition = 0, host_id = '\x09', rpc_address = '10.0.0.1', rpc_port = 7233, role = 1, session_start = now(), last_heartbeat = now(), record_expiry = now();
\echo == T5 buffered_events per-workflow MAX(id)+1 insert
INSERT INTO buffered_events (shard_id, namespace_id, workflow_id, run_id, id, data, data_encoding)
SELECT 1, '\x01', 'wf', '\x02', COALESCE(MAX(id), 0) + 1, '\x0a', 'proto3' FROM buffered_events WHERE shard_id = 1 AND namespace_id = '\x01' AND workflow_id = 'wf' AND run_id = '\x02';
INSERT INTO buffered_events (shard_id, namespace_id, workflow_id, run_id, id, data, data_encoding)
SELECT 1, '\x01', 'wf', '\x02', COALESCE(MAX(id), 0) + v.n, v.d, 'proto3' FROM buffered_events, (VALUES (1, '\x0b'::bytea), (2, '\x0c'::bytea)) AS v(n, d) WHERE shard_id = 1 AND namespace_id = '\x01' AND workflow_id = 'wf' AND run_id = '\x02' GROUP BY v.n, v.d;
SELECT id, data FROM buffered_events ORDER BY id;
\echo == T6 namespace transaction across reference tables
BEGIN;
SELECT notification_version FROM namespace_metadata WHERE partition_id = 54321 FOR UPDATE;
INSERT INTO namespaces (partition_id, id, name, notification_version, data, data_encoding, is_global) VALUES (0, '\x10', 'default', 1, '\x00', 'proto3', false);
UPDATE namespace_metadata SET notification_version = 2 WHERE notification_version = 1 AND partition_id = 54321;
COMMIT;
SELECT name, notification_version FROM namespaces;
\echo == T7 name UNIQUE enforced on reference table
INSERT INTO namespaces (partition_id, id, name, notification_version, data, data_encoding, is_global) VALUES (0, '\x11', 'default', 1, '\x00', 'proto3', false);
\echo == T8 CHECK constraint on reference table
INSERT INTO nexus_endpoints_partition_status (id, version) VALUES (1, 1);
\echo == T9 standard + sharded + reference in one transaction
-- schema_version is normally created by temporal-sql-tool as a standard table.
CREATE TABLE IF NOT EXISTS schema_version(version_partition INT not null, db_name VARCHAR(255) not null, creation_time TIMESTAMP, curr_version VARCHAR(64), min_compatible_version VARCHAR(64), PRIMARY KEY (version_partition, db_name));
BEGIN;
INSERT INTO schema_version (version_partition, db_name, curr_version) VALUES (0, 'temporal', '1.19');
INSERT INTO shards (shard_id, range_id, data, data_encoding) VALUES (1, 1, '\x00', 'proto3');
INSERT INTO cluster_metadata_info (metadata_partition, cluster_name, data, data_encoding, version) VALUES (0, 'active', '\x00', 'proto3', 1);
COMMIT;
SELECT (SELECT count(*) FROM schema_version) AS sv, (SELECT count(*) FROM shards) AS sh, (SELECT count(*) FROM cluster_metadata_info) AS cm;
\echo == T10 queue v1 GetLastMessageID (outer query has no queue_type filter)
INSERT INTO queue (queue_type, message_id, message_payload, message_encoding) VALUES (1, 1, '\x00', 'proto3');
BEGIN;
SELECT message_id FROM queue WHERE message_id >= (SELECT message_id FROM queue WHERE queue_type=1 ORDER BY message_id DESC LIMIT 1) FOR UPDATE;
COMMIT;
\echo == T11 range_hash fan-out scan used by task queue scavenger
INSERT INTO task_queues_v2 (range_hash, task_queue_id, range_id, data, data_encoding) VALUES (5, '\x01', 1, '\x00', 'proto3'), (900000000, '\x02', 1, '\x00', 'proto3');
SELECT range_hash, task_queue_id FROM task_queues_v2 WHERE range_hash >= 0 AND range_hash <= 4294967295 AND task_queue_id > '\x00' ORDER BY task_queue_id ASC LIMIT 10;
