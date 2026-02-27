# altctl

_Last reviewed: February 28, 2026_

**Location:** `altctl/`

## Purpose

Go CLI tool for Alt platform Docker Compose orchestration. Provides stack-based management with automatic dependency resolution, feature-based dependency warnings, and structured error output. Built with Cobra/Viper on Go 1.25.

## Cobra CLI Architecture

```
main.go
  cmd/root.go          rootCmd (global flags: --verbose, --dry-run, --quiet, --color, --config)
    cmd/up.go          upCmd        [stacks...]
    cmd/down.go        downCmd      [stacks...]
    cmd/restart.go     restartCmd   [stacks...]
    cmd/status.go      statusCmd
    cmd/logs.go        logsCmd      <service|stack>
    cmd/list.go        listCmd      (alias: ls)
    cmd/build.go       buildCmd     [stacks...]
    cmd/exec.go        execCmd      <service> -- <command...>
    cmd/config.go      configCmd
    cmd/version.go     versionCmd
    cmd/completion.go  completionCmd [bash|zsh|fish|powershell]
    cmd/docs.go        docsCmd      (hidden)
    cmd/migrate.go     migrateCmd
      cmd/migrate_backup.go   migrate backup
      cmd/migrate_restore.go  migrate restore
      cmd/migrate_list.go     migrate list
      cmd/migrate_verify.go   migrate verify
      cmd/migrate_status.go   migrate status
```

Internal packages:

| Package | Responsibility |
|---------|---------------|
| `internal/stack` | Stack definitions, dependency resolution, feature warnings |
| `internal/compose` | Docker Compose client (exec, up, down, ps, build, logs) |
| `internal/config` | Viper-based configuration loading (.altctl.yaml) |
| `internal/output` | Printer, table rendering, colored output, structured CLIError |
| `internal/migrate` | Volume backup/restore (pg_dump, tar) for migration |

## Stack Definitions (16 stacks)

| Stack | Services | Dependencies | Provides | Requires | Optional |
|-------|----------|--------------|----------|----------|----------|
| base | (shared resources) | - | - | - | no |
| db | db, meilisearch, clickhouse | base | database | - | no |
| auth | kratos-db, kratos-migrate, kratos, auth-hub | base | auth | - | no |
| core | nginx, alt-frontend, alt-frontend-sv, alt-backend, migrate | base, db, auth | - | search, bff | no |
| workers | pre-processor-sidecar, search-indexer, tag-generator, oauth-token-init, auth-token-manager | base, db, mq, core | search | - | no |
| ai | redis-cache, news-creator-backend, news-creator, news-creator-volume-init, pre-processor | base, db, mq, core | ai | - | yes (GPU) |
| recap | recap-db, recap-db-migrator, recap-worker, recap-subworker, dashboard, recap-evaluator | base, db, core | recap | - | yes |
| logging | rask-log-aggregator + 13 log forwarders | base, db | logging | - | yes |
| rag | rag-db, rag-db-migrator, rag-orchestrator | base, db, core, workers | rag | - | yes |
| observability | nginx-exporter, prometheus, grafana, cadvisor | base, db, core | observability | - | yes |
| mq | redis-streams, mq-hub | base | mq | - | yes |
| bff | alt-butterfly-facade | base, db, auth, core | bff | - | yes |
| perf | alt-perf, k6 | base, db, auth, core | - | - | yes |
| backup | restic-backup | base, db | - | - | yes |
| dev | mock-auth, alt-frontend-sv, alt-backend, db, migrate | base | - | - | yes |
| frontend-dev | mock-auth, alt-frontend-sv | (none) | - | - | yes |

## Stack Dependency Resolution

The `DependencyResolver` in `internal/stack/dependency.go` uses topological sort (depth-first) to ensure stacks start in the correct order. Key behaviors:

- **`Resolve(names)`** -- Walks `DependsOn` recursively, returning stacks in dependency-first order. Used by `up`, `restart`, `build`.
- **`ResolveReverse(names)`** -- Returns stacks in reverse dependency order for graceful shutdown.
- **`ResolveWithDependents(names)`** -- Finds all stacks that transitively depend on the given stacks. Used by `down --with-deps`.
- **`DetectCycles()`** -- Kahn's algorithm cycle detection over the full stack graph.
- **`GetDependencyGraph()`** -- Returns the full dependency map for `list --deps` visualization.

Example: `altctl up ai` resolves to `base -> db -> mq -> auth -> core -> ai`.

## Feature Dependencies

Feature warnings are separate from hard dependency resolution. A stack's `RequiresFeatures` declares capabilities it needs but does not auto-start the provider. The `FeatureResolver` checks which features are missing and suggests additional stacks.

Currently defined features: `search`, `ai`, `recap`, `rag`, `logging`, `auth`, `database`, `observability`, `mq`, `bff`.

```bash
$ altctl up core

Feature Warnings
  Stack 'core' requires feature 'search' which is not available.
  Suggestion: Also start: workers
  Stack 'core' requires feature 'bff' which is not available.
  Suggestion: Also start: bff

# Full functionality
$ altctl up core workers bff
```

## Commands Reference

```bash
# Stack lifecycle
altctl up [stacks...]              # Start (deps auto-resolved)
altctl up --all                    # Start all stacks including optional
altctl up ai --build               # Start with image rebuild
altctl up core --no-deps           # Start without dependency resolution
altctl down [stacks...]            # Stop
altctl down --volumes              # Stop and remove volumes
altctl down db --with-deps         # Stop db and all stacks that depend on it
altctl restart [stacks...]         # Down then up (deps auto-resolved)
altctl restart core --build        # Restart with image rebuild

# Inspection
altctl status [--json|--watch]     # View service status grouped by stack
altctl logs <service|stack> [-f]   # Stream logs (accepts service or stack name)
altctl logs recap -n 200           # Show last 200 lines from all recap services
altctl list [--services|--deps]    # List stacks (alias: ls)
altctl list --json                 # Machine-readable stack output

# Container interaction
altctl exec <service> -- <cmd...>  # Execute command in running container
altctl exec db -- psql -U postgres # Example: open psql shell
altctl exec alt-backend -- sh      # Example: open shell in backend

# Build
altctl build [stacks...]           # Build images for stacks
altctl build --no-cache --pull     # Force fresh build

# Migration (volume backup/restore)
altctl migrate backup              # Full backup of all persistent volumes
altctl migrate restore --from DIR  # Restore from backup
altctl migrate list                # List available backups
altctl migrate verify --backup DIR # Verify backup integrity
altctl migrate status              # Show migration status

# Utility
altctl config [--json|--path]      # Show current configuration
altctl version [--short|--json]    # Print version and build info
altctl completion [bash|zsh|fish|powershell]  # Generate shell completions
```

### Global Flags

```
--verbose, -v    Verbose output (debug logging)
--dry-run        Show commands without executing
--quiet, -q      Suppress non-error output
--color          Color output: always, auto, never
--config         Config file path (default: .altctl.yaml)
--project-dir    Alt project directory (default: auto-detect)
```

### Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | General error |
| 2 | Usage error (invalid arguments or unknown stack) |
| 3 | Docker Compose error |
| 4 | Configuration error |
| 5 | Timeout |

## Common Pitfalls

| Issue | Solution |
|-------|----------|
| Stack dependency errors | Check `internal/stack/registry.go` definitions |
| Missing services | Verify compose file mapping in registry |
| GPU stack fails | Ensure NVIDIA runtime available; `ai` stack has 10-min timeout |
| Search not working | Start `workers` stack with `core` |
| Feature warning appears | Follow the suggestion to add missing stacks |
| `--quiet` and `--verbose` conflict | These flags are mutually exclusive |
| Partial startup failure | Run `altctl status` to see which services failed |

## References

### Official Documentation
- [Cobra CLI](https://github.com/spf13/cobra)
- [Viper Configuration](https://github.com/spf13/viper)

### Best Practices
- [Claude Code Best Practices](https://www.anthropic.com/engineering/claude-code-best-practices)
