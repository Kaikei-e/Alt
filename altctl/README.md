# altctl

Alt platform orchestration CLI - Docker Compose stacks management with automatic dependency resolution.

## Installation

```bash
cd altctl
go build -o altctl .
sudo mv altctl /usr/local/bin/
```

Or use the Makefile:

```bash
make build && make install-local
```

## Quick Start

```bash
# Start default stacks (db, auth, core, workers)
altctl up

# Start specific stack with auto-resolved dependencies
altctl up ai

# Stop all running stacks
altctl down

# View service status
altctl status

# List available stacks
altctl list
```

## Commands

### Stack Management

| Command | Description |
|---------|-------------|
| `altctl up [stacks...]` | Start stacks with dependency resolution |
| `altctl down [stacks...]` | Stop running stacks |
| `altctl status` | Show service status by stack |
| `altctl list` | List available stacks |
| `altctl logs <service>` | Stream logs from a service |
| `altctl config` | Show effective configuration |

### Migration (Backup/Restore)

| Command | Description |
|---------|-------------|
| `altctl migrate backup` | Create full backup of all volumes |
| `altctl migrate restore` | Restore volumes from backup |
| `altctl migrate list` | List available backups |
| `altctl migrate verify` | Verify backup integrity |

## Stack Reference

| Stack | Services | Dependencies |
|-------|----------|--------------|
| base | (shared resources) | - |
| db | db, meilisearch, clickhouse | base |
| auth | kratos-db, kratos, auth-hub | base |
| core | nginx, alt-frontend, alt-backend | base, db, auth |
| ai | news-creator, pre-processor | base, db, core |
| workers | search-indexer, tag-generator | base, db, core |
| recap | recap-worker, recap-subworker | base, db, core |
| logging | rask-log-aggregator | base, db |
| rag | rag-orchestrator | base, db, core, workers |
| perf | alt-perf | base, db, auth, core |

## Migration Guide

See [Migration Runbook](../docs/altctl-migration-runbook.md) for detailed backup/restore procedures.

### Quick Backup

```bash
# Stop services for consistent backup
altctl down

# Create backup
altctl migrate backup

# Backup location: ./backups/YYYYMMDD_HHMMSS/
```

### Quick Restore

```bash
# List available backups
altctl migrate list

# Verify backup integrity
altctl migrate verify --backup ./backups/20251231_120000

# Restore (requires --force)
altctl migrate restore --from ./backups/20251231_120000 --force

# Restart services
altctl up
```

## Configuration

Configuration is loaded from `.altctl.yaml` in the project root:

```yaml
# Default stacks to start with 'altctl up'
defaults:
  stacks:
    - db
    - auth
    - core
    - workers

# Compose file configuration
compose:
  dir: "compose"

# Output preferences
output:
  colors: true
```

## Global Flags

| Flag | Description |
|------|-------------|
| `--config` | Config file path (default: .altctl.yaml) |
| `--project-dir` | Alt project directory (default: auto-detect) |
| `--dry-run` | Show commands without executing |
| `-v, --verbose` | Verbose output |

## License

Internal use only - Alt Project
