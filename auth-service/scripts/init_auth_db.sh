#!/bin/bash

# Auth Database Initialization Script
# This script initializes the auth-postgres database with the required schema

set -euo pipefail

# Configuration
DB_HOST="${DB_HOST:-auth-postgres}"
DB_PORT="${DB_PORT:-5432}"
DB_NAME="${DB_NAME:-auth_db}"
DB_USER="${DB_USER:-auth_user}"
DB_PASSWORD="${DB_PASSWORD:-}"
MIGRATIONS_DIR="$(dirname "$0")/migrations"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Check if required environment variables are set
if [ -z "$DB_PASSWORD" ]; then
    log_error "DB_PASSWORD environment variable is required"
    exit 1
fi

# Function to check if database is accessible
check_database_connection() {
    log_info "Checking database connection..."
    
    export PGPASSWORD="$DB_PASSWORD"
    
    if pg_isready -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" -d "$DB_NAME" > /dev/null 2>&1; then
        log_info "Database connection successful"
        return 0
    else
        log_error "Cannot connect to database"
        return 1
    fi
}

# Function to run a migration file
run_migration() {
    local migration_file="$1"
    local migration_name=$(basename "$migration_file" .up.sql)
    
    log_info "Running migration: $migration_name"
    
    export PGPASSWORD="$DB_PASSWORD"
    
    if psql -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" -d "$DB_NAME" -f "$migration_file" > /dev/null 2>&1; then
        log_info "Migration $migration_name completed successfully"
        return 0
    else
        log_error "Migration $migration_name failed"
        return 1
    fi
}

# Function to create database if it doesn't exist
create_database_if_not_exists() {
    log_info "Checking if database $DB_NAME exists..."
    
    export PGPASSWORD="$DB_PASSWORD"
    
    # Connect to postgres database to check if auth_db exists
    if psql -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" -d postgres -tc "SELECT 1 FROM pg_database WHERE datname = '$DB_NAME'" | grep -q 1; then
        log_info "Database $DB_NAME already exists"
    else
        log_info "Creating database $DB_NAME..."
        if psql -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" -d postgres -c "CREATE DATABASE $DB_NAME;" > /dev/null 2>&1; then
            log_info "Database $DB_NAME created successfully"
        else
            log_error "Failed to create database $DB_NAME"
            return 1
        fi
    fi
}

# Function to run all migrations
run_all_migrations() {
    log_info "Running all migrations from $MIGRATIONS_DIR"
    
    if [ ! -d "$MIGRATIONS_DIR" ]; then
        log_error "Migrations directory not found: $MIGRATIONS_DIR"
        return 1
    fi
    
    # Find all .up.sql files and sort them
    local migration_files=$(find "$MIGRATIONS_DIR" -name "*.up.sql" | sort)
    
    if [ -z "$migration_files" ]; then
        log_warn "No migration files found in $MIGRATIONS_DIR"
        return 0
    fi
    
    # Run each migration
    for migration_file in $migration_files; do
        if ! run_migration "$migration_file"; then
            log_error "Migration failed, stopping execution"
            return 1
        fi
    done
    
    log_info "All migrations completed successfully"
}

# Function to verify schema
verify_schema() {
    log_info "Verifying database schema..."
    
    export PGPASSWORD="$DB_PASSWORD"
    
    # Check if required tables exist
    local required_tables=("tenants" "users" "user_sessions" "csrf_tokens" "audit_logs" "user_preferences")
    
    for table in "${required_tables[@]}"; do
        if psql -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" -d "$DB_NAME" -tc "SELECT 1 FROM information_schema.tables WHERE table_name = '$table'" | grep -q 1; then
            log_info "Table $table exists"
        else
            log_error "Required table $table does not exist"
            return 1
        fi
    done
    
    log_info "Schema verification completed successfully"
}

# Main execution
main() {
    log_info "Starting Auth Database initialization..."
    log_info "Target: $DB_USER@$DB_HOST:$DB_PORT/$DB_NAME"
    
    # Wait for database to be ready
    local retry_count=0
    local max_retries=30
    
    while [ $retry_count -lt $max_retries ]; do
        if check_database_connection; then
            break
        fi
        
        retry_count=$((retry_count + 1))
        log_info "Waiting for database to be ready... (attempt $retry_count/$max_retries)"
        sleep 2
    done
    
    if [ $retry_count -eq $max_retries ]; then
        log_error "Database connection timeout after $max_retries attempts"
        exit 1
    fi
    
    # Create database if needed
    create_database_if_not_exists
    
    # Run migrations
    run_all_migrations
    
    # Verify schema
    verify_schema
    
    log_info "Auth Database initialization completed successfully!"
}

# Execute main function
main "$@"