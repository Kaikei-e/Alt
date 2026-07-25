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

## Stack Registry (derived, not hardcoded)

`internal/stack.NewRegistry(composeDir, configPath)` derives the stack list from
`compose/*.yaml` on disk instead of a hardcoded Go table:

- **Stack name** = compose filename stem (`db.yaml` -> `db`).
- **Services** = that file's own top-level `services:` map keys, read fresh every
  call (`gopkg.in/yaml.v3`, via `yaml.Node` so anchors/merge keys like
  `pki.yaml`'s `<<: *pki-agent` never need resolving) — so this table can never
  drift from what's actually in compose/.
- **Semantics that YAML can't express** (`depends_on`, `optional`, `requires_gpu`,
  `startup_timeout`, `provides`/`requires_features`) live in the root
  `.altctl.yaml`'s `stacks:` section. A stack declared there with no matching
  compose file is a hard load error (fail-fast, Critical Rule 9). A compose file
  with services and no declared entry auto-registers with defaults
  (`optional: true`, `depends_on: [base]`) and prints a notice — visible, not fatal.
- Files that overlay another stack's services rather than define a new one
  (`compose.dev.yaml`, `compose.staging.yaml`) are listed under `.altctl.yaml`'s
  `overlays:` key so they're never mistaken for stacks.
- `base.yaml` has no services of its own (shared secrets/networks/volumes only) —
  it stays a valid stack (and dependency root) purely because it's declared in
  `.altctl.yaml`, not because it has services.

Current stacks (see `.altctl.yaml` for the authoritative `depends_on`/`optional`/
`provides` per stack; see `compose/*.yaml` for the authoritative service lists):

| Stack | Compose file | Optional |
|-------|--------------|----------|
| base | base.yaml | no |
| db | db.yaml | no |
| pgbouncer | pgbouncer.yaml | no |
| auth | auth.yaml | no |
| sovereign | sovereign.yaml | no |
| core | core.yaml | no |
| workers | workers.yaml | no |
| mq | mq.yaml | yes |
| ai | ai.yaml | yes (GPU) |
| recap | recap.yaml | yes |
| logging | logging.yaml | yes |
| rag | rag.yaml | yes |
| observability | observability.yaml | yes |
| bff | bff.yaml | yes |
| perf | perf.yaml | yes |
| backup | backup.yaml | yes |
| load-test | load-test.yaml | yes |
| pact | pact.yaml | yes (CI) |
| dev | dev.yaml | yes |
| frontend-dev | frontend-dev.yaml | yes |
| acolyte | acolyte.yaml | yes |
| pki | pki.yaml | yes |

Run `altctl list --services` for the live, derived service lists per stack.

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
