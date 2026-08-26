-- These tables hold per-workflow state and are only ever accessed through the full workflow key
-- (namespace_id, workflow_id, run_id). shard_id led the primary key purely as a locality prefix,
-- which pinned every row to whichever shard the workflow hashed to at write time and would force a
-- rewrite whenever numHistoryShards changes. Dropping the column removes that coupling.
ALTER TABLE buffered_events
  DROP PRIMARY KEY,
  DROP COLUMN shard_id,
  ADD PRIMARY KEY (namespace_id, workflow_id, run_id, id);

ALTER TABLE activity_info_maps
  DROP PRIMARY KEY,
  DROP COLUMN shard_id,
  ADD PRIMARY KEY (namespace_id, workflow_id, run_id, schedule_id);

ALTER TABLE timer_info_maps
  DROP PRIMARY KEY,
  DROP COLUMN shard_id,
  ADD PRIMARY KEY (namespace_id, workflow_id, run_id, timer_id);

ALTER TABLE child_execution_info_maps
  DROP PRIMARY KEY,
  DROP COLUMN shard_id,
  ADD PRIMARY KEY (namespace_id, workflow_id, run_id, initiated_id);

ALTER TABLE request_cancel_info_maps
  DROP PRIMARY KEY,
  DROP COLUMN shard_id,
  ADD PRIMARY KEY (namespace_id, workflow_id, run_id, initiated_id);

ALTER TABLE signal_info_maps
  DROP PRIMARY KEY,
  DROP COLUMN shard_id,
  ADD PRIMARY KEY (namespace_id, workflow_id, run_id, initiated_id);

ALTER TABLE signals_requested_sets
  DROP PRIMARY KEY,
  DROP COLUMN shard_id,
  ADD PRIMARY KEY (namespace_id, workflow_id, run_id, signal_id);

ALTER TABLE chasm_node_maps
  DROP PRIMARY KEY,
  DROP COLUMN shard_id,
  ADD PRIMARY KEY (namespace_id, workflow_id, run_id, chasm_path);
