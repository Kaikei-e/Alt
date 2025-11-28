#!/bin/sh
set -e

# Read secrets from files (remove trailing newline)
if [ -f /run/secrets/kratos_db_password ]; then
    export KRATOS_DB_PASSWORD=$(cat /run/secrets/kratos_db_password | tr -d '\n')
fi

if [ -f /run/secrets/kratos_cookie_secret ]; then
    export KRATOS_COOKIE_SECRET=$(cat /run/secrets/kratos_cookie_secret | tr -d '\n')
fi

if [ -f /run/secrets/kratos_cipher_secret ]; then
    export KRATOS_CIPHER_SECRET=$(cat /run/secrets/kratos_cipher_secret | tr -d '\n')
fi

# Construct DSN if not already set (or override it to ensure password is used)
# We assume other DSN components are set via env vars or defaults
DB_USER=${KRATOS_DB_USER:-kratos_user}
DB_HOST=${KRATOS_DB_HOST:-kratos-db}
DB_PORT=${KRATOS_DB_PORT:-5432}
DB_NAME=${KRATOS_DB_NAME:-kratos}
DB_SSLMODE=${KRATOS_DB_SSLMODE:-disable}

export DSN="postgres://${DB_USER}:${KRATOS_DB_PASSWORD}@${DB_HOST}:${DB_PORT}/${DB_NAME}?sslmode=${DB_SSLMODE}"

# Expand environment variables in kratos.yml using sed
if [ -f /etc/config/kratos/kratos.yml ]; then
    # Create a temporary file with expanded environment variables
    sed "s|\${KRATOS_COOKIE_SECRET}|${KRATOS_COOKIE_SECRET}|g; s|\${KRATOS_CIPHER_SECRET}|${KRATOS_CIPHER_SECRET}|g; s|\${DSN}|${DSN}|g" /etc/config/kratos/kratos.yml > /tmp/kratos.yml
    # Use the expanded config file
    export KRATOS_CONFIG_FILE=/tmp/kratos.yml
fi

# Execute the passed command, replacing config path if we created a temp file
if [ -n "$KRATOS_CONFIG_FILE" ]; then
    # Build new command with replaced config path
    CMD=""
    SKIP_NEXT=false
    for arg in "$@"; do
        if [ "$SKIP_NEXT" = true ]; then
            SKIP_NEXT=false
            CMD="$CMD $KRATOS_CONFIG_FILE"
            continue
        fi
        if [ "$arg" = "--config" ]; then
            CMD="$CMD --config"
            SKIP_NEXT=true
        elif echo "$arg" | grep -q "^--config="; then
            CMD="$CMD --config=$KRATOS_CONFIG_FILE"
        else
            CMD="$CMD $arg"
        fi
    done
    exec sh -c "$CMD"
else
    exec "$@"
fi
