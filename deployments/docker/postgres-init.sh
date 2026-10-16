#!/bin/sh
# Creates the RLS-enforced application role. Runs once, on first container
# start (docker-entrypoint-initdb.d), as the postgres superuser.
#
# The application connects as this role and therefore CANNOT bypass
# Row-Level Security: it does not own the tables. Migrations keep using the
# owner role.
#
# POSIX sh: the postgres:*-alpine image has no bash.
set -eu

APP_USER="${POSTGRES_APP_USER:-wallet_app}"
APP_PASSWORD="${POSTGRES_APP_PASSWORD:-wallet_app_dev_password}"

psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" <<EOSQL
DO \$\$
BEGIN
  IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = '${APP_USER}') THEN
    CREATE ROLE ${APP_USER} LOGIN PASSWORD '${APP_PASSWORD}';
  ELSE
    ALTER ROLE ${APP_USER} WITH LOGIN PASSWORD '${APP_PASSWORD}';
  END IF;
END
\$\$;

GRANT CONNECT ON DATABASE ${POSTGRES_DB} TO ${APP_USER};
GRANT USAGE ON SCHEMA public TO ${APP_USER};
EOSQL

echo "postgres-init: application role '${APP_USER}' ready"
