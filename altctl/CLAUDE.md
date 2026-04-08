# altctl/CLAUDE.md

## Overview

CLI for Alt platform Docker Compose orchestration. **Go**, Cobra/Viper.

> Details: `docs/services/altctl.md`

## Commands

```bash
# Test (TDD first)
go test ./...

# Build
make build && make install-local

# Usage
altctl up              # Start default stacks
altctl up ai           # Start specific stack (deps auto-resolved)
altctl down            # Stop all
altctl restart recap   # Restart specific stack
altctl status          # View status
altctl exec db -- psql -U postgres  # Execute in container
altctl logs recap      # Tail all recap stack logs

# Knowledge Home operations
altctl home health                          # Projection health
altctl home slo                             # SLO status
altctl home reproject start --mode=live     # Start reprojection
altctl home reproject status --run-id=<id>  # Check reproject status
altctl home snapshot list                   # List snapshots
altctl home snapshot create                 # Create snapshot
altctl home retention status                # Retention log
altctl home retention eligible              # Eligible partitions
altctl home retention run                   # Dry-run retention
altctl home retention run --live            # Execute retention
altctl home storage                         # Table storage stats
altctl home audit                           # Run projection audit
altctl home backfill trigger                # Trigger backfill
altctl home backfill status --job-id=<id>   # Check backfill status

# Backup & Restore (migrate)
altctl migrate snapshot                           # Quick DB-only hot backup
altctl migrate backup --force                     # Essential profile (default, no metrics)
altctl migrate backup --profile db --force        # DB-only backup
altctl migrate backup --profile all --force       # Full backup (all 14 volumes)
altctl migrate backup --exclude clickhouse_data   # Exclude specific volumes
altctl migrate restore --from ./backups/xxx --force
altctl migrate restore --from ./backups/xxx --profile db --force
altctl migrate restore --from ./backups/xxx --volumes db_data_17 --force
altctl migrate list                               # List available backups
altctl migrate verify --backup ./backups/xxx      # Verify integrity
altctl migrate status                             # Backup health check
```

## Backup Profiles

| Profile | Categories | Volumes | Use Case |
|---------|------------|---------|----------|
| db | critical | 6 PG | Quick DB snapshot |
| essential | critical + data + search | 10 | Standard backup (no metrics/models) |
| all | all | 14 | Complete backup (migration) |

## Stack Quick Reference

| Stack | Key Services | Optional |
|-------|--------------|----------|
| base | (shared resources) | no |
| db | db, meilisearch, clickhouse | no |
| pgbouncer | pgbouncer, pgbouncer-kratos | no |
| auth | kratos, auth-hub | no |
| sovereign | knowledge-sovereign-db, knowledge-sovereign | no |
| core | nginx, alt-frontend-sv, alt-backend | no |
| workers | search-indexer, tag-generator, auth-token-manager | no |
| ai | redis-cache, news-creator-backend, news-creator, pre-processor | yes (GPU) |
| recap | recap-worker, recap-subworker, dashboard | yes |
| logging | rask-log-aggregator + 13 forwarders | yes |
| rag | rag-orchestrator | yes |
| observability | prometheus, grafana, cadvisor | yes |
| mq | redis-streams, mq-hub | yes |
| bff | alt-butterfly-facade | yes |
| perf | alt-perf | yes |
| backup | restic-backup | yes |
| pact | pact-db, pact-broker | yes (CI) |
| dev | mock-auth, alt-frontend-sv, alt-backend | yes |
| frontend-dev | mock-auth, alt-frontend-sv | yes |

## Home Subcommand Clients

| Subcommand | Target | Client |
|------------|--------|--------|
| health, slo, reproject, audit, backfill | alt-backend :9001 | adminclient (Connect-RPC JSON) |
| snapshot, retention, storage | knowledge-sovereign :9511 | sovereignclient (REST) |

## TDD Workflow

**IMPORTANT**: Write failing tests BEFORE implementation.

## Critical Rules

1. **TDD First**: No implementation without failing tests
2. **Dependency Resolution**: Stacks auto-start their dependencies
3. **Feature Warnings**: `core` requires `workers` for search
4. **Structured Output**: Support table and JSON formats
