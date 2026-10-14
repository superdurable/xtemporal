\timing on
INSERT INTO executions_visibility (namespace_id, run_id, _version, start_time, execution_time, workflow_id, workflow_type_name, status, close_time, history_length, history_size_bytes, execution_duration, state_transition_count, memo, encoding, task_queue, search_attributes)
SELECT
  md5((g % 20)::text),
  md5(g::text),
  1,
  timestamp '2026-01-01' + (g % 86400) * interval '1 second',
  timestamp '2026-01-01' + (g % 86400) * interval '1 second',
  'workflow-' || g,
  'type-' || (g % 10),
  1 + g % 7,
  CASE WHEN g % 7 = 0 THEN NULL ELSE timestamp '2026-01-01' + (g % 86400) * interval '1 second' + interval '1 minute' END,
  g % 10000,
  g % 100000,
  g % 3600,
  g % 100,
  NULL,
  'json/plain',
  'queue-' || (g % 10),
  jsonb_build_object('Keyword01', 'tenant-' || (g % 100), 'Keyword02', 'prefix-' || (g % 1000), 'KeywordList01', jsonb_build_array('tag-' || (g % 20)), 'Text01', CASE WHEN g % 10 = 0 THEN 'hello world' ELSE 'ordinary document' END, 'Int01', g % 1000, 'Bool01', g % 2 = 0)
FROM generate_series(1, 200000) g;
ANALYZE executions_visibility;
