# altctl/CLAUDE.md

## Overview

Go CLI tool for Alt platform Docker Compose orchestration. Provides stack-based management with automatic dependency resolution and feature-based dependency warnings.

## Quick Start

```bash
# Build and install
make build && make install-local

# Start default stacks (db, auth, core, workers)
altctl up

# Start specific stack (deps auto-resolved)
altctl up ai

# Stop all
altctl down

# View status
altctl status
```

## Stack Definitions

| Stack | Services | Dependencies | Provides | Requires |
|-------|----------|--------------|----------|----------|
| base | (shared resources) | - | - | - |
| db | db, meilisearch, clickhouse | base | database | - |
| auth | kratos-db, kratos, auth-hub | base | auth | - |
| core | nginx, alt-frontend, alt-backend | base, db, auth | - | search |
| workers | search-indexer, tag-generator | base, db, core | search | - |
| ai | news-creator, pre-processor | base, db, core | ai | - |
| recap | recap-worker, recap-subworker | base, db, core | recap | - |
| logging | rask-log-aggregator | base, db | logging | - |
| rag | rag-orchestrator | base, db, core, workers | rag | - |

## Feature Dependencies

The `core` stack exposes search UI but requires the `workers` stack for search functionality:

```bash
$ altctl up core

Feature Warnings
  Stack 'core' requires feature 'search' which is not available.
  Suggestion: Also start: workers

# Full functionality
$ altctl up core workers
```

## TDD Workflow

**IMPORTANT**: Always write failing tests BEFORE implementation.

```bash
# Run tests
go test ./...

# Format code
make fmt
```

## Critical Guidelines

1. **TDD First**: No implementation without failing tests
2. **Dependency Resolution**: Stacks automatically start their dependencies
3. **Cobra/Viper**: Use standard patterns for CLI and config
4. **Structured Output**: Support both table and JSON formats

## Key Commands

```bash
altctl up [stacks...]          # Start (deps auto-resolved)
altctl down [stacks...]        # Stop
altctl status [--json|--watch] # View status
altctl logs <service> [-f]     # Stream logs
altctl list [--services|--deps]# List stacks
```

## Common Pitfalls

| Issue | Solution |
|-------|----------|
| Stack dependency errors | Check registry.go definitions |
| Missing services | Verify compose.yaml mapping |
| GPU stack fails | Ensure NVIDIA runtime available |
| Search not working | Start workers stack with core |
| Feature warning appears | Follow the suggestion to add missing stacks |

## Appendix: References

### Official Documentation
- [Cobra CLI](https://github.com/spf13/cobra)
- [Viper Configuration](https://github.com/spf13/viper)

### Best Practices
- [Claude Code Best Practices](https://www.anthropic.com/engineering/claude-code-best-practices)
