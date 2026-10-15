-- Minimal reproduction of the Aurora PostgreSQL Limitless (16.13-limitless) silent write loss:
-- after EXPLAIN with limitless_explain_options including shard_plans, a later write of the
-- explained key in the SAME session reports success but stores nothing.
-- Run in a disposable database, in ONE session: psql "$DSN" -X -f develop/limitless/sql/explain-lost-write.sql
DROP TABLE IF EXISTS explain_lost_write;
CREATE TABLE explain_lost_write (shard_id integer PRIMARY KEY, range_id bigint NOT NULL);
CALL rds_aurora.limitless_alter_table_type_sharded('public.explain_lost_write', ARRAY['shard_id']);

SET rds_aurora.limitless_explain_options = 'shard_plans, single_shard_optimization';
EXPLAIN SELECT * FROM explain_lost_write WHERE shard_id = 123456;
INSERT INTO explain_lost_write (shard_id, range_id) VALUES (123456, 5);
-- Expected 1; the bug returns 0. A new session also sees 0, and re-inserting 123456 succeeds.
SELECT count(*) AS rows_after_insert FROM explain_lost_write WHERE shard_id + 0 = 123456;

DROP TABLE explain_lost_write;
