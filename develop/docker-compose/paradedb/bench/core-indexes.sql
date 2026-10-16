CREATE INDEX by_workflow_type ON executions_visibility (namespace_id, workflow_type_name, (COALESCE(close_time, '9999-12-31 23:59:59')) DESC, start_time DESC, run_id);
CREATE INDEX by_status ON executions_visibility (namespace_id, status, (COALESCE(close_time, '9999-12-31 23:59:59')) DESC, start_time DESC, run_id);
