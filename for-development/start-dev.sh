#!/bin/bash
# Helper script to start the development environment for alt-frontend-sv

# Determine the directory where this script is located
SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )"
# Project root is one level up
PROJECT_ROOT="$SCRIPT_DIR/.."

# Move to project root context for docker compose
cd "$PROJECT_ROOT"

# Ensure .env exists
if [ ! -f ".env" ]; then
    echo "Creating .env from example..."
    cp .env.example .env
fi

# Generate secrets if they don't exist
if [ ! -d "secrets" ]; then
    echo "Secrets directory not found. Generating dummy secrets..."
    "$SCRIPT_DIR/generate-secrets.sh"
    # Note: generate-secrets.sh writes to current directory (./secrets) which is PROJECT_ROOT now.
fi

# Kratos configuration is generated dynamically from canonical kratos_template.yml
# by entrypoint.sh inside the container on every run, preventing duplicate authoritative drift.

# Configure local dev auth redirection and cookie parameters for canonical entrypoint
export KRATOS_PUBLIC_URL=${KRATOS_PUBLIC_URL:-http://localhost/ory}
export KRATOS_DEFAULT_BROWSER_RETURN_URL=${KRATOS_DEFAULT_BROWSER_RETURN_URL:-http://localhost:4173/sv/}
export KRATOS_LOGIN_UI_URL=${KRATOS_LOGIN_UI_URL:-http://localhost:4173/sv/auth/login}
export KRATOS_REGISTRATION_UI_URL=${KRATOS_REGISTRATION_UI_URL:-http://localhost:4173/sv/register}
export KRATOS_ERROR_UI_URL=${KRATOS_ERROR_UI_URL:-http://localhost:4173/sv/error}
export KRATOS_SETTINGS_UI_URL=${KRATOS_SETTINGS_UI_URL:-http://localhost:4173/sv/settings}
export KRATOS_RECOVERY_UI_URL=${KRATOS_RECOVERY_UI_URL:-http://localhost:4173/sv/recovery}
export KRATOS_VERIFICATION_UI_URL=${KRATOS_VERIFICATION_UI_URL:-http://localhost:4173/sv/verification}
export KRATOS_COOKIE_DOMAIN=""

# Export passwords for Docker Compose variable substitution
if [ -f "secrets/postgres_password.txt" ]; then
    export POSTGRES_PASSWORD=$(cat secrets/postgres_password.txt)
fi
if [ -f "secrets/kratos_db_password.txt" ]; then
    export KRATOS_DB_PASSWORD=$(cat secrets/kratos_db_password.txt)
fi
if [ -f "secrets/db_password.txt" ]; then
    export DB_PASSWORD=$(cat secrets/db_password.txt)
fi
if [ -f "secrets/pre_processor_db_password.txt" ]; then
    export PRE_PROCESSOR_DB_PASSWORD=$(cat secrets/pre_processor_db_password.txt)
fi
# tag_generator_db_password was removed per ADR-000241 / ADR-000397;
# tag-generator no longer accesses alt-db directly.

echo "Starting development environment..."
echo "Services: alt-frontend-sv (Dev Mode), alt-backend, alt-db, plecto-proxy"

docker compose -f compose/compose.yaml -f compose/compose.dev.yaml up -d --build

echo "Development environment started."
echo "Frontend available at: http://localhost/sv/"
echo "Logs: docker compose -f compose/compose.yaml logs -f alt-frontend-sv"
