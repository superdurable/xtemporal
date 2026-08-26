-- These tables hold per-workflow state and are only ever accessed through the full workflow key
-- (namespace_id, workflow_id, run_id). shard_id led the primary key purely as a locality prefix,
-- which pinned every row to whichever shard the workflow hashed to at write time and would force a
-- rewrite whenever numHistoryShards changes. Dropping the column removes that coupling.
--
-- SQLite cannot drop a column that participates in the primary key, so each table is recreated.
CREATE TABLE buffered_events_new (
	namespace_id BINARY(16) NOT NULL,
	workflow_id VARCHAR(255) NOT NULL,
	run_id BINARY(16) NOT NULL,
	id BIGINT AUTO_INCREMENT UNIQUE,
	--
	data MEDIUMBLOB NOT NULL,
	data_encoding VARCHAR(16) NOT NULL,
	PRIMARY KEY (namespace_id, workflow_id, run_id, id)
);

INSERT INTO buffered_events_new (namespace_id, workflow_id, run_id, id, data, data_encoding)
	SELECT namespace_id, workflow_id, run_id, id, data, data_encoding FROM buffered_events;

DROP TABLE buffered_events;

ALTER TABLE buffered_events_new RENAME TO buffered_events;

CREATE TABLE activity_info_maps_new (
-- each row corresponds to one key of one map<string, ActivityInfo>
	namespace_id BINARY(16) NOT NULL,
	workflow_id VARCHAR(255) NOT NULL,
	run_id BINARY(16) NOT NULL,
	schedule_id BIGINT NOT NULL,
--
	data MEDIUMBLOB NOT NULL,
	data_encoding VARCHAR(16),
	PRIMARY KEY (namespace_id, workflow_id, run_id, schedule_id)
);

INSERT INTO activity_info_maps_new (namespace_id, workflow_id, run_id, schedule_id, data, data_encoding)
	SELECT namespace_id, workflow_id, run_id, schedule_id, data, data_encoding FROM activity_info_maps;

DROP TABLE activity_info_maps;

ALTER TABLE activity_info_maps_new RENAME TO activity_info_maps;

CREATE TABLE timer_info_maps_new (
	namespace_id BINARY(16) NOT NULL,
	workflow_id VARCHAR(255) NOT NULL,
	run_id BINARY(16) NOT NULL,
	timer_id VARCHAR(255) NOT NULL,
--
	data MEDIUMBLOB NOT NULL,
	data_encoding VARCHAR(16),
	PRIMARY KEY (namespace_id, workflow_id, run_id, timer_id)
);

INSERT INTO timer_info_maps_new (namespace_id, workflow_id, run_id, timer_id, data, data_encoding)
	SELECT namespace_id, workflow_id, run_id, timer_id, data, data_encoding FROM timer_info_maps;

DROP TABLE timer_info_maps;

ALTER TABLE timer_info_maps_new RENAME TO timer_info_maps;

CREATE TABLE child_execution_info_maps_new (
	namespace_id BINARY(16) NOT NULL,
	workflow_id VARCHAR(255) NOT NULL,
	run_id BINARY(16) NOT NULL,
	initiated_id BIGINT NOT NULL,
--
	data MEDIUMBLOB NOT NULL,
	data_encoding VARCHAR(16),
	PRIMARY KEY (namespace_id, workflow_id, run_id, initiated_id)
);

INSERT INTO child_execution_info_maps_new (namespace_id, workflow_id, run_id, initiated_id, data, data_encoding)
	SELECT namespace_id, workflow_id, run_id, initiated_id, data, data_encoding FROM child_execution_info_maps;

DROP TABLE child_execution_info_maps;

ALTER TABLE child_execution_info_maps_new RENAME TO child_execution_info_maps;

CREATE TABLE request_cancel_info_maps_new (
	namespace_id BINARY(16) NOT NULL,
	workflow_id VARCHAR(255) NOT NULL,
	run_id BINARY(16) NOT NULL,
	initiated_id BIGINT NOT NULL,
--
	data MEDIUMBLOB NOT NULL,
	data_encoding VARCHAR(16),
	PRIMARY KEY (namespace_id, workflow_id, run_id, initiated_id)
);

INSERT INTO request_cancel_info_maps_new (namespace_id, workflow_id, run_id, initiated_id, data, data_encoding)
	SELECT namespace_id, workflow_id, run_id, initiated_id, data, data_encoding FROM request_cancel_info_maps;

DROP TABLE request_cancel_info_maps;

ALTER TABLE request_cancel_info_maps_new RENAME TO request_cancel_info_maps;

CREATE TABLE signal_info_maps_new (
	namespace_id BINARY(16) NOT NULL,
	workflow_id VARCHAR(255) NOT NULL,
	run_id BINARY(16) NOT NULL,
	initiated_id BIGINT NOT NULL,
--
	data MEDIUMBLOB NOT NULL,
	data_encoding VARCHAR(16),
	PRIMARY KEY (namespace_id, workflow_id, run_id, initiated_id)
);

INSERT INTO signal_info_maps_new (namespace_id, workflow_id, run_id, initiated_id, data, data_encoding)
	SELECT namespace_id, workflow_id, run_id, initiated_id, data, data_encoding FROM signal_info_maps;

DROP TABLE signal_info_maps;

ALTER TABLE signal_info_maps_new RENAME TO signal_info_maps;

CREATE TABLE signals_requested_sets_new (
	namespace_id BINARY(16) NOT NULL,
	workflow_id VARCHAR(255) NOT NULL,
	run_id BINARY(16) NOT NULL,
	signal_id VARCHAR(255) NOT NULL,
	--
	PRIMARY KEY (namespace_id, workflow_id, run_id, signal_id)
);

INSERT INTO signals_requested_sets_new (namespace_id, workflow_id, run_id, signal_id)
	SELECT namespace_id, workflow_id, run_id, signal_id FROM signals_requested_sets;

DROP TABLE signals_requested_sets;

ALTER TABLE signals_requested_sets_new RENAME TO signals_requested_sets;

CREATE TABLE chasm_node_maps_new (
  namespace_id BINARY(16) NOT NULL,
  workflow_id VARCHAR(255) NOT NULL,
  run_id BINARY(16) NOT NULL,
  chasm_path BINARY(1536) NOT NULL,
--
  metadata MEDIUMBLOB NOT NULL,
  metadata_encoding VARCHAR(16),
  data MEDIUMBLOB,
  data_encoding VARCHAR(16),
  PRIMARY KEY (namespace_id, workflow_id, run_id, chasm_path)
);

INSERT INTO chasm_node_maps_new (namespace_id, workflow_id, run_id, chasm_path, metadata, metadata_encoding, data, data_encoding)
	SELECT namespace_id, workflow_id, run_id, chasm_path, metadata, metadata_encoding, data, data_encoding FROM chasm_node_maps;

DROP TABLE chasm_node_maps;

ALTER TABLE chasm_node_maps_new RENAME TO chasm_node_maps;
