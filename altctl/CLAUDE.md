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
altctl status          # View status
```

## Stack Quick Reference

| Stack | Key Services |
|-------|--------------|
| db | db, meilisearch, clickhouse |
| auth | kratos, auth-hub |
| core | nginx, alt-frontend, alt-backend |
| workers | search-indexer, tag-generator |
| ai | news-creator, pre-processor |

## TDD Workflow

**IMPORTANT**: Write failing tests BEFORE implementation.

## Critical Rules

1. **TDD First**: No implementation without failing tests
2. **Dependency Resolution**: Stacks auto-start their dependencies
3. **Feature Warnings**: `core` requires `workers` for search
4. **Structured Output**: Support table and JSON formats
