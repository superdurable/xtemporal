-- Aurora PostgreSQL Limitless schema; mirrors PostgreSQL v12 and must be updated whenever v12 changes.

CREATE TABLE namespaces(
  partition_id INTEGER NOT NULL,
  id BYTEA NOT NULL,
  name VARCHAR(255) UNIQUE NOT NULL,
  notification_version BIGINT NOT NULL,
  --
  data BYTEA NOT NULL,
  data_encoding VARCHAR(16) NOT NULL,
  is_global BOOLEAN NOT NULL,
  PRIMARY KEY(partition_id, id)
);
CALL rds_aurora.limitless_alter_table_type_reference('public.namespaces');

CREATE TABLE namespace_metadata (
  partition_id INTEGER NOT NULL,
  notification_version BIGINT NOT NULL,
  PRIMARY KEY(partition_id)
);
CALL rds_aurora.limitless_alter_table_type_reference('public.namespace_metadata');

INSERT INTO namespace_metadata (partition_id, notification_version) VALUES (54321, 1);

CREATE TABLE shards (
  shard_id INTEGER NOT NULL,
  --
  range_id BIGINT NOT NULL,
  data BYTEA NOT NULL,
  data_encoding VARCHAR(16) NOT NULL,
  PRIMARY KEY (shard_id)
);
CALL rds_aurora.limitless_alter_table_type_sharded('public.shards', ARRAY['shard_id']);

CREATE TABLE executions(
  shard_id INTEGER NOT NULL,
  namespace_id BYTEA NOT NULL,
  workflow_id VARCHAR(255) NOT NULL,
  run_id BYTEA NOT NULL,
  --
  next_event_id BIGINT NOT NULL,
  last_write_version BIGINT NOT NULL,
  data BYTEA NOT NULL,
  data_encoding VARCHAR(16) NOT NULL,
  state BYTEA NOT NULL,
  state_encoding VARCHAR(16) NOT NULL,
  db_record_version BIGINT NOT NULL DEFAULT 0,
  PRIMARY KEY (shard_id, namespace_id, workflow_id, run_id)
);
CALL rds_aurora.limitless_alter_table_type_sharded('public.executions', ARRAY['shard_id'], 'public.shards');

CREATE TABLE current_executions(
  shard_id INTEGER NOT NULL,
  namespace_id BYTEA NOT NULL,
  workflow_id VARCHAR(255) NOT NULL,
  --
  run_id BYTEA NOT NULL,
  create_request_id VARCHAR(255) NOT NULL,
  state INTEGER NOT NULL,
  status INTEGER NOT NULL,
  start_version BIGINT NOT NULL DEFAULT 0,
  start_time TIMESTAMP NULL,
  last_write_version BIGINT NOT NULL,
  -- `data` contains the WorkflowExecutionState (same as in `executions.state` above)
  data BYTEA NULL,
  data_encoding VARCHAR(16) NOT NULL DEFAULT '',
  PRIMARY KEY (shard_id, namespace_id, workflow_id)
);
CALL rds_aurora.limitless_alter_table_type_sharded('public.current_executions', ARRAY['shard_id'], 'public.shards');

CREATE TABLE current_chasm_executions(
  shard_id INTEGER NOT NULL,
  namespace_id BYTEA NOT NULL,
  business_id VARCHAR(255) NOT NULL,
  archetype_id BIGINT NOT NULL,
  --
  run_id BYTEA NOT NULL,
  create_request_id VARCHAR(255) NOT NULL,
  state INTEGER NOT NULL,
  status INTEGER NOT NULL,
  start_version BIGINT NOT NULL DEFAULT 0,
  start_time TIMESTAMP NULL,
  last_write_version BIGINT NOT NULL,
  -- `data` contains the ExecutionState (same as in `executions.state` above)
  data BYTEA NULL,
  data_encoding VARCHAR(16) NOT NULL DEFAULT '',
  PRIMARY KEY (shard_id, namespace_id, business_id, archetype_id)
);
CALL rds_aurora.limitless_alter_table_type_sharded('public.current_chasm_executions', ARRAY['shard_id'], 'public.shards');

CREATE TABLE buffered_events (
  shard_id INTEGER NOT NULL,
  namespace_id BYTEA NOT NULL,
  workflow_id VARCHAR(255) NOT NULL,
  run_id BYTEA NOT NULL,
  id BIGINT NOT NULL,
  --
  data BYTEA NOT NULL,
  data_encoding VARCHAR(16) NOT NULL,
  PRIMARY KEY (shard_id, namespace_id, workflow_id, run_id, id)
);
CALL rds_aurora.limitless_alter_table_type_sharded('public.buffered_events', ARRAY['shard_id'], 'public.shards');

CREATE TABLE tasks (
  range_hash BIGINT NOT NULL,
  task_queue_id BYTEA NOT NULL,
  task_id BIGINT NOT NULL,
  --
  data BYTEA NOT NULL,
  data_encoding VARCHAR(16) NOT NULL,
  PRIMARY KEY (range_hash, task_queue_id, task_id)
);
CALL rds_aurora.limitless_alter_table_type_sharded('public.tasks', ARRAY['range_hash']);

-- Stores ephemeral task queue information such as ack levels and expiry times
CREATE TABLE task_queues (
  range_hash BIGINT NOT NULL,
  task_queue_id BYTEA NOT NULL,
  --
  range_id BIGINT NOT NULL,
  data BYTEA NOT NULL,
  data_encoding VARCHAR(16) NOT NULL,
  PRIMARY KEY (range_hash, task_queue_id)
);
CALL rds_aurora.limitless_alter_table_type_sharded('public.task_queues', ARRAY['range_hash'], 'public.tasks');

-- Stores activity or workflow tasks
-- Used for fairness scheduling. (pass, task_id) are monotonically increasing.
CREATE TABLE tasks_v2 (
  range_hash BIGINT NOT NULL,
  task_queue_id BYTEA NOT NULL,
  pass BIGINT NOT NULL, -- pass for tasks (see stride scheduling algorithm for fairness)
  task_id BIGINT NOT NULL,
  --
  data BYTEA NOT NULL,
  data_encoding VARCHAR(16) NOT NULL,
  PRIMARY KEY (range_hash, task_queue_id, pass, task_id)
);
CALL rds_aurora.limitless_alter_table_type_sharded('public.tasks_v2', ARRAY['range_hash'], 'public.tasks');

-- Stores ephemeral task queue information such as ack levels and expiry times
CREATE TABLE task_queues_v2 (
  range_hash BIGINT NOT NULL,
  task_queue_id BYTEA NOT NULL,
  --
  range_id BIGINT NOT NULL,
  data BYTEA NOT NULL,
  data_encoding VARCHAR(16) NOT NULL,
  PRIMARY KEY (range_hash, task_queue_id)
);
CALL rds_aurora.limitless_alter_table_type_sharded('public.task_queues_v2', ARRAY['range_hash'], 'public.tasks');

-- Stores task queue information such as user provided versioning data
CREATE TABLE task_queue_user_data (
  namespace_id    BYTEA NOT NULL,
  task_queue_name VARCHAR(255) NOT NULL,
  data            BYTEA NOT NULL,       -- temporal.server.api.persistence.v1.TaskQueueUserData
  data_encoding   VARCHAR(16) NOT NULL, -- Encoding type used for serialization, in practice this should always be proto3
  version         BIGINT NOT NULL,      -- Version of this row, used for optimistic concurrency
  PRIMARY KEY (namespace_id, task_queue_name)
);
CALL rds_aurora.limitless_alter_table_type_sharded('public.task_queue_user_data', ARRAY['task_queue_name']);

-- Stores a mapping between build ids and task queues
CREATE TABLE build_id_to_task_queue (
  namespace_id    BYTEA NOT NULL,
  build_id        VARCHAR(255) NOT NULL,
  task_queue_name VARCHAR(255) NOT NULL,
  PRIMARY KEY (namespace_id, build_id, task_queue_name)
);
CALL rds_aurora.limitless_alter_table_type_sharded('public.build_id_to_task_queue', ARRAY['task_queue_name'], 'public.task_queue_user_data');

CREATE TABLE history_immediate_tasks(
  shard_id INTEGER NOT NULL,
  category_id INTEGER NOT NULL,
  task_id BIGINT NOT NULL,
  --
  data BYTEA NOT NULL,
  data_encoding VARCHAR(16) NOT NULL,
  PRIMARY KEY (shard_id, category_id, task_id)
);
CALL rds_aurora.limitless_alter_table_type_sharded('public.history_immediate_tasks', ARRAY['shard_id'], 'public.shards');

CREATE TABLE history_scheduled_tasks (
  shard_id INTEGER NOT NULL,
  category_id INTEGER NOT NULL,
  visibility_timestamp TIMESTAMP NOT NULL,
  task_id BIGINT NOT NULL,
  --
  data BYTEA NOT NULL,
  data_encoding VARCHAR(16) NOT NULL,
  PRIMARY KEY (shard_id, category_id, visibility_timestamp, task_id)
);
CALL rds_aurora.limitless_alter_table_type_sharded('public.history_scheduled_tasks', ARRAY['shard_id'], 'public.shards');

CREATE TABLE transfer_tasks(
  shard_id INTEGER NOT NULL,
  task_id BIGINT NOT NULL,
  --
  data BYTEA NOT NULL,
  data_encoding VARCHAR(16) NOT NULL,
  PRIMARY KEY (shard_id, task_id)
);
CALL rds_aurora.limitless_alter_table_type_sharded('public.transfer_tasks', ARRAY['shard_id'], 'public.shards');

CREATE TABLE timer_tasks (
  shard_id INTEGER NOT NULL,
  visibility_timestamp TIMESTAMP NOT NULL,
  task_id BIGINT NOT NULL,
  --
  data BYTEA NOT NULL,
  data_encoding VARCHAR(16) NOT NULL,
  PRIMARY KEY (shard_id, visibility_timestamp, task_id)
);
CALL rds_aurora.limitless_alter_table_type_sharded('public.timer_tasks', ARRAY['shard_id'], 'public.shards');

CREATE TABLE replication_tasks (
  shard_id INTEGER NOT NULL,
  task_id BIGINT NOT NULL,
  --
  data BYTEA NOT NULL,
  data_encoding VARCHAR(16) NOT NULL,
  PRIMARY KEY (shard_id, task_id)
);
CALL rds_aurora.limitless_alter_table_type_sharded('public.replication_tasks', ARRAY['shard_id'], 'public.shards');

CREATE TABLE replication_tasks_dlq (
  source_cluster_name VARCHAR(255) NOT NULL,
  shard_id INTEGER NOT NULL,
  task_id BIGINT NOT NULL,
  --
  data BYTEA NOT NULL,
  data_encoding VARCHAR(16) NOT NULL,
  PRIMARY KEY (source_cluster_name, shard_id, task_id)
);
CALL rds_aurora.limitless_alter_table_type_sharded('public.replication_tasks_dlq', ARRAY['shard_id'], 'public.shards');

CREATE TABLE visibility_tasks(
  shard_id INTEGER NOT NULL,
  task_id BIGINT NOT NULL,
  --
  data BYTEA NOT NULL,
  data_encoding VARCHAR(16) NOT NULL,
  PRIMARY KEY (shard_id, task_id)
);
CALL rds_aurora.limitless_alter_table_type_sharded('public.visibility_tasks', ARRAY['shard_id'], 'public.shards');

CREATE TABLE activity_info_maps (
-- each row corresponds to one key of one map<string, ActivityInfo>
  shard_id INTEGER NOT NULL,
  namespace_id BYTEA NOT NULL,
  workflow_id VARCHAR(255) NOT NULL,
  run_id BYTEA NOT NULL,
  schedule_id BIGINT NOT NULL,
--
  data BYTEA NOT NULL,
  data_encoding VARCHAR(16),
  PRIMARY KEY (shard_id, namespace_id, workflow_id, run_id, schedule_id)
);
CALL rds_aurora.limitless_alter_table_type_sharded('public.activity_info_maps', ARRAY['shard_id'], 'public.shards');

CREATE TABLE timer_info_maps (
  shard_id INTEGER NOT NULL,
  namespace_id BYTEA NOT NULL,
  workflow_id VARCHAR(255) NOT NULL,
  run_id BYTEA NOT NULL,
  timer_id VARCHAR(255) NOT NULL,
--
  data BYTEA NOT NULL,
  data_encoding VARCHAR(16),
  PRIMARY KEY (shard_id, namespace_id, workflow_id, run_id, timer_id)
);
CALL rds_aurora.limitless_alter_table_type_sharded('public.timer_info_maps', ARRAY['shard_id'], 'public.shards');

CREATE TABLE child_execution_info_maps (
  shard_id INTEGER NOT NULL,
  namespace_id BYTEA NOT NULL,
  workflow_id VARCHAR(255) NOT NULL,
  run_id BYTEA NOT NULL,
  initiated_id BIGINT NOT NULL,
--
  data BYTEA NOT NULL,
  data_encoding VARCHAR(16),
  PRIMARY KEY (shard_id, namespace_id, workflow_id, run_id, initiated_id)
);
CALL rds_aurora.limitless_alter_table_type_sharded('public.child_execution_info_maps', ARRAY['shard_id'], 'public.shards');

CREATE TABLE request_cancel_info_maps (
  shard_id INTEGER NOT NULL,
  namespace_id BYTEA NOT NULL,
  workflow_id VARCHAR(255) NOT NULL,
  run_id BYTEA NOT NULL,
  initiated_id BIGINT NOT NULL,
--
  data BYTEA NOT NULL,
  data_encoding VARCHAR(16),
  PRIMARY KEY (shard_id, namespace_id, workflow_id, run_id, initiated_id)
);
CALL rds_aurora.limitless_alter_table_type_sharded('public.request_cancel_info_maps', ARRAY['shard_id'], 'public.shards');

CREATE TABLE signal_info_maps (
  shard_id INTEGER NOT NULL,
  namespace_id BYTEA NOT NULL,
  workflow_id VARCHAR(255) NOT NULL,
  run_id BYTEA NOT NULL,
  initiated_id BIGINT NOT NULL,
--
  data BYTEA NOT NULL,
  data_encoding VARCHAR(16),
  PRIMARY KEY (shard_id, namespace_id, workflow_id, run_id, initiated_id)
);
CALL rds_aurora.limitless_alter_table_type_sharded('public.signal_info_maps', ARRAY['shard_id'], 'public.shards');

CREATE TABLE signals_requested_sets (
  shard_id INTEGER NOT NULL,
  namespace_id BYTEA NOT NULL,
  workflow_id VARCHAR(255) NOT NULL,
  run_id BYTEA NOT NULL,
  signal_id VARCHAR(255) NOT NULL,
  --
  PRIMARY KEY (shard_id, namespace_id, workflow_id, run_id, signal_id)
);
CALL rds_aurora.limitless_alter_table_type_sharded('public.signals_requested_sets', ARRAY['shard_id'], 'public.shards');

CREATE TABLE chasm_node_maps (
  shard_id INTEGER NOT NULL,
  namespace_id BYTEA NOT NULL,
  workflow_id VARCHAR(255) NOT NULL,
  run_id BYTEA NOT NULL,
  chasm_path BYTEA NOT NULL,
--
  metadata BYTEA NOT NULL,
  metadata_encoding VARCHAR(16),
  data BYTEA,
  data_encoding VARCHAR(16),
  PRIMARY KEY (shard_id, namespace_id, workflow_id, run_id, chasm_path)
);
CALL rds_aurora.limitless_alter_table_type_sharded('public.chasm_node_maps', ARRAY['shard_id'], 'public.shards');

-- history eventsV2: history_node stores history event data
CREATE TABLE history_node (
  shard_id       INTEGER NOT NULL,
  tree_id        BYTEA NOT NULL,
  branch_id      BYTEA NOT NULL,
  node_id        BIGINT NOT NULL,
  txn_id         BIGINT NOT NULL,
  --
  prev_txn_id    BIGINT NOT NULL DEFAULT 0,
  data           BYTEA NOT NULL,
  data_encoding  VARCHAR(16) NOT NULL,
  PRIMARY KEY (shard_id, tree_id, branch_id, node_id, txn_id)
);
CALL rds_aurora.limitless_alter_table_type_sharded('public.history_node', ARRAY['shard_id'], 'public.shards');

-- history eventsV2: history_tree stores branch metadata
CREATE TABLE history_tree (
  shard_id       INTEGER NOT NULL,
  tree_id        BYTEA NOT NULL,
  branch_id      BYTEA NOT NULL,
  --
  data           BYTEA NOT NULL,
  data_encoding  VARCHAR(16) NOT NULL,
  PRIMARY KEY (shard_id, tree_id, branch_id)
);
CALL rds_aurora.limitless_alter_table_type_sharded('public.history_tree', ARRAY['shard_id'], 'public.shards');

CREATE TABLE queue (
  queue_type        INTEGER NOT NULL,
  message_id        BIGINT NOT NULL,
  message_payload   BYTEA NOT NULL,
  message_encoding  VARCHAR(16) NOT NULL,
  PRIMARY KEY(queue_type, message_id)
);
CALL rds_aurora.limitless_alter_table_type_reference('public.queue');

CREATE TABLE queue_metadata (
  queue_type     INTEGER NOT NULL,
  data BYTEA     NOT NULL,
  data_encoding  VARCHAR(16) NOT NULL,
  version        BIGINT NOT NULL,
  PRIMARY KEY(queue_type)
);
CALL rds_aurora.limitless_alter_table_type_reference('public.queue_metadata');

CREATE TABLE cluster_metadata_info (
  metadata_partition        INTEGER NOT NULL,
  cluster_name              VARCHAR(255) NOT NULL,
  data                      BYTEA NOT NULL,
  data_encoding             VARCHAR(16) NOT NULL,
  version                   BIGINT NOT NULL,
  PRIMARY KEY(metadata_partition, cluster_name)
);
CALL rds_aurora.limitless_alter_table_type_reference('public.cluster_metadata_info');

CREATE TABLE cluster_membership
(
    membership_partition INTEGER NOT NULL,
    host_id              BYTEA NOT NULL,
    rpc_address          VARCHAR(128) NOT NULL,
    rpc_port             SMALLINT NOT NULL,
    role                 SMALLINT NOT NULL,
    session_start        TIMESTAMP DEFAULT '1970-01-01 00:00:01+00:00',
    last_heartbeat       TIMESTAMP DEFAULT '1970-01-01 00:00:01+00:00',
    record_expiry        TIMESTAMP DEFAULT '1970-01-01 00:00:01+00:00',
    PRIMARY KEY (membership_partition, host_id)
);
CALL rds_aurora.limitless_alter_table_type_sharded('public.cluster_membership', ARRAY['membership_partition']);

CREATE TABLE queues (
    queue_type INT NOT NULL,
    queue_name VARCHAR(255) NOT NULL,
    metadata_payload BYTEA NOT NULL,
    metadata_encoding VARCHAR(16) NOT NULL,
    PRIMARY KEY (queue_type, queue_name)
);
CALL rds_aurora.limitless_alter_table_type_sharded('public.queues', ARRAY['queue_type','queue_name']);

CREATE TABLE queue_messages (
    queue_type INT NOT NULL,
    queue_name VARCHAR(255) NOT NULL,
    queue_partition BIGINT NOT NULL,
    message_id BIGINT NOT NULL,
    message_payload BYTEA NOT NULL,
    message_encoding VARCHAR(16) NOT NULL,
    PRIMARY KEY (
        queue_type,
        queue_name,
        queue_partition,
        message_id
    )
);
CALL rds_aurora.limitless_alter_table_type_sharded('public.queue_messages', ARRAY['queue_type','queue_name'], 'public.queues');

-- Stores information about Nexus endpoints
CREATE TABLE nexus_endpoints (
    id            BYTEA NOT NULL,
    data          BYTEA NOT NULL,  -- temporal.server.api.persistence.v1.NexusEndpoint
    data_encoding VARCHAR(16) NOT NULL, -- Encoding type used for serialization, in practice this should always be proto3
    version       BIGINT NOT NULL,      -- Version of this row, used for optimistic concurrency
    PRIMARY KEY (id)
);
CALL rds_aurora.limitless_alter_table_type_reference('public.nexus_endpoints');

-- Stores the version of Nexus endpoints table as a whole
CREATE TABLE nexus_endpoints_partition_status (
    id      INT NOT NULL PRIMARY KEY DEFAULT 0,
    version BIGINT NOT NULL,                -- Version of the nexus_endpoints table
    CONSTRAINT only_one_row CHECK (id = 0)  -- Restrict the table to a single row since it will only be used for endpoints
);
CALL rds_aurora.limitless_alter_table_type_reference('public.nexus_endpoints_partition_status');

CREATE UNIQUE INDEX cm_idx_rolehost ON cluster_membership (membership_partition, role, host_id);
CREATE INDEX cm_idx_rolelasthb ON cluster_membership (role, last_heartbeat);
CREATE INDEX cm_idx_rpchost ON cluster_membership (rpc_address, role);
CREATE INDEX cm_idx_lasthb ON cluster_membership (last_heartbeat);
CREATE INDEX cm_idx_recordexpiry ON cluster_membership (record_expiry);
