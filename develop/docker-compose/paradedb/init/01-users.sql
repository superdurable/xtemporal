CREATE ROLE temporal LOGIN PASSWORD 'temporal' CREATEDB;
GRANT pg_signal_backend TO temporal;
