CREATE EXTENSION IF NOT EXISTS pg_search;

CREATE TABLE executions_visibility (
    namespace_id text NOT NULL,
    run_id text NOT NULL,
    workflow_id text NOT NULL,
    workflow_type_name text NOT NULL,
    status integer NOT NULL,
    start_time timestamp NOT NULL,
    close_time timestamp,
    execution_time timestamp NOT NULL,
    sort_close_time timestamp GENERATED ALWAYS AS
        (coalesce(close_time, '9999-12-31 23:59:59'::timestamp)) STORED,
    task_queue text NOT NULL DEFAULT '',
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

CREATE INDEX visibility_core_bm25 ON executions_visibility USING paradedb (
    (namespace_id::pdb.literal),
    (run_id::pdb.literal),
    (workflow_id::pdb.literal),
    (workflow_type_name::pdb.literal),
    status, start_time, close_time, sort_close_time, execution_time,
    (task_queue::pdb.literal), history_length, history_size_bytes,
    execution_duration, state_transition_count,
    (sa_keyword::pdb.literal),
    (sa_text::pdb.whitespace),
    (sa_number::pdb.whitespace('columnar=true')),
    (sa_datetime::pdb.whitespace('columnar=true')),
    (sa_bool::pdb.whitespace('columnar=true'))
);

CREATE INDEX default_idx ON executions_visibility (namespace_id, (COALESCE(close_time, '9999-12-31 23:59:59')) DESC, start_time DESC, run_id);
