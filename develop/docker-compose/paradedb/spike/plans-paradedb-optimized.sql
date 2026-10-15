\pset pager off
\echo 'plain namespace/status/time'
EXPLAIN (ANALYZE, BUFFERS) SELECT run_id FROM executions_visibility WHERE namespace_id = md5('0') AND status = 1 AND start_time >= '2026-01-01' AND start_time < '2026-01-02' ORDER BY sort_close_time DESC, start_time DESC, run_id COLLATE "C" LIMIT 20;
\echo 'anchored namespace/status/time'
EXPLAIN (ANALYZE, BUFFERS) SELECT run_id FROM executions_visibility WHERE namespace_id @@@ md5('0') AND status = 1 AND start_time >= '2026-01-01' AND start_time < '2026-01-02' ORDER BY sort_close_time DESC, start_time DESC, run_id COLLATE "C" LIMIT 20;
\echo 'workflow type/status'
EXPLAIN (ANALYZE, BUFFERS) SELECT run_id FROM executions_visibility WHERE namespace_id @@@ md5('0') AND workflow_type_name = 'type-0' AND status = 1 ORDER BY sort_close_time DESC, start_time DESC, run_id COLLATE "C" LIMIT 20;
\echo 'text search'
EXPLAIN (ANALYZE, BUFFERS) SELECT run_id FROM executions_visibility WHERE namespace_id @@@ md5('0') AND Text01 ||| 'hello' ORDER BY sort_close_time DESC, start_time DESC, run_id COLLATE "C" LIMIT 20;
\echo 'keyword in'
EXPLAIN (ANALYZE, BUFFERS) SELECT run_id FROM executions_visibility WHERE namespace_id @@@ md5('0') AND Keyword01 IN ('tenant-0', 'tenant-20') ORDER BY sort_close_time DESC, start_time DESC, run_id COLLATE "C" LIMIT 20;
\echo 'keyword prefix'
EXPLAIN (ANALYZE, BUFFERS) SELECT run_id FROM executions_visibility WHERE namespace_id @@@ md5('0') AND Keyword02 @@@ pdb.regex('prefix-12.*') ORDER BY sort_close_time DESC, start_time DESC, run_id COLLATE "C" LIMIT 20;
\echo 'keyword list'
EXPLAIN (ANALYZE, BUFFERS) SELECT run_id FROM executions_visibility WHERE namespace_id @@@ md5('0') AND KeywordList01 @@@ pdb.term('tag-0') ORDER BY sort_close_time DESC, start_time DESC, run_id COLLATE "C" LIMIT 20;
\echo 'count'
EXPLAIN (ANALYZE, BUFFERS) SELECT COUNT(*) FROM executions_visibility WHERE namespace_id @@@ md5('0') AND status = 1;
\echo 'group count'
EXPLAIN (ANALYZE, BUFFERS) SELECT status, COUNT(*) FROM executions_visibility WHERE namespace_id @@@ md5('0') GROUP BY status;
\echo 'raw field order'
EXPLAIN (ANALYZE, BUFFERS) SELECT run_id FROM executions_visibility WHERE namespace_id @@@ md5('0') ORDER BY close_time DESC, start_time DESC, run_id LIMIT 20;
