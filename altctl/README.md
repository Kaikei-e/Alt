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
| `altctl up [stacks...]` | Start stacks with dependency resolution, and **wait until every service is Ready** before reporting success |
| `altctl down [stacks...]` | Stop running stacks (no args = full aggregate teardown; named stacks = scoped stop+remove) |
| `altctl restart [stacks...]` | Restart stacks (down then up), waiting for Ready the same way `up` does |
| `altctl rebuild <service\|stack> [more...]` | Rebuild images and **force-recreate** just the named services/stacks, then wait for Ready |
| `altctl status` | Show service status by stack |
| `altctl list` | List available stacks |
| `altctl logs <service\|stack>` | Stream logs from a service or stack |
| `altctl exec <service> -- <cmd>` | Execute a command in a running container |
| `altctl config` | Show effective configuration |

### Compose invocation strategy: aggregate-file-first

`up`/`restart`/`rebuild`/`logs`/`exec` build their `docker compose -f ...`
file list around `compose/compose.yaml` -- the top-level `include:`
aggregate that combines every "production" stack -- instead of assembling a
narrow per-stack subset. This isn't a style choice: a narrow subset is
**rejected by real `docker compose`**, e.g. what `altctl up core` used to
build (`-f base.yaml -f db.yaml -f pgbouncer.yaml -f auth.yaml -f
sovereign.yaml -f core.yaml`) fails compose project validation, because
several per-stack files transitively `include: pki.yaml`, whose pki-agent
sidecars `depends_on` services scattered across many other stacks. Even a
*single* stack's own file can fail alone for the same reason.

The rule:

- Whenever every stack involved is reachable through `compose.yaml`'s
  `include:` graph, the invocation is `docker compose -f compose/compose.yaml
  -p alt <cmd> <services...>` -- scoped by naming every resolved stack's own
  services explicitly (compose auto-starts each named service's transitive
  `depends_on` beyond what's named, same as running the command by hand).
- `dev`, `frontend-dev`, and `load-test` sit outside that `include:` graph
  (local-dev-only overlays) and are handled per-case, verified against real
  `docker compose ... config`:
  - `dev`/`frontend-dev` must **never** be combined with the aggregate file
    -- `compose.yaml` pulls in `core.yaml`, which redeclares
    `alt-frontend-sv`/`alt-backend` with resource limits that conflict with
    `dev.yaml`'s/`frontend-dev.yaml`'s own redeclaration of the same
    service names. Each already `include: base.yaml` itself, so its own
    file is self-sufficient alone.
  - `load-test` is the opposite case: it is **not** self-sufficient alone
    (`perf.yaml`'s `k6` service `depends_on: alt-backend`, which only
    exists in `core.yaml` -- outside load-test's own dependency closure of
    `base` + `perf` + `load-test`), so it needs the aggregate file **plus**
    `load-test.yaml` layered on top -- exactly the recipe
    `compose/load-test.yaml`'s own header comment documents.
- A stack with a compose `profiles:` gate (e.g. `perf` -> `--profile perf`)
  gets `--profile <p>` added for every profile among the resolved stacks.

`altctl down` (no stack args) is the one exception that stays a real,
unscoped `docker compose -f compose/compose.yaml down` -- there's nothing to
scope to. `altctl down <stack>` and `restart <stack>`'s stop phase use `stop`
+ `rm -f` instead of `down [SERVICES]`: `down [SERVICES]` scopes
containers/networks to the named services but **not** volumes (`-v` still
targets every named volume across the `-f` files, not just the ones the
named services own), so a stack-scoped `down --volumes` could otherwise
delete an unrelated stack's shared volume. `stop` + `rm -f -v` correctly
scope both to just the named services.

**Ready-wait scope**: the Ready-wait target set is always the *requested
(resolved)* stacks' own services -- e.g. `altctl up core` waits for
base/db/pgbouncer/auth/sovereign/core's services, even though `core`'s
`alt-backend` container actually `depends_on` services in `workers`/`mq`
too, started implicitly by compose but never in this wait's scope. On
timeout, the diagnostic points at `altctl doctor` for root-causing across
that boundary.

### `up` / `restart`: trustworthy success

`altctl up` and `altctl restart` don't report success the moment `docker compose
up -d` returns -- that only means the containers were *created*, not that
they're usable. Instead, after compose returns they poll `docker compose ps
--format json` (~every 2s) until every target service is **Ready**
(`internal/health`):

- a service **with** a healthcheck is Ready when `State=running` **and**
  `Health=healthy`;
- a service **without** a healthcheck is Ready as soon as `State=running`;
- a **one-shot** container (a migrator/init job expected to exit) is Ready
  when `State=exited` with `ExitCode=0`, regardless of any healthcheck.

Live per-service progress prints while waiting (e.g. `12/17 Ready — waiting:
alt-backend (starting), rerank-local (health: starting)`); pass `--quiet` to
suppress it. The wait's timeout is the **largest `startup_timeout`** among the
resolved stacks (`.altctl.yaml`; `ai`/`recap` are 1200s) -- not the `--timeout`
flag, which only bounds the `docker compose up`/`down` invocation itself.

On timeout, or if a service never becomes Ready, `altctl up`/`restart` print
the same service-status diagnostic table `up` has always printed on a hard
compose failure, plus a `docker compose logs --tail 20 --no-color` capture for
every not-Ready service, and exit non-zero: **exit code 5** if the wait timed
out, **exit code 3** if `docker compose up` itself failed.

Pass `--detach`/`-d` to `up` to restore the old fire-and-forget behavior:
start the stack and return immediately without waiting for Ready ("started
(detached) — not verified Ready").

Ctrl-C (SIGINT) or SIGTERM cancels the wait (and any in-flight `docker`
subprocess) promptly instead of leaving `altctl` to hang until the timeout;
altctl prints `interrupted — stack may be partially started; run altctl
doctor` and exits non-zero.

### `rebuild`: kill the "forgot --build" pitfall

`altctl rebuild <service|stack> [more...]` exists for the repo's #1
footgun -- changing Go/Rust/TS code, forgetting `--build`, and the old
binary keeps running silently (root `CLAUDE.md` Critical Rule 3). Unlike
`altctl up --build`, which rebuilds and (re)starts an entire stack plus its
dependencies, `rebuild` touches only the services you name:

```bash
altctl rebuild alt-backend             # Rebuild + recreate one service
altctl rebuild core                    # Rebuild + recreate every service in the core stack
altctl rebuild alt-backend migrate     # Multiple targets, mixing services and stacks
altctl rebuild core --no-cache         # Rebuild without the Docker build cache
altctl rebuild core --detach           # Rebuild + recreate, skip the Ready-wait
```

Each argument is resolved as either a stack name (expands to every service
in that stack) or a service name (via the derived stack registry); an
unknown name gets a "did you mean" suggestion drawn from the live stack and
service lists. Under the hood, for exactly the resolved services:

```bash
docker compose build <svcs>
docker compose up -d --no-deps --force-recreate <svcs>
```

`--force-recreate` is not optional: plain `docker compose up` will not
recreate a container whose image tag hasn't changed, so a freshly rebuilt
image with the same tag would otherwise leave the stale container running
untouched (documented failure pattern ADR-000761 / PM-2026-005) -- the exact
silent-old-binary failure this command exists to prevent. `--no-deps` keeps
the blast radius to just the named services, not their dependents or
dependencies.

One-shot services (migrators/init jobs) are valid rebuild targets: Ready for
them means `exited 0`, the same rule `up`/`restart` use. `rebuild` reuses
the same trustworthy-success Ready-wait as `up`/`restart` (see above) --
`--detach` skips it for fire-and-forget use, `--dry-run` prints the
commands without running them, and a not-Ready/timeout failure gets the
same diagnostic table + tail-20 logs + exit code 5/3.

### Migration (Backup/Restore)

| Command | Description |
|---------|-------------|
| `altctl migrate backup` | Create full backup of all volumes |
| `altctl migrate restore` | Restore volumes from backup |
| `altctl migrate list` | List available backups |
| `altctl migrate verify` | Verify backup integrity |

## Stack Reference

Stacks are **derived from `compose/*.yaml`**, not hardcoded: the stack name is the
compose filename stem (`db.yaml` -> `db`), and its services are that file's own
`services:` map keys, read fresh on every `altctl` invocation
(`internal/stack.NewRegistry`). Dependency order, optionality, GPU/timeout
requirements, and feature provide/require relationships -- things a compose file
alone can't express -- are declared in the root `.altctl.yaml`'s `stacks:`
section. See that file for the authoritative list; `altctl list` (or
`altctl list --services --deps`) shows the live, derived result.

| Stack | Dependencies | Optional |
|-------|--------------|----------|
| base | - | no |
| db | base | no |
| pgbouncer | base, db | no |
| auth | base, pgbouncer | no |
| sovereign | base | no |
| core | base, db, auth, sovereign | no |
| workers | base, db, mq, core | no |
| mq | base | yes |
| ai | base, db, mq, core | yes (GPU) |
| recap | base, db, core | yes |
| logging | base, db | yes |
| rag | base, db, core, workers | yes |
| observability | base, db, core | yes |
| bff | base, db, auth, core | yes |
| perf | base, db, auth, core | yes |
| backup | base, db | yes |
| load-test | base, perf | yes |
| pact | base | yes (CI) |
| dev | base | yes |
| frontend-dev | - | yes |
| acolyte | base | yes |
| pki | base | yes |

`compose.dev.yaml` and `compose.staging.yaml` are overlays (they modify an
existing stack's services rather than define a new one) and are declared as such
under `.altctl.yaml`'s `overlays:` key so they're never mistaken for stacks.

## Migration Guide

See [Backup & Restore Runbook](../docs/runbooks/backup-restore.md) for detailed backup and recovery procedures.

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

Configuration is loaded from `.altctl.yaml` in the project root. Besides the
usual project/compose/logging/output settings, it declares the parts of the
stack registry a compose file can't express on its own:

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

# Compose files that overlay an existing stack rather than define a new one
# (never auto-registered as stacks, even though they have a services: key)
overlays:
  - compose.dev.yaml
  - compose.staging.yaml

# Per-stack semantics compose/*.yaml can't express: dependency order,
# optionality, GPU/timeout requirements, feature provide/require.
# Stack name = compose filename stem; services are always derived from
# that file, never listed here. See "Stack Reference" above.
stacks:
  ai:
    depends_on: [base, db, mq, core]
    optional: true
    requires_gpu: true
    startup_timeout: 1200s
    provides: [ai]

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

## `altctl doctor`

`altctl doctor [stack...]` is a **read-only** diagnosis: it never restarts,
recreates, or otherwise changes anything -- it only runs `docker info`,
`docker compose ps`, `docker compose config`, and `docker compose logs`, plus
reads `.env`/`secrets/`/`compose/*.yaml` on disk.

```bash
altctl doctor                # Diagnose the whole running stack
altctl doctor core sovereign # Diagnose just these stacks
altctl doctor --json         # Machine-readable findings
```

With no arguments the scope is every non-optional stack plus any optional
stack that currently has containers; naming stacks narrows the scope to
exactly those.

For each problem service it reports:

- **State**: missing (expected but no container), unhealthy, restarting
  (crash-looping), exited non-zero, or still starting.
- **Evidence**: the last `--tail` (default 30) log lines.
- **Root cause**: if service A is down only because its `depends_on` B is
  down, doctor points at B ("A is ... -- waiting on B (unhealthy), fix that
  first") instead of leaving you to trace it by hand.
- **Config landmines**: `depends_on: {condition: service_healthy}` pointing
  at a service with no `healthcheck:` block is flagged statically, even when
  nothing is currently running.
- **Environment preflight**: docker daemon unreachable (reported loudly, never
  as "no services running"), missing `.env` at the repo root, missing
  `secrets/*.txt` files (compared against `compose/base.yaml`'s `secrets:`
  block), `DOCKER_GROUP_ID` unset when the `logging` stack is in scope.
- **Prescription**: a concrete next command per finding (`altctl logs <svc>
  -f`, `docker compose ... up -d --force-recreate <svc>`, `altctl init`, ...).

Exit codes: `0` nothing wrong, `1` problems found, `3` docker itself is
unreachable.

## License

Licensed under the Apache License 2.0. See the project root [LICENSE](../LICENSE).
