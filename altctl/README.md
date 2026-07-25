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

## License

Licensed under the Apache License 2.0. See the project root [LICENSE](../LICENSE).
