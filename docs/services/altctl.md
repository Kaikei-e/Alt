# altctl

_Last reviewed: September 5, 2026_

**Location:** `altctl/`

## Purpose

Go CLI tool for Alt platform Docker Compose orchestration. Provides stack-based management with automatic dependency resolution, feature-based dependency warnings, and structured error output. Built with Cobra/Viper on Go 1.26.

## Cobra CLI Architecture

```
main.go
  cmd/root.go          rootCmd (global flags: --verbose, --dry-run, --quiet, --color, --config, --project-dir)
    cmd/init.go        initCmd      (bootstrap .env/secrets/migrations)
    cmd/seed.go        seedCmd      <dev|e2e>
    cmd/up.go          upCmd        [stacks...]
    cmd/down.go        downCmd      [stacks...]
    cmd/restart.go     restartCmd   [stacks...]
    cmd/rebuild.go     rebuildCmd   <service|stack> [more...]
    cmd/doctor.go      doctorCmd    [stack...]     (read-only diagnosis)
    cmd/status.go      statusCmd
    cmd/logs.go        logsCmd      <service|stack>
    cmd/list.go        listCmd      (alias: ls)
    cmd/build.go       buildCmd     [stacks...]
    cmd/exec.go        execCmd      <service> -- <command...>
    cmd/config.go      configCmd
    cmd/version.go     versionCmd
    cmd/completion.go  completionCmd [bash|zsh|fish|powershell]
    cmd/docs.go        docsCmd      (hidden)
    cmd/home.go        homeCmd           Knowledge Home operations
      cmd/home_health.go     home health
      cmd/home_slo.go        home slo
      cmd/home_flags.go      home flags
      cmd/home_reproject.go  home reproject [start|status|compare|swap|rollback]
      cmd/home_snapshot.go   home snapshot [list|latest|create]
      cmd/home_retention.go  home retention [run|status|eligible]
      cmd/home_storage.go    home storage
      cmd/home_audit.go      home audit
      cmd/home_backfill.go   home backfill [trigger|status|pause|resume]
    cmd/migrate.go     migrateCmd
      cmd/migrate_snapshot.go migrate snapshot
      cmd/migrate_backup.go   migrate backup
      cmd/migrate_restore.go  migrate restore
      cmd/migrate_list.go     migrate list
      cmd/migrate_verify.go   migrate verify
      cmd/migrate_status.go   migrate status
```

Internal packages:

| Package | Responsibility |
|---------|---------------|
| `internal/stack` | Stack registry derived from `compose/*.yaml` + `.altctl.yaml` semantics, dependency resolution, feature warnings, aggregate-file detection |
| `internal/compose` | Docker Compose client (exec, up, down, ps, build, logs) |
| `internal/config` | Viper-based configuration loading (.altctl.yaml) |
| `internal/output` | Printer, table rendering, colored output, structured CLIError |
| `internal/migrate` | Volume backup/restore (pg_dump, tar) for migration |
| `internal/setup` | `altctl init`: prerequisite checks, `.env`/secret generation, Atlas checksum regen |
| `internal/health` | Compose `ps` polling / Ready-wait used by `up`, `restart`, `rebuild` |
| `internal/doctor` | Read-only diagnosis behind `altctl doctor` (root-cause via `depends_on`, config landmine checks) |
| `internal/adminclient` | HTTP client for alt-backend's Knowledge Home admin API (Connect-RPC over HTTP/1.1 + JSON, default `http://localhost:9102`). Auth is at the network layer (internal listener, 127.0.0.1-bound) — this client sends no service-token header |
| `internal/sovereignclient` | HTTP client for knowledge-sovereign's metrics API (`home snapshot`/`retention`/`storage`, default `http://localhost:9511`) |

## Stack Registry (22 stacks, derived from compose/*.yaml)

The stack list is **not** a hardcoded Go table. `internal/stack.NewRegistry` derives
it from `compose/*.yaml` on disk (stack name = filename stem, services = that
file's own `services:` keys) and layers `.altctl.yaml`'s `stacks:` section on top
for what YAML can't express (`depends_on`, `optional`, `requires_gpu`,
`startup_timeout`, `provides`/`requires_features`). See `.altctl.yaml` for the
authoritative dependency graph and `compose/*.yaml` for the authoritative service
lists; run `altctl list --services` for the live, derived output.

| Stack | Optional | Notes |
|-------|----------|-------|
| base | no | Shared secrets/networks/volumes; no services of its own |
| db | no | PostgreSQL, Meilisearch, ClickHouse, pre-processor-db(+migrator) |
| pgbouncer | no | Connection pooling for the main DB and Kratos DB |
| auth | no | Kratos, kratos-db, auth-hub |
| sovereign | no | Knowledge Sovereign (durable knowledge state owner) |
| core | no | plecto-proxy, alt-frontend-sv, alt-backend, alt-harvester, alt-notifier, alt-data-hub, migrate; `requires_features: [search, bff]` |
| workers | no | pre-processor-sidecar, search-indexer, tag-generator, auth-token-manager, etc. |
| mq | yes | Redis Streams, mq-hub |
| ai | yes (GPU) | Ollama, news-creator, pre-processor; 1200s startup timeout |
| recap | yes | recap-db, recap-worker/subworker, dashboard, evaluator; 1200s startup timeout |
| logging | yes | rask-log-aggregator + log forwarders |
| rag | yes | rag-db, rag-orchestrator |
| observability | yes | Prometheus, Alertmanager, Grafana, cAdvisor |
| bff | yes | alt-butterfly-facade |
| perf | yes | alt-perf (Deno/Astral) + k6 |
| backup | yes | restic-backup |
| dev | yes | Local-dev stack (mock-auth + alt-frontend-sv + backend + db); overlaps `core`, never combined with it |
| frontend-dev | yes | Frontend-only dev (mock backend, no database) |
| load-test | yes | Mock RSS server for k6; not self-sufficient alone (`k6` needs `alt-backend` from `core`) |
| pact | yes | Pact Broker for CDC testing |
| acolyte | yes | Acolyte knowledge orchestrator (own Postgres DB) |
| pki | yes | Internal mTLS PKI: `step-ca` + `step-ca-bootstrap` only. Workload enrollment is in-process on 14 parent services (`PKI_ENROLLMENT=enabled`, [[000978]]), not a compose sidecar — do not re-add `pki-agent-*` services |

`compose.dev.yaml` and `compose.staging.yaml` are overlays (declared under
`.altctl.yaml`'s `overlays:` key), not stacks in their own right.

## Stack Dependency Resolution

The `DependencyResolver` in `internal/stack/dependency.go` uses topological sort (depth-first) over the derived `Registry` to ensure stacks start in the correct order. Key behaviors:

- **`Resolve(names)`** -- Walks `DependsOn` recursively, returning stacks in dependency-first order. Used by `up`, `restart`, `build`.
- **`ResolveReverse(names)`** -- Returns stacks in reverse dependency order for graceful shutdown.
- **`ResolveWithDependents(names)`** -- Finds all stacks that transitively depend on the given stacks. Used by `down --with-deps`.
- **`DetectCycles()`** -- Kahn's algorithm cycle detection over the full stack graph.
- **`GetDependencyGraph()`** -- Returns the full dependency map for `list --deps` visualization.

Example: `altctl up ai` pulls in `ai`'s full dependency closure per `.altctl.yaml` — `base`, `db`, `pgbouncer`, `mq`, `sovereign`, `auth`, `core` — topologically sorted rather than one fixed chain, before `ai` itself.

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
# Bootstrap
altctl init                        # Prereq checks, .env/secrets, Atlas checksums
altctl init --force                # Overwrite existing .env and secrets
altctl seed dev                    # Load dev seed data (db/seeds/dev-comprehensive.sql)
altctl seed e2e                    # Load E2E seed data (db/seeds/e2e-integration.sql)

# Stack lifecycle
altctl up [stacks...]              # Start (deps auto-resolved), wait until every service is Ready
altctl up --all                    # Start all stacks including optional
altctl up ai --build               # Start with image rebuild
altctl up core --no-deps           # Start without dependency resolution
altctl up ai --detach              # Fire-and-forget: skip the Ready-wait
altctl down [stacks...]            # Stop
altctl down --volumes              # Stop and remove volumes
altctl down db --with-deps         # Stop db and all stacks that depend on it
altctl restart [stacks...]         # Down then up (deps auto-resolved)
altctl restart core --build        # Restart with image rebuild
altctl rebuild alt-backend         # Rebuild image + force-recreate one service only
altctl rebuild core --no-cache     # Rebuild every service in a stack, no build cache
altctl doctor [stack...]           # Read-only diagnosis: state, root cause, next steps

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
altctl migrate snapshot            # Quick DB-only hot backup
altctl migrate backup              # Full backup of all persistent volumes
altctl migrate restore --from DIR  # Restore from backup
altctl migrate list                # List available backups
altctl migrate verify --backup DIR # Verify backup integrity
altctl migrate status              # Show migration status

# Knowledge Home (alt-backend admin API, default http://localhost:9102 via --backend-url)
altctl home health                               # Projection health
altctl home slo                                  # Show SLO status (SLI Name, Current, Target, Status, Budget Used)
altctl home flags                                # Show feature flag configuration
altctl home reproject start --mode [dry_run|shadow|live] --from V --to V  # Start reproject
altctl home reproject status --run-id UUID       # Query run status
altctl home reproject compare --run-id UUID      # Compare projection versions
altctl home reproject swap --run-id UUID         # Activate shadow projection
altctl home reproject rollback --run-id UUID     # Revert swapped projection
altctl home audit                                # Run a projection correctness audit
altctl home backfill trigger                     # Trigger a backfill job
altctl home backfill status --job-id UUID        # Check backfill status
altctl home backfill pause / resume --job-id UUID
# home snapshot/retention/storage talk to knowledge-sovereign instead (default http://localhost:9511 via --sovereign-url)
altctl home snapshot list / latest / create
altctl home retention status / eligible / run [--live]
altctl home storage                              # Table storage stats
# No auth flag: both APIs are internal, 127.0.0.1-bound listeners — auth is at the network layer, not a token altctl sends

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

## Known failure patterns

Cross-cutting incident patterns are catalogued in [[crystallized-knowledge]].

- Every `altctl home` admin operation failed → one-character header drift (`Service-Token` vs `X-Service-Token`); service-to-service auth must standardize on a single pattern → [[000618]] [[000622]]. `X-Service-Token` itself was later dropped entirely for this API: `internal/adminclient` now sends no auth header at all, relying on alt-backend's admin listener being internal/127.0.0.1-bound instead ([[000743]] removed the header; [[000954]] upholds that decision and adds the loopback-only listener as the replacement control).
- A service silently absent from the running stack → compose file include omissions are silent (sovereign.yaml sat unincluded with profiles) → [[000578]]. The stack registry no longer hardcodes a service list to drift from compose files at all — it derives services from `compose/*.yaml` at registry construction time (`internal/stack/registry_sync_test.go`'s `TestRealRepo_NoOrphanComposeFiles`); a compose file with services and no matching `.altctl.yaml` entry auto-registers with a printed notice instead of silently vanishing.
- "Deployed but old behavior persists" → `docker compose up --wait` neither rebuilds nor recreates same-tag containers (pre-build + `--force-recreate` required), and `latest` tags are implicitly pinned at container creation time → [[000761]], PM-2026-005 [[000564]].
- Host port still unbound after restart → `docker compose restart` does not re-bind host ports; containers that failed the bind need `--force-recreate`.
- `up --wait` aborts unexpectedly → `depends_on.condition: service_healthy` pointing at a service without a healthcheck; catch via a compose-config render gate in CI → [[000809]].
- Ad-hoc `docker compose up --build` outside altctl / alt-deploy → bypasses the Pact-gated deploy pipeline and is recorded as a procedure deviation; production rollout goes through the deploy pipeline only → PM-2026-031.

## Common Pitfalls

| Issue | Solution |
|-------|----------|
| Stack dependency errors | Check `.altctl.yaml`'s `stacks:` section (depends_on/optional/provides) |
| Missing services | Verify the stack's compose file under `compose/*.yaml` — services are derived from it live |
| GPU stack fails | Ensure NVIDIA runtime available; `ai` and `recap` stacks have a 20-min (`1200s`) startup timeout |
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
