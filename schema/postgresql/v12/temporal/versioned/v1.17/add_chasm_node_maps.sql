CREATE TABLE chasm_node_maps (
  namespace_id BYTEA NOT NULL,
  workflow_id VARCHAR(255) NOT NULL,
  run_id BYTEA NOT NULL,
  chasm_path BYTEA NOT NULL,
--
  metadata BYTEA NOT NULL,
  metadata_encoding VARCHAR(16),
  data BYTEA,
  data_encoding VARCHAR(16),
  PRIMARY KEY (namespace_id, workflow_id, run_id, chasm_path)
);
