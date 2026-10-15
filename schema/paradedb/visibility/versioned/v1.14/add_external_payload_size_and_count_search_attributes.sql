ALTER TABLE executions_visibility
  ADD COLUMN IF NOT EXISTS TemporalExternalPayloadSizeBytes BIGINT GENERATED ALWAYS AS ((search_attributes->'TemporalExternalPayloadSizeBytes')::bigint)  STORED,
  ADD COLUMN IF NOT EXISTS TemporalExternalPayloadCount     BIGINT GENERATED ALWAYS AS ((search_attributes->'TemporalExternalPayloadCount')::bigint)      STORED;

-- Drop invalid indices
DO LANGUAGE 'plpgsql' $$
DECLARE
  r RECORD;
BEGIN
  FOR r IN
    SELECT i.relname as indexname
    FROM
      pg_class i,
      pg_index ix
    WHERE
      i.oid = ix.indexrelid
      AND ix.indrelid = (SELECT oid FROM pg_class WHERE relname = 'executions_visibility')
      AND i.relname IN (
        'by_temporal_external_payload_size_bytes',
        'by_temporal_external_payload_count'
      )
      AND NOT ix.indisvalid
  LOOP
    EXECUTE format('DROP INDEX %I', r.indexname);
    RAISE NOTICE 'Dropped invalid index %', r.indexname;
  END LOOP;
END $$;


CREATE INDEX CONCURRENTLY IF NOT EXISTS by_temporal_external_payload_size_bytes ON executions_visibility (namespace_id, TemporalExternalPayloadSizeBytes, (COALESCE(close_time, '9999-12-31 23:59:59')) DESC, start_time DESC, run_id);
CREATE INDEX CONCURRENTLY IF NOT EXISTS by_temporal_external_payload_count      ON executions_visibility (namespace_id, TemporalExternalPayloadCount,     (COALESCE(close_time, '9999-12-31 23:59:59')) DESC, start_time DESC, run_id);

ALTER TABLE executions_visibility ADD COLUMN sort_close_time TIMESTAMP GENERATED ALWAYS AS (coalesce(close_time, '9999-12-31 23:59:59')) STORED;
CREATE INDEX visibility_core_bm25 ON executions_visibility USING paradedb ((namespace_id::pdb.literal), (run_id::pdb.literal), (workflow_id::pdb.literal), (workflow_type_name::pdb.literal), status, start_time, close_time, sort_close_time, execution_time, (task_queue::pdb.literal), history_length, history_size_bytes, execution_duration, state_transition_count, (Text01::pdb.whitespace('lowercase=false')), (Text02::pdb.whitespace('lowercase=false')), (Text03::pdb.whitespace('lowercase=false')), (Keyword01::pdb.literal), (Keyword02::pdb.literal), (Keyword03::pdb.literal), (Keyword04::pdb.literal), (Keyword05::pdb.literal), (Keyword06::pdb.literal), (Keyword07::pdb.literal), (Keyword08::pdb.literal), (Keyword09::pdb.literal), (Keyword10::pdb.literal), (KeywordList01::pdb.literal), (KeywordList02::pdb.literal), (KeywordList03::pdb.literal));
