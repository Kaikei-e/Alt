#!/bin/bash
set -euo pipefail

echo "Configuring PostgreSQL database and service roles..."

# Helper: Read secret from _FILE env var if available, otherwise use direct env var
# This follows the same pattern as official postgres docker image
read_secret() {
    local var_name="$1"
    local file_var="${var_name}_FILE"

    if [[ -n "${!file_var:-}" && -f "${!file_var}" ]]; then
        cat "${!file_var}"
    else
        echo "${!var_name:-}"
    fi
}

PRIMARY_DB="${POSTGRES_DB:-postgres}"

# Some services reference DB_NAME, fall back to POSTGRES_DB when absent
TARGET_DB="${DB_NAME:-$PRIMARY_DB}"
APP_DB_USER="${DB_USER:-}"
APP_DB_PASSWORD="$(read_secret DB_PASSWORD)"

PREPROCESSOR_USER="${PRE_PROCESSOR_DB_USER:-pre_processor_user}"
PREPROCESSOR_PASSWORD="$(read_secret PRE_PROCESSOR_DB_PASSWORD)"


PRE_PROCESSOR_SIDECAR_USER="${PRE_PROCESSOR_SIDECAR_DB_USER:-pre_processor_sidecar_user}"
PRE_PROCESSOR_SIDECAR_PASSWORD="$(read_secret PRE_PROCESSOR_SIDECAR_DB_PASSWORD)"

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

# Helper to upsert a login role with optional password.
# If password is empty, a NOLOGIN role is created (sufficient for GRANT/OWNER).
create_or_update_role() {
    local role_name="$1"
    local role_password="${2:-}"

    if [[ -z "$role_name" ]]; then
        return
    fi

    if [[ -n "$role_password" ]]; then
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
    else
        psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$PRIMARY_DB" <<-EOSQL
            DO \$\$
            DECLARE
                target_role text := '${role_name}';
            BEGIN
                IF NOT EXISTS (SELECT 1 FROM pg_catalog.pg_roles WHERE rolname = target_role) THEN
                    EXECUTE format('CREATE ROLE %I', target_role);
                END IF;
            END;
            \$\$;
EOSQL
    fi
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
# Skip when DB_USER == POSTGRES_USER: the password is already set via POSTGRES_PASSWORD_FILE
if [[ -n "$APP_DB_USER" && "$APP_DB_USER" != "$POSTGRES_USER" && -n "$APP_DB_PASSWORD" ]]; then
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

# Create migration-referenced roles that are legacy names from earlier schema versions.
# Migrations contain "OWNER TO alt_db_user" and "GRANT ... TO alt_appuser" statements
# that require these roles to exist. They are NOLOGIN roles — no password needed.
for alias_role in alt_db_user alt_appuser; do
    echo "Ensuring migration-referenced role '${alias_role}' exists..."
    create_or_update_role "$alias_role"
    grant_basic_access "$alias_role"
done

echo "Ensuring pre-processor-sidecar role '${PRE_PROCESSOR_SIDECAR_USER}' exists..."
create_or_update_role "$PRE_PROCESSOR_SIDECAR_USER" "$PRE_PROCESSOR_SIDECAR_PASSWORD"
grant_basic_access "$PRE_PROCESSOR_SIDECAR_USER"

# Enable pg_stat_statements extension for query performance analysis
echo "Enabling pg_stat_statements extension..."
psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$PRIMARY_DB" <<-EOSQL
    CREATE EXTENSION IF NOT EXISTS pg_stat_statements;
EOSQL

echo "Database role provisioning completed."
