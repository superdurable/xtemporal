-- PostgreSQL v12 B-tree indexes for core-field filters, used by the hybrid variants so core-only queries avoid the BM25 index.
-- They match what a production hybrid deployment would need, so the write cost is realistic, not just what the benchmark queries touch.
-- default_idx is already part of both base ParadeDB schemas.
CREATE INDEX by_execution_time          ON executions_visibility (namespace_id, execution_time,         (COALESCE(close_time, '9999-12-31 23:59:59')) DESC, start_time DESC, run_id);
CREATE INDEX by_workflow_id             ON executions_visibility (namespace_id, workflow_id,            (COALESCE(close_time, '9999-12-31 23:59:59')) DESC, start_time DESC, run_id);
CREATE INDEX by_task_queue              ON executions_visibility (namespace_id, task_queue,             (COALESCE(close_time, '9999-12-31 23:59:59')) DESC, start_time DESC, run_id);
CREATE INDEX by_workflow_type ON executions_visibility (namespace_id, workflow_type_name, (COALESCE(close_time, '9999-12-31 23:59:59')) DESC, start_time DESC, run_id);
CREATE INDEX by_status ON executions_visibility (namespace_id, status, (COALESCE(close_time, '9999-12-31 23:59:59')) DESC, start_time DESC, run_id);
