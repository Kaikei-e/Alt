#!/bin/bash
set -euo pipefail

echo "Configuring PostgreSQL database and service roles..."

PRIMARY_DB="${POSTGRES_DB:-postgres}"

# Some services reference DB_NAME, fall back to POSTGRES_DB when absent
TARGET_DB="${DB_NAME:-$PRIMARY_DB}"
APP_DB_USER="${DB_USER:-}"
APP_DB_PASSWORD="${DB_PASSWORD:-}"

PREPROCESSOR_USER="${PRE_PROCESSOR_DB_USER:-pre_processor_user}"
PREPROCESSOR_PASSWORD="${PRE_PROCESSOR_DB_PASSWORD:-}"

TAG_GENERATOR_USER="${DB_TAG_GENERATOR_USER:-tag_generator}"
TAG_GENERATOR_PASSWORD="${DB_TAG_GENERATOR_PASSWORD:-}"

SEARCH_INDEXER_USER="${SEARCH_INDEXER_DB_USER:-search_indexer_user}"
SEARCH_INDEXER_PASSWORD="${SEARCH_INDEXER_DB_PASSWORD:-}"

# Ensure the target database exists even if the data directory was pre-populated
psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname postgres <<-EOSQL
    DO \$\$
    DECLARE
        db_name text := '${PRIMARY_DB}';
    BEGIN
        IF NOT EXISTS (SELECT 1 FROM pg_database WHERE datname = db_name) THEN
            EXECUTE format('CREATE DATABASE %I', db_name);
        END IF;
    END;
    \$\$;
EOSQL

# Helper to upsert a login role with password
create_or_update_role() {
    local role_name="$1"
    local role_password="$2"

    if [[ -z "$role_name" || -z "$role_password" ]]; then
        return
    fi

    psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$PRIMARY_DB" <<-EOSQL
        DO \$\$
        DECLARE
            target_role text := '${role_name}';
            target_password text := '${role_password}';
        BEGIN
            IF NOT EXISTS (SELECT 1 FROM pg_catalog.pg_roles WHERE rolname = target_role) THEN
                EXECUTE format('CREATE ROLE %I LOGIN PASSWORD %L', target_role, target_password);
            ELSE
                EXECUTE format('ALTER ROLE %I WITH LOGIN PASSWORD %L', target_role, target_password);
            END IF;
        END;
        \$\$;
EOSQL
}

# Grant baseline privileges for service accounts
grant_basic_access() {
    local role_name="$1"

    if [[ -z "$role_name" ]]; then
        return
    fi

    psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$PRIMARY_DB" <<-EOSQL
        DO \$\$
        DECLARE
            target_role text := '${role_name}';
            target_db text := '${TARGET_DB}';
        BEGIN
            IF NOT EXISTS (SELECT 1 FROM pg_catalog.pg_roles WHERE rolname = target_role) THEN
                RETURN;
            END IF;

            EXECUTE format('GRANT CONNECT ON DATABASE %I TO %I', target_db, target_role);
            EXECUTE format('GRANT USAGE ON SCHEMA public TO %I', target_role);
        END;
        \$\$;
EOSQL
}

# Provision the main application user if configured separately from the superuser
if [[ -n "$APP_DB_USER" && -n "$APP_DB_PASSWORD" ]]; then
    echo "Ensuring application role '$APP_DB_USER' exists..."
    create_or_update_role "$APP_DB_USER" "$APP_DB_PASSWORD"

    psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$PRIMARY_DB" <<-EOSQL
        DO \$\$
        DECLARE
            target_role text := '${APP_DB_USER}';
            target_db text := '${TARGET_DB}';
        BEGIN
            IF NOT EXISTS (SELECT 1 FROM pg_catalog.pg_roles WHERE rolname = target_role) THEN
                RETURN;
            END IF;

            EXECUTE format('GRANT CONNECT ON DATABASE %I TO %I', target_db, target_role);
            EXECUTE format('GRANT USAGE ON SCHEMA public TO %I', target_role);
            EXECUTE format('GRANT CREATE, TEMP ON DATABASE %I TO %I', target_db, target_role);
        END;
        \$\$;
EOSQL
fi

echo "Ensuring pre-processor role '${PREPROCESSOR_USER}' exists..."
create_or_update_role "$PREPROCESSOR_USER" "$PREPROCESSOR_PASSWORD"
grant_basic_access "$PREPROCESSOR_USER"

echo "Ensuring tag-generator role '${TAG_GENERATOR_USER}' exists..."
create_or_update_role "$TAG_GENERATOR_USER" "$TAG_GENERATOR_PASSWORD"
grant_basic_access "$TAG_GENERATOR_USER"

echo "Ensuring search-indexer role '${SEARCH_INDEXER_USER}' exists..."
create_or_update_role "$SEARCH_INDEXER_USER" "$SEARCH_INDEXER_PASSWORD"
grant_basic_access "$SEARCH_INDEXER_USER"

echo "Database role provisioning completed."
