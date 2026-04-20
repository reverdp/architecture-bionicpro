#!/bin/sh
set -eu

docker-entrypoint.sh postgres &
postgres_pid=$!

cleanup() {
  kill "$postgres_pid" 2>/dev/null || true
}

trap cleanup INT TERM

until pg_isready -h 127.0.0.1 -p 5432 -U "$POSTGRES_USER" -d "$POSTGRES_DB" >/dev/null 2>&1; do
  sleep 1
done

psql -v ON_ERROR_STOP=1 -h 127.0.0.1 -U "$POSTGRES_USER" -d "$POSTGRES_DB" -f /opt/startup/telemetry-seed.sql

wait "$postgres_pid"
