CREATE TABLE chasm_node_maps (
  namespace_id BINARY(16) NOT NULL,
  workflow_id VARCHAR(255) NOT NULL,
  run_id BINARY(16) NOT NULL,
  chasm_path VARBINARY(1536) NOT NULL,
--
  metadata MEDIUMBLOB NOT NULL,
  metadata_encoding VARCHAR(16),
  data MEDIUMBLOB,
  data_encoding VARCHAR(16),
  PRIMARY KEY (namespace_id, workflow_id, run_id, chasm_path)
);
