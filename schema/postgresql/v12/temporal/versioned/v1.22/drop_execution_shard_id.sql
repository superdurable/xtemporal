-- These tables are only ever accessed by the full workflow key: executions by
-- (namespace_id, workflow_id, run_id), current_executions by (namespace_id, workflow_id), and
-- current_chasm_executions by (namespace_id, business_id, archetype_id). shard_id led the primary
-- key purely as a locality prefix.
--
-- Beyond removing the rewrite that changing numHistoryShards would otherwise require, dropping the
-- column is what keeps "one current run per workflow" enforceable by the database once the shard
-- count can change: with shard_id in the key, the same workflow could occupy two current_executions
-- rows under two different shard ids and the uniqueness guarantee would silently break.
ALTER TABLE executions DROP CONSTRAINT executions_pkey;
ALTER TABLE executions DROP COLUMN shard_id;
ALTER TABLE executions ADD PRIMARY KEY (namespace_id, workflow_id, run_id);

ALTER TABLE current_executions DROP CONSTRAINT current_executions_pkey;
ALTER TABLE current_executions DROP COLUMN shard_id;
ALTER TABLE current_executions ADD PRIMARY KEY (namespace_id, workflow_id);

ALTER TABLE current_chasm_executions DROP CONSTRAINT current_chasm_executions_pkey;
ALTER TABLE current_chasm_executions DROP COLUMN shard_id;
ALTER TABLE current_chasm_executions ADD PRIMARY KEY (namespace_id, business_id, archetype_id);
