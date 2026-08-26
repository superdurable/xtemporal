-- These tables are only ever accessed by the full workflow key: executions by
-- (namespace_id, workflow_id, run_id), current_executions by (namespace_id, workflow_id), and
-- current_chasm_executions by (namespace_id, business_id, archetype_id). shard_id led the primary
-- key purely as a locality prefix.
--
-- Beyond removing the rewrite that changing numHistoryShards would otherwise require, dropping the
-- column is what keeps "one current run per workflow" enforceable by the database once the shard
-- count can change: with shard_id in the key, the same workflow could occupy two current_executions
-- rows under two different shard ids and the uniqueness guarantee would silently break.
--
-- SQLite cannot drop a column that participates in the primary key, so each table is recreated.
CREATE TABLE executions_new (
	namespace_id BINARY(16) NOT NULL,
	workflow_id VARCHAR(255) NOT NULL,
	run_id BINARY(16) NOT NULL,
	--
	next_event_id BIGINT NOT NULL,
	last_write_version BIGINT NOT NULL,
	data MEDIUMBLOB NOT NULL,
	data_encoding VARCHAR(16) NOT NULL,
	state MEDIUMBLOB NOT NULL,
	state_encoding VARCHAR(16) NOT NULL,
	db_record_version BIGINT NOT NULL DEFAULT 0,
	PRIMARY KEY (namespace_id, workflow_id, run_id)
);

INSERT INTO executions_new (namespace_id, workflow_id, run_id, next_event_id, last_write_version, data, data_encoding, state, state_encoding, db_record_version)
	SELECT namespace_id, workflow_id, run_id, next_event_id, last_write_version, data, data_encoding, state, state_encoding, db_record_version FROM executions;

DROP TABLE executions;

ALTER TABLE executions_new RENAME TO executions;

CREATE TABLE current_executions_new (
	namespace_id BINARY(16) NOT NULL,
	workflow_id VARCHAR(255) NOT NULL,
	--
	run_id BINARY(16) NOT NULL,
	create_request_id VARCHAR(255) NOT NULL,
	state INT NOT NULL,
	status INT NOT NULL,
	start_time TIMESTAMP NULL,
	last_write_version BIGINT NOT NULL,
	-- `data` contains the WorkflowExecutionState (same as in `executions.state` above)
	data MEDIUMBLOB NOT NULL,
	data_encoding VARCHAR(16) NOT NULL DEFAULT '',
	PRIMARY KEY (namespace_id, workflow_id)
);

INSERT INTO current_executions_new (namespace_id, workflow_id, run_id, create_request_id, state, status, start_time, last_write_version, data, data_encoding)
	SELECT namespace_id, workflow_id, run_id, create_request_id, state, status, start_time, last_write_version, data, data_encoding FROM current_executions;

DROP TABLE current_executions;

ALTER TABLE current_executions_new RENAME TO current_executions;

CREATE TABLE current_chasm_executions_new (
	namespace_id BINARY(16) NOT NULL,
	business_id VARCHAR(255) NOT NULL,
	archetype_id BIGINT NOT NULL,
	--
	run_id BINARY(16) NOT NULL,
	create_request_id VARCHAR(255) NOT NULL,
	state INT NOT NULL,
	status INT NOT NULL,
	start_time TIMESTAMP NULL,
	last_write_version BIGINT NOT NULL,
	-- `data` contains the ExecutionState (same as in `executions.state` above)
	data MEDIUMBLOB NOT NULL,
	data_encoding VARCHAR(16) NOT NULL DEFAULT '',
	PRIMARY KEY (namespace_id, business_id, archetype_id)
);

INSERT INTO current_chasm_executions_new (namespace_id, business_id, archetype_id, run_id, create_request_id, state, status, start_time, last_write_version, data, data_encoding)
	SELECT namespace_id, business_id, archetype_id, run_id, create_request_id, state, status, start_time, last_write_version, data, data_encoding FROM current_chasm_executions;

DROP TABLE current_chasm_executions;

ALTER TABLE current_chasm_executions_new RENAME TO current_chasm_executions;
