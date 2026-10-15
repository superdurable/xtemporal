#!/usr/bin/env bash
set -euo pipefail

for name in LIMITLESS_HOST LIMITLESS_PORT LIMITLESS_USER LIMITLESS_PASSWORD LIMITLESS_DATABASE; do
  if [[ -z "${!name:-}" ]]; then
    printf '%s is required\n' "$name" >&2
    exit 1
  fi
done

export SQL_HOST="$LIMITLESS_HOST"
export SQL_PORT="$LIMITLESS_PORT"
export SQL_USER="$LIMITLESS_USER"
export SQL_PASSWORD="$LIMITLESS_PASSWORD"
export SQL_DATABASE="$LIMITLESS_DATABASE"
export SQL_PLUGIN=postgres12_limitless
export SQL_CONNECT_ATTRIBUTES=sslmode=require

go run ./cmd/tools/sql setup-schema -v 0.0
go run ./cmd/tools/sql update-schema --schema-name limitless/temporal
LIMITLESS_TEST=aurora LIMITLESS_SQL_TOOL_VERIFY=1 go test -tags test_dep -count=1 ./common/persistence/tests/ -run '^TestLimitlessSQLToolVerifyVersion$'
