-- history_node and history_tree are only ever accessed by their full primary key, keyed off the
-- branch token. Carrying shard_id in that key pinned every history event to the shard the workflow
-- happened to hash to at write time, which would force a rewrite of the largest tables in the
-- database whenever numHistoryShards changes. Dropping the column removes that coupling.
--
-- SQLite cannot drop a column that participates in the primary key, so both tables are recreated.
CREATE TABLE history_node_new (
	tree_id BINARY(16) NOT NULL,
	branch_id BINARY(16) NOT NULL,
	node_id BIGINT NOT NULL,
	txn_id BIGINT NOT NULL,
	--
	prev_txn_id BIGINT NOT NULL DEFAULT 0,
	data MEDIUMBLOB NOT NULL,
	data_encoding VARCHAR(16) NOT NULL,
	PRIMARY KEY (tree_id, branch_id, node_id, txn_id)
);

INSERT INTO history_node_new (tree_id, branch_id, node_id, txn_id, prev_txn_id, data, data_encoding)
	SELECT tree_id, branch_id, node_id, txn_id, prev_txn_id, data, data_encoding FROM history_node;

DROP TABLE history_node;

ALTER TABLE history_node_new RENAME TO history_node;

CREATE TABLE history_tree_new (
	tree_id BINARY(16) NOT NULL,
	branch_id BINARY(16) NOT NULL,
	--
	data MEDIUMBLOB NOT NULL,
	data_encoding VARCHAR(16) NOT NULL,
	PRIMARY KEY (tree_id, branch_id)
);

INSERT INTO history_tree_new (tree_id, branch_id, data, data_encoding)
	SELECT tree_id, branch_id, data, data_encoding FROM history_tree;

DROP TABLE history_tree;

ALTER TABLE history_tree_new RENAME TO history_tree;
