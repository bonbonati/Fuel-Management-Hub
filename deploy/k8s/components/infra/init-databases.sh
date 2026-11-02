#!/bin/sh
# Creates one database and one owner role per service (database-per-service).
# Runs once, on the first start of an empty Postgres volume
# (docker-entrypoint-initdb.d). Passwords come from <SERVICE>_DB_PASSWORD.
set -eu

for svc in auth fleet station payment fueling; do
  var="$(echo "$svc" | tr '[:lower:]' '[:upper:]')_DB_PASSWORD"
  pass="$(printenv "$var" || true)"
  if [ -z "$pass" ]; then
    echo "init-databases: $var is not set" >&2
    exit 1
  fi
  psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname postgres \
       -v svc="$svc" -v pass="$pass" <<'SQL'
CREATE ROLE :"svc" LOGIN PASSWORD :'pass';
CREATE DATABASE :"svc" OWNER :"svc";
REVOKE ALL ON DATABASE :"svc" FROM PUBLIC;
SQL
  echo "init-databases: created database and role '$svc'"
done
