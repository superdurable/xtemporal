-- These tables hold per-workflow state and are only ever accessed through the full workflow key
-- (namespace_id, workflow_id, run_id). shard_id led the primary key purely as a locality prefix,
-- which pinned every row to whichever shard the workflow hashed to at write time and would force a
-- rewrite whenever numHistoryShards changes. Dropping the column removes that coupling.
ALTER TABLE buffered_events DROP CONSTRAINT buffered_events_pkey;
ALTER TABLE buffered_events DROP COLUMN shard_id;
ALTER TABLE buffered_events ADD PRIMARY KEY (namespace_id, workflow_id, run_id, id);

ALTER TABLE activity_info_maps DROP CONSTRAINT activity_info_maps_pkey;
ALTER TABLE activity_info_maps DROP COLUMN shard_id;
ALTER TABLE activity_info_maps ADD PRIMARY KEY (namespace_id, workflow_id, run_id, schedule_id);

ALTER TABLE timer_info_maps DROP CONSTRAINT timer_info_maps_pkey;
ALTER TABLE timer_info_maps DROP COLUMN shard_id;
ALTER TABLE timer_info_maps ADD PRIMARY KEY (namespace_id, workflow_id, run_id, timer_id);

ALTER TABLE child_execution_info_maps DROP CONSTRAINT child_execution_info_maps_pkey;
ALTER TABLE child_execution_info_maps DROP COLUMN shard_id;
ALTER TABLE child_execution_info_maps ADD PRIMARY KEY (namespace_id, workflow_id, run_id, initiated_id);

ALTER TABLE request_cancel_info_maps DROP CONSTRAINT request_cancel_info_maps_pkey;
ALTER TABLE request_cancel_info_maps DROP COLUMN shard_id;
ALTER TABLE request_cancel_info_maps ADD PRIMARY KEY (namespace_id, workflow_id, run_id, initiated_id);

ALTER TABLE signal_info_maps DROP CONSTRAINT signal_info_maps_pkey;
ALTER TABLE signal_info_maps DROP COLUMN shard_id;
ALTER TABLE signal_info_maps ADD PRIMARY KEY (namespace_id, workflow_id, run_id, initiated_id);

ALTER TABLE signals_requested_sets DROP CONSTRAINT signals_requested_sets_pkey;
ALTER TABLE signals_requested_sets DROP COLUMN shard_id;
ALTER TABLE signals_requested_sets ADD PRIMARY KEY (namespace_id, workflow_id, run_id, signal_id);

ALTER TABLE chasm_node_maps DROP CONSTRAINT chasm_node_maps_pkey;
ALTER TABLE chasm_node_maps DROP COLUMN shard_id;
ALTER TABLE chasm_node_maps ADD PRIMARY KEY (namespace_id, workflow_id, run_id, chasm_path);
