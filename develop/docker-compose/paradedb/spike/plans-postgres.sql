\pset pager off
\echo 'namespace/status/time'
EXPLAIN (ANALYZE, BUFFERS) SELECT run_id FROM executions_visibility WHERE namespace_id = md5('0') AND status = 1 AND start_time >= '2026-01-01' AND start_time < '2026-01-02' ORDER BY coalesce(close_time, '9999-12-31 23:59:59') DESC, start_time DESC, run_id LIMIT 20;
\echo 'workflow type/status'
EXPLAIN (ANALYZE, BUFFERS) SELECT run_id FROM executions_visibility WHERE namespace_id = md5('0') AND workflow_type_name = 'type-0' AND status = 1 ORDER BY coalesce(close_time, '9999-12-31 23:59:59') DESC, start_time DESC, run_id LIMIT 20;
\echo 'text search'
EXPLAIN (ANALYZE, BUFFERS) SELECT run_id FROM executions_visibility WHERE namespace_id = md5('0') AND Text01 @@ 'hello'::tsquery ORDER BY coalesce(close_time, '9999-12-31 23:59:59') DESC, start_time DESC, run_id LIMIT 20;
\echo 'keyword in'
EXPLAIN (ANALYZE, BUFFERS) SELECT run_id FROM executions_visibility WHERE namespace_id = md5('0') AND Keyword01 IN ('tenant-0', 'tenant-20') ORDER BY coalesce(close_time, '9999-12-31 23:59:59') DESC, start_time DESC, run_id LIMIT 20;
\echo 'keyword prefix'
EXPLAIN (ANALYZE, BUFFERS) SELECT run_id FROM executions_visibility WHERE namespace_id = md5('0') AND Keyword02 LIKE 'prefix-12%' ORDER BY coalesce(close_time, '9999-12-31 23:59:59') DESC, start_time DESC, run_id LIMIT 20;
\echo 'keyword list'
EXPLAIN (ANALYZE, BUFFERS) SELECT run_id FROM executions_visibility WHERE namespace_id = md5('0') AND KeywordList01 @> jsonb_build_array('tag-0') ORDER BY coalesce(close_time, '9999-12-31 23:59:59') DESC, start_time DESC, run_id LIMIT 20;
\echo 'count'
EXPLAIN (ANALYZE, BUFFERS) SELECT COUNT(*) FROM executions_visibility WHERE namespace_id = md5('0') AND status = 1;
\echo 'group count'
EXPLAIN (ANALYZE, BUFFERS) SELECT status, COUNT(*) FROM executions_visibility WHERE namespace_id = md5('0') GROUP BY status;
