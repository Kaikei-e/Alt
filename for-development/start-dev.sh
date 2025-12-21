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

# Ensure kratos.yml exists from template
if [ ! -f "kratos/kratos.yml" ]; then
    echo "Creating kratos.yml from template..."
    cp kratos/kratos_template.yml kratos/kratos.yml
fi

# Ensure kratos.yml has correct base_url for dev
if [ -f "kratos/kratos.yml" ]; then
    # Update base_url specifically
    sed -i 's|base_url: https://example.com|base_url: http://localhost|g' kratos/kratos.yml
    
    # Remove example.com from allowed_origins to avoid duplication if localhost is already there
    sed -i '/- https:\/\/example.com/d' kratos/kratos.yml

    # Fix session cookie domain
    sed -i 's|domain: .example.com|# domain: localhost|g' kratos/kratos.yml
    
    # Update other example.com references that should be localhost (e.g. return_url)
    sed -i 's|default_browser_return_url: https://example.com|default_browser_return_url: http://localhost/sv|g' kratos/kratos.yml
    
    # Ensure UI URLs point to /sv/ path
    sed -i 's|ui_url: http://localhost/auth/|ui_url: http://localhost/sv/auth/|g' kratos/kratos.yml
fi

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
if [ -f "secrets/tag_generator_db_password.txt" ]; then
    export DB_TAG_GENERATOR_PASSWORD=$(cat secrets/tag_generator_db_password.txt)
fi

echo "Starting development environment..."
echo "Services: alt-frontend-sv (Dev Mode), alt-backend, alt-db, nginx"

docker compose -f compose.yaml -f compose.dev.yaml up -d --build

echo "Development environment started."
echo "Frontend available at: http://localhost/sv/"
echo "Logs: docker compose logs -f alt-frontend-sv"
