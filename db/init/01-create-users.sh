#!/bin/bash
set -e

# Create application users for tag-generator and pre-processor services
# This script runs during database initialization and has access to environment variables

echo "Creating application users..."

# Create tag-generator user
psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" <<-EOSQL
    DO \$\$
    BEGIN
        IF NOT EXISTS (SELECT FROM pg_catalog.pg_roles WHERE rolname = 'tag_generator') THEN
            CREATE ROLE tag_generator LOGIN PASSWORD '${DB_TAG_GENERATOR_PASSWORD}';
        END IF;
    END
    \$\$;

    -- Grant basic database connection privileges
    GRANT CONNECT ON DATABASE ${POSTGRES_DB} TO tag_generator;
    GRANT USAGE ON SCHEMA public TO tag_generator;
EOSQL

# Create pre-processor user
psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" <<-EOSQL
    DO \$\$
    BEGIN
        IF NOT EXISTS (SELECT FROM pg_catalog.pg_roles WHERE rolname = 'pre_processor_user') THEN
            CREATE ROLE pre_processor_user LOGIN PASSWORD '${PRE_PROCESSOR_DB_PASSWORD}';
        END IF;
    END
    \$\$;

    -- Grant basic database connection privileges
    GRANT CONNECT ON DATABASE ${POSTGRES_DB} TO pre_processor_user;
    GRANT USAGE ON SCHEMA public TO pre_processor_user;
EOSQL

echo "Application users created successfully."