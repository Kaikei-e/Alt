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
| `altctl restart [stacks...]` | Restart stacks (down then up) |
| `altctl status` | Show service status by stack |
| `altctl list` | List available stacks |
| `altctl logs <service\|stack>` | Stream logs from a service or stack |
| `altctl exec <service> -- <cmd>` | Execute a command in a running container |
| `altctl config` | Show effective configuration |

### Migration (Backup/Restore)

| Command | Description |
|---------|-------------|
| `altctl migrate backup` | Create full backup of all volumes |
| `altctl migrate restore` | Restore volumes from backup |
| `altctl migrate list` | List available backups |
| `altctl migrate verify` | Verify backup integrity |

## Stack Reference

| Stack | Services | Dependencies | Optional |
|-------|----------|--------------|----------|
| base | (shared resources) | - | no |
| db | db, meilisearch, clickhouse | base | no |
| auth | kratos-db, kratos-migrate, kratos, auth-hub | base | no |
| core | nginx, alt-frontend, alt-frontend-sv, alt-backend, migrate | base, db, auth | no |
| workers | pre-processor-sidecar, search-indexer, tag-generator, oauth-token-init, auth-token-manager | base, db, core | no |
| ai | redis-cache, news-creator-backend, news-creator, news-creator-volume-init, pre-processor | base, db, core | yes (GPU) |
| recap | recap-db, recap-db-migrator, recap-worker, recap-subworker, dashboard, recap-evaluator | base, db, core | yes |
| logging | rask-log-aggregator, nginx-logs, alt-backend-logs, auth-hub-logs, + 10 forwarders | base, db | yes |
| rag | rag-db, rag-db-migrator, rag-orchestrator | base, db, core, workers | yes |
| observability | nginx-exporter, prometheus, grafana, cadvisor | base, db, core | yes |
| mq | redis-streams, mq-hub | base | yes |
| bff | alt-butterfly-facade | base, db, auth, core | yes |
| perf | alt-perf | base, db, auth, core | yes |
| backup | restic-backup, postgres-backup | base, db | yes |
| dev | mock-auth, alt-frontend-sv, alt-backend, db, migrate | base | yes |
| frontend-dev | mock-auth, alt-frontend-sv | - | yes |

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
| `-q, --quiet` | Suppress non-error output (mutually exclusive with `--verbose`) |
| `--color` | Color output: `always`, `auto` (default), `never`. Respects `NO_COLOR` env |

## Version Info

```bash
altctl version              # Full version info with commit hash and build time
altctl version --short      # Version string only
altctl version --json       # JSON format (useful for CI/CD)
```

## Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | General error |
| 2 | Usage error (invalid arguments, unknown stack) |
| 3 | Docker Compose error |
| 4 | Configuration error |
| 5 | Timeout |

## License

Internal use only - Alt Project
