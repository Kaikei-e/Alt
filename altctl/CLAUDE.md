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
```

## Stack Quick Reference

| Stack | Key Services | Optional |
|-------|--------------|----------|
| base | (shared resources) | no |
| db | db, meilisearch, clickhouse | no |
| auth | kratos, auth-hub | no |
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
| backup | restic-backup, postgres-backup | yes |
| dev | mock-auth, alt-frontend-sv, alt-backend | yes |
| frontend-dev | mock-auth, alt-frontend-sv | yes |

## TDD Workflow

**IMPORTANT**: Write failing tests BEFORE implementation.

## Critical Rules

1. **TDD First**: No implementation without failing tests
2. **Dependency Resolution**: Stacks auto-start their dependencies
3. **Feature Warnings**: `core` requires `workers` for search
4. **Structured Output**: Support table and JSON formats
