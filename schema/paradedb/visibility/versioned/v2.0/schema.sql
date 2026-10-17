-- Dex-profile visibility schema for ParadeDB 0.26.1.
-- Version 2.0 requires a fresh database.

DO LANGUAGE 'plpgsql' $$
BEGIN
  IF (NOT EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'pg_search')) THEN
    CREATE EXTENSION pg_search;
  END IF;
END $$;

CREATE TABLE executions_visibility (
    namespace_id text NOT NULL,
    run_id text NOT NULL,
    workflow_id text NOT NULL,
    workflow_type_name text NOT NULL,
    status integer NOT NULL,
    start_time timestamp NOT NULL,
    close_time timestamp,
    sort_close_time timestamp GENERATED ALWAYS AS
        (coalesce(close_time, '9999-12-31 23:59:59'::timestamp)) STORED,
    namespace_division text,
    flow_type text,
    task_queue text NOT NULL DEFAULT '',
    execution_time timestamp NOT NULL,
    parent_workflow_id text,
    parent_run_id text,
    root_workflow_id text NOT NULL DEFAULT '',
    root_run_id text NOT NULL DEFAULT '',
    history_length bigint,
    history_size_bytes bigint,
    execution_duration bigint,
    state_transition_count bigint,
    memo bytea,
    encoding text NOT NULL DEFAULT 'json/plain',
    _version bigint NOT NULL DEFAULT 0,
    sa_keyword jsonb NOT NULL DEFAULT '{}'::jsonb,
    sa_text jsonb NOT NULL DEFAULT '{}'::jsonb,
    sa_number jsonb NOT NULL DEFAULT '{}'::jsonb,
    sa_datetime jsonb NOT NULL DEFAULT '{}'::jsonb,
    sa_bool jsonb NOT NULL DEFAULT '{}'::jsonb,
    PRIMARY KEY (namespace_id, run_id)
);

CREATE INDEX default_idx ON executions_visibility
    (namespace_id, sort_close_time DESC, start_time DESC, run_id COLLATE "C")
    WHERE namespace_division IS NULL;
CREATE INDEX by_flow_type ON executions_visibility
    (namespace_id, flow_type, sort_close_time DESC, start_time DESC, run_id COLLATE "C")
    WHERE namespace_division IS NULL;
CREATE INDEX by_workflow_id ON executions_visibility (namespace_id, workflow_id);
CREATE INDEX by_namespace_division ON executions_visibility
    (namespace_id, namespace_division, sort_close_time DESC, start_time DESC, run_id COLLATE "C")
    WHERE namespace_division IS NOT NULL;

CREATE INDEX visibility_core_bm25 ON executions_visibility USING paradedb (
    (namespace_id::pdb.literal),
    (run_id::pdb.literal),
    (workflow_id::pdb.literal),
    (workflow_type_name::pdb.literal),
    (namespace_division::pdb.literal),
    (flow_type::pdb.literal),
    status, start_time, close_time, sort_close_time,
    (sa_keyword::pdb.literal),
    (sa_text::pdb.whitespace),
    (sa_number::pdb.whitespace('columnar=true')),
    (sa_datetime::pdb.whitespace('columnar=true')),
    (sa_bool::pdb.whitespace('columnar=true'))
);
