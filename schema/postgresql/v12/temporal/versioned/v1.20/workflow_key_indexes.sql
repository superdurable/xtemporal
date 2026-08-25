-- Keep shard_id and the existing primary keys so old processes remain compatible during a rolling deployment.
CREATE UNIQUE INDEX executions_by_workflow
  ON executions (namespace_id, workflow_id, run_id);
CREATE UNIQUE INDEX current_executions_by_workflow
  ON current_executions (namespace_id, workflow_id);
CREATE UNIQUE INDEX current_chasm_executions_by_business
  ON current_chasm_executions (namespace_id, business_id, archetype_id);
CREATE INDEX buffered_events_by_workflow
  ON buffered_events (namespace_id, workflow_id, run_id, id);
CREATE UNIQUE INDEX activity_info_maps_by_workflow
  ON activity_info_maps (namespace_id, workflow_id, run_id, schedule_id);
CREATE UNIQUE INDEX timer_info_maps_by_workflow
  ON timer_info_maps (namespace_id, workflow_id, run_id, timer_id);
CREATE UNIQUE INDEX child_execution_info_maps_by_workflow
  ON child_execution_info_maps (namespace_id, workflow_id, run_id, initiated_id);
CREATE UNIQUE INDEX request_cancel_info_maps_by_workflow
  ON request_cancel_info_maps (namespace_id, workflow_id, run_id, initiated_id);
CREATE UNIQUE INDEX signal_info_maps_by_workflow
  ON signal_info_maps (namespace_id, workflow_id, run_id, initiated_id);
CREATE UNIQUE INDEX signals_requested_sets_by_workflow
  ON signals_requested_sets (namespace_id, workflow_id, run_id, signal_id);
CREATE UNIQUE INDEX chasm_node_maps_by_workflow
  ON chasm_node_maps (namespace_id, workflow_id, run_id, chasm_path);
CREATE UNIQUE INDEX history_node_by_branch
  ON history_node (tree_id, branch_id, node_id, txn_id);
CREATE UNIQUE INDEX history_tree_by_branch
  ON history_tree (tree_id, branch_id);
