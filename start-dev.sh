#!/bin/bash
# Helper script to start the development environment for alt-frontend-sv

# Generate secrets if they don't exist
./generate-secrets.sh

echo "Starting development environment..."
echo "Services: alt-frontend-sv (Dev Mode), alt-backend, alt-db, nginx"

docker compose -f compose.yaml -f compose.dev.yaml up -d --build

echo "Development environment started."
echo "Frontend available at: http://localhost/sv/"
echo "Logs: docker compose logs -f alt-frontend-sv"
