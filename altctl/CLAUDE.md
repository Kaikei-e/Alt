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
altctl init            # First-run bootstrap: prereqs, .env/secrets, Atlas checksums
altctl seed dev        # Load dev seed data (db/seeds/dev-comprehensive.sql)
altctl seed e2e        # Load E2E seed data (db/seeds/e2e-integration.sql)
altctl up              # Start default stacks, wait until every service is Ready
altctl up ai           # Start specific stack (deps auto-resolved), wait for Ready
altctl up ai --detach  # Fire-and-forget: start and return, skip the Ready-wait
altctl down            # Stop all
altctl restart recap   # Restart specific stack, wait until every service is Ready
altctl rebuild alt-backend        # Rebuild image + force-recreate one service, wait for Ready
altctl rebuild core --no-cache    # Rebuild every service in a stack, no Docker build cache
altctl doctor          # Read-only diagnosis: state, root cause, next steps (see below)
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
altctl migrate backup --profile all --force       # Full backup (all 15 volumes)
altctl migrate backup --exclude clickhouse_data   # Exclude specific volumes
altctl migrate restore --from ./backups/xxx --force
altctl migrate restore --from ./backups/xxx --profile db --force
altctl migrate restore --from ./backups/xxx --volumes db_data_17 --force
altctl migrate list                               # List available backups
altctl migrate verify --backup ./backups/xxx      # Verify integrity
altctl migrate status                             # Backup health check
```

## Compose Invocation Strategy (aggregate-file-first)

`up`/`restart`/`rebuild`/`logs`/`exec` (`cmd/compose_target.go`'s
`buildStackInvocation`) build their `-f` list around `compose/compose.yaml`
(the `include:` aggregate) instead of a per-stack subset — a narrow subset
is rejected by real `docker compose` (several per-stack files transitively
`include: pki.yaml`, whose `step-ca-bootstrap` and other cross-file
`depends_on` — e.g. `migrate` → `db` — reach services defined in other
stacks; even a single stack's own file can fail
alone, e.g. `-f core.yaml`: `migrate` depends on undefined service `db`).

- Every stack reachable through `compose.yaml`'s `include:` graph
  (`Stack.AggregateCovered`) → `docker compose -f compose/compose.yaml -p alt
  <cmd> <services...>`, scoped by naming services explicitly (compose
  auto-starts each named service's transitive `depends_on`).
- `dev`/`frontend-dev` (outside the aggregate, and conflict with it —
  `core.yaml` and `dev.yaml` both redeclare `alt-frontend-sv` with
  incompatible resource limits) → their own file alone, never combined with
  the aggregate.
- `load-test` (outside the aggregate, but NOT self-sufficient alone —
  `perf.yaml`'s `k6` depends on `alt-backend`, only defined in `core.yaml`)
  → aggregate file + `load-test.yaml` layered on top.
- A stack with a compose `profiles:` gate gets `--profile <p>` added.
- `down` (no args) stays a real, unscoped `docker compose -f
  compose/compose.yaml down`. `down <stack>`/`restart`'s stop phase use
  `stop` + `rm -f` instead of `down [SERVICES]`, since `down [SERVICES]`
  doesn't scope `-v` to just the named services' own volumes.
- Ready-wait target set is always the *requested (resolved)* stacks' own
  services, never whatever compose auto-started implicitly beyond that — a
  timeout's diagnostic points at `altctl doctor` for root-causing across
  that boundary.

See `internal/doctor/probe.go`'s `aggregateComposeFile` for where this was
first discovered (doctor already probed the aggregate for the same reason)
and `internal/stack/aggregate.go` / `cmd/compose_target.go` for the
implementation.

## `up` / `restart` Reliability (trustworthy success)

`up` and `restart` do not report success just because `docker compose up -d`
returned -- that only means containers were created. They poll `docker
compose ps --format json` (`internal/health.Waiter`) until every target
service is **Ready**:

- healthcheck present -> `State=running` AND `Health=healthy`
- no healthcheck -> `State=running`
- one-shot container (migrator/init job) -> `State=exited` with `ExitCode=0`

Timeout = max `startup_timeout` across the resolved stacks (`.altctl.yaml`;
`ai`/`recap` are 1200s), not the `--timeout` flag. On timeout or a not-Ready
service: diagnostic table + `docker compose logs --tail 20` per not-Ready
service, exit code **5** (timeout) or **3** (compose itself failed). A
timeout's suggestion also points at `altctl doctor`: the Ready-wait target
set is only the requested stacks' own services (Compose Invocation Strategy
above), so a timeout can be caused by a service compose started implicitly
(another stack's dependency) that isn't in this wait's scope at all — doctor
probes the whole aggregate and root-causes across that boundary. Ctrl-C
cancels the wait and in-flight `docker` subprocesses promptly instead of
hanging. `up --detach`/`-d` skips the wait (fire-and-forget).

## `rebuild` (targeted rebuild + force-recreate)

`altctl rebuild <service|stack> [more...]` (`cmd/rebuild.go`) is the fix for
the repo's #1 pitfall: "changed Go/Rust/TS code but forgot `--build`; the
old binary keeps running silently" (root `CLAUDE.md` Critical Rule 3).
Unlike `up --build`, which rebuilds/(re)starts an entire stack plus its
dependencies, `rebuild` touches only the services you name:

```
docker compose build <svcs>
docker compose up -d --no-deps --force-recreate <svcs>
```

- Each arg resolves via the derived stack registry as either a stack name
  (expands to all its services) or a service name (`Registry.FindByService`);
  mixing stacks and services in one invocation is allowed, duplicates across
  args are deduped. An unknown arg gets a Levenshtein "did you mean"
  suggestion drawn from every known stack/service name.
- `--force-recreate` is mandatory, not a flag: plain `docker compose up`
  will not recreate a container whose image tag is unchanged, so a freshly
  rebuilt image with the same tag would otherwise leave the stale container
  running untouched (documented failure pattern ADR-000761 / PM-2026-005) —
  exactly the silent-old-binary failure this command exists to prevent.
- `--no-deps` keeps the blast radius to just the named services.
- One-shot services (migrators/init jobs) are valid targets — Ready means
  `exited 0`, same rule as `up`/`restart` (see `internal/health`).
- Reuses `up`/`restart`'s trustworthy-success Ready-wait and diagnostic
  rendering (`maxStartupTimeout`, `waitForReady`, `classifyServices`,
  `renderReadyFailure` in `cmd/up.go`) against a synthetic per-stack
  `*stack.Stack` whose `Services` is narrowed to only the targeted subset,
  so Ready-wait/diagnostics never wait on services this invocation didn't
  touch. `--no-cache` passes through to `docker compose build`; `--detach`
  skips the Ready-wait; `--dry-run`/`--quiet`/`--verbose`/`--timeout` behave
  like the equivalent `up` flags.

## Backup Profiles

| Profile | Categories | Volumes | Use Case |
|---------|------------|---------|----------|
| db | critical | 7 PG | Quick DB snapshot |
| essential | critical + data + search | 11 | Standard backup (no metrics/models) |
| all | all | 15 | Complete backup (migration) |

## Stack Registry (derived, not hardcoded)

`internal/stack.NewRegistry(composeDir, configPath)` derives the stack list from
`compose/*.yaml` on disk instead of a hardcoded Go table:

- **Stack name** = compose filename stem (`db.yaml` -> `db`).
- **Services** = that file's own top-level `services:` map keys, read fresh every
  call (`gopkg.in/yaml.v3`, via `yaml.Node` so anchors/merge keys
  never need resolving) — so this table can never
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
- **`Stack.AggregateCovered`** — computed once at registry construction by
  reading `compose/compose.yaml`'s own `include:` list (`internal/stack/
  aggregate.go`): true when the stack's compose file is reachable through
  the aggregate. Drives the Compose Invocation Strategy above.
- **`Registry.FindByService`** returns `(*Stack, error)`, deterministically:
  a service declared in more than one stack (e.g. `alt-backend` in both
  `core.yaml` and the local-dev override `dev.yaml`) resolves to the
  `AggregateCovered` stack (`core`, never `dev`) — before this fix it
  iterated a Go map directly and picked either one roughly 50/50 across
  runs. A service ambiguous even after that preference (today: impossible
  for real compose files, since compose itself rejects two stacks declaring
  the same service name in one project) returns an error naming every
  candidate instead of guessing.

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
| health, slo, flags, reproject, audit, backfill | alt-backend :9102 (internal listener, 127.0.0.1-bound) | adminclient (Connect-RPC JSON) |
| snapshot, retention, storage | knowledge-sovereign :9511 | sovereignclient (REST) |

## TDD Workflow

**IMPORTANT**: Write failing tests BEFORE implementation.

## Critical Rules

1. **TDD First**: No implementation without failing tests
2. **Dependency Resolution**: Stacks auto-start their dependencies
3. **Feature Warnings**: `core` requires `workers` for search
4. **Structured Output**: Support table and JSON formats

## `altctl doctor` (read-only diagnosis)

`altctl doctor [stack...]` (`internal/doctor/`, wired in `cmd/doctor.go`)
never mutates state -- only `docker info` / `compose ps` / `compose config` /
`compose logs` and filesystem reads. Default scope: non-optional stacks +
any optional stack with containers; explicit args narrow it.

- **Aggregate probe first**: `ps`/`config` run once against
  `compose/compose.yaml` (the `include:` aggregate), not per-stack `-f`
  combinations -- several per-stack files transitively `include: pki.yaml`,
  whose `step-ca-bootstrap` (and other cross-stack `depends_on`) reach
  services defined in other stacks, so a narrow `-f` subset fails compose project validation even for
  an otherwise-unrelated stack. `dev`/`frontend-dev`/`load-test` aren't in
  the aggregate (local-dev-only); they're probed in isolation via
  `stack.NewDependencyResolver`, and only when explicitly named.
- **Root cause**: walks `depends_on` from `docker compose config
  --format json` to find the deepest broken ancestor of a failing service.
- **Config landmine check**: `depends_on: {condition: service_healthy}` with
  no `healthcheck:` on the target is flagged statically (docs/services/
  altctl.md known failure pattern, [[000809]]), independent of runtime state.
- **DOCKER_GROUP_ID workaround**: `cmd/doctor.go`'s executor injects a
  harmless placeholder for its own read-only calls when the real env var is
  unset (compose/logging.yaml's `${DOCKER_GROUP_ID:?...}` would otherwise
  hard-fail the aggregate probe for users not touching `logging` at all);
  the real unset condition is still separately flagged as a Finding whenever
  `logging` ends up in scope.
- Fully unit-testable against a fake `compose.Executor` (see
  `internal/doctor/doctor_test.go`) -- no live Docker daemon required.
