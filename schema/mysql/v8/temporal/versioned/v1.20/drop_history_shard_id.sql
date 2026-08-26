-- history_node and history_tree are only ever accessed by their full primary key, keyed off the
-- branch token. Carrying shard_id in that key pinned every history event to the shard the workflow
-- happened to hash to at write time, which would force a rewrite of the largest tables in the
-- database whenever numHistoryShards changes. Dropping the column removes that coupling.
ALTER TABLE history_node
  DROP PRIMARY KEY,
  DROP COLUMN shard_id,
  ADD PRIMARY KEY (tree_id, branch_id, node_id, txn_id);

ALTER TABLE history_tree
  DROP PRIMARY KEY,
  DROP COLUMN shard_id,
  ADD PRIMARY KEY (tree_id, branch_id);
