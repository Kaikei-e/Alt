# Atlas Database Migrations for Alt

Database schema migration management for `alt-db` using Atlas CLI and Docker Compose.

## Overview

Alt uses Atlas CLI to manage versioned PostgreSQL migrations. Migrations run as a short-lived container task (`migrate` service) orchestrated via Docker Compose (`compose/core.yaml` and `compose/dev.yaml`).

Key features:
- **Atlas CLI Integration**: Declarative and versioned PostgreSQL schema migration management
- **Compose-First Lifecycle**: The `migrate` container depends on `db` being healthy before executing `migrate.sh apply`
- **Transaction Safety**: Migrations run inside transactions (`--tx-mode=file`); do not use `CONCURRENTLY` (Atlas policy uses standard `CREATE INDEX` for transaction safety)
- **Security**: Database credentials supplied via Docker secrets (`postgres_password`) or `.env` in development

## Architecture & Flow

```
compose up → db (healthy) → migrate (db_migrator executes migrate.sh apply) → alt-data-hub (starts)
```

In `compose/core.yaml`:
- Service: `migrate` (`container_name: db_migrator`)
- Dockerfile: `docker/Dockerfile`
- Volume: `../migrations-atlas/migrations:/migrations:ro`
- Entrypoint: constructs `DATABASE_URL` from credentials secret and executes `/scripts/migrate.sh "$@"` with command `["apply"]`
- Depends on: `db` (`condition: service_healthy`)

## Quick Start

### 1. Run Migrations with Docker Compose

```bash
# Apply pending migrations
docker compose -f compose/compose.yaml -p alt run --rm migrate

# Check migration status
docker compose -f compose/compose.yaml -p alt run --rm migrate status

# Check migration syntax offline
docker compose -f compose/compose.yaml -p alt run --rm migrate syntax-check
```

### 2. Run Migrations Locally (Direct Script)

```bash
export DATABASE_URL="postgres://user:pass@localhost:5432/alt_db?sslmode=disable"

# Check status
./docker/scripts/migrate.sh status

# Validate migration files
./docker/scripts/migrate.sh validate

# Apply migrations
./docker/scripts/migrate.sh apply
```

## Migration Development

### Adding New Migrations

1. **Create SQL file** in `migrations/` directory following timestamp format:
   ```sql
   -- 20260901000100_add_new_feature.sql
   -- Migration: Add new feature table
   -- Atlas Version: v0.35+
   
   CREATE TABLE IF NOT EXISTS new_feature (
       id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
       name TEXT NOT NULL,
       created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
   );
   
   CREATE INDEX IF NOT EXISTS idx_new_feature_name ON new_feature(name);
   ```

2. **Validate and lint**:
   ```bash
   ./docker/scripts/migrate.sh validate
   ```

3. **Re-hash migration directory**:
   When new migration files are added or updated, ensure the migration hash is refreshed if using Atlas directory integrity verification:
   ```bash
   atlas migrate hash --dir "file://migrations"
   ```

### Migration Best Practices

#### DO
- Use timestamp-based naming: `YYYYMMDDHHMMSS_description.sql`
- Add descriptive comments
- Use `IF NOT EXISTS` for idempotent schema operations
- Test migrations against a clean local database before committing

#### DON'T
- Use `CONCURRENTLY` operations inside transactional migrations
- Modify existing migration files that have already been applied to upstream environments
- Hardcode credentials or production connection strings

## Related Documentation

- [Project CLAUDE.md](../CLAUDE.md)
- [alt-backend Architecture](../docs/services/alt-backend.md)