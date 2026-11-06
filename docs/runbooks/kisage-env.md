---
title: "kisage / LKAL Host Environment Reconciliation & Read-Only Preflight"
date: 2026-11-05
tags:
  - runbook
  - kisage
  - lkal
  - compose
  - security
affected_services:
  - compose
  - alt-backend
  - auth-hub
  - pre-processor
  - search-indexer
  - tag-generator
  - recap-worker
  - recap-subworker
  - recap-evaluator
  - acolyte-orchestrator
  - rag-orchestrator
  - knowledge-sovereign
aliases:
  - kisage-env
---

# Runbook: `kisage` / `LKAL` Host Environment Reconciliation & Preflight

Operational guide for Alt's bounded `kisage` (`v0.1.2`) and official `lkal` (`v0.1`) pilot covering repository declaration verification, read-only `.env` preflight checks, role boundaries between `agy` workers and the host operator, and stateful rollback.

---

## TL;DR Commands

```bash
# 1. Provision pinned official lkal + kisage v0.1.2 into ~/.local/bin
make install-lkal install-kisage

# 2. Offline / local source build override (no network required)
make install-lkal LKAL_SRC_DIR=/path/to/LKAL
make install-kisage KISAGE_SRC_DIR=/path/to/kisage LKAL_SRC_DIR=/path/to/LKAL

# 3. Verify deploy/desired-env.lkal lockstep with deploy/host-prereqs.yaml & ADR-000982 (>= 5s floor)
make kisage-verify
# or directly:
python3 scripts/kisage-env.py verify

# 4. Read-only preflight against an actual dotenv file (shadow check; emits KEY names only)
make kisage-preflight ENV_FILE=.env
# or directly:
python3 scripts/kisage-env.py preflight --env-file .env

# 5. Run the portable isolated acceptance & regression suite (zero writes to repo .kisage/; does not require gitignored .claude/settings.json or .mcp.json)
python3 scripts/tests/test-kisage-env.py

# 6. Explicit read-only structural indicator audit of gitignored host-local agent config (.claude/settings.json & .mcp.json; fails closed if missing; does not verify hook execution or OS isolation)
python3 scripts/kisage-env.py audit-host-config
python3 scripts/tests/test-kisage-env.py --audit-host-config
```

---

## Role Split: `agy` Worker vs. Host Operator

| Actor | Permitted Scope | Prohibited Actions |
| :--- | :--- | :--- |
| **`agy` Worker** (AI coding agent / CI) | Edit repository-owned declarations (`deploy/desired-env.lkal`), helper (`scripts/kisage-env.py`), tests (`scripts/tests/test-kisage-env.py`, `tests/scripts/test_kisage_clean_checkout.py`), and docs. Run read-only `verify`, `preflight`, and isolated temp-dir tests. | **Never** edit `.env` or `.kisage/*` directly (guarded on configured operator hosts by `.claude/settings.json` `PreToolUse` hook). Never run automatic `kisage adopt` or `kisage prune`. Never log or print secret values. |
| **Host Operator** | Owns `.kisage/host.json`, `.kisage/host-env.lkal` (including `/local/*`), `.kisage/state/ownership.json`, live `.env`, and gitignored host-local agent config (`.claude/settings.json`, `.mcp.json`). Executes stateful `kisage plan`, `kisage apply`, `kisage prune`, explicit `kisage explain` + `kisage adopt`, and explicit `audit-host-config`. | Never bypass `K203` (`ModifiedExternally`) or `K206` (`UnownedConflict`) with blind script automation; always inspect `kisage explain` first. |

> **Portable CI vs. Host-Local Structural Audit Scope**:
> - **Portable CI (`python3 scripts/tests/test-kisage-env.py`)** runs in clean Git checkouts without `.claude/settings.json`, `.mcp.json`, `.kisage/`, or `.env`. It exercises real `lkal` and `kisage` CLI/MCP boundaries plus synthetic `audit-host-config` fixtures, and explicitly notes that host-local `.claude/settings.json` and `.mcp.json` were not inspected.
> - **Explicit Host Structural Audit (`python3 scripts/kisage-env.py audit-host-config` or `python3 scripts/tests/test-kisage-env.py --audit-host-config`)** is a read-only structural configuration indicator check on the live workspace's `.claude/settings.json` and `.mcp.json`. It fails closed if either file is missing, malformed, non-object JSON (including `null`), or lacks the expected structural entries (`hooks.PreToolUse` entry matching `Edit` and `Write` with a non-empty `type: "command"` hook command referencing `.kisage/*`; `mcpServers.kisage` with non-empty `command`, `args: ["mcp"]`, and non-empty `env.KISAGE_HOST_CONFIG`). Because `audit-host-config` never executes hook or MCP commands, it checks configuration indicator presence only and **cannot prove runtime hook blocking or OS-level isolation**.

---

## Staged Read-Only Preflight & Explicit Scope Limits

`python3 scripts/kisage-env.py preflight --env-file <path>` is a **read-only shadow gate** invoked explicitly via `make kisage-preflight` (it is **not** wired into `scripts/deploy.sh` and performs no automatic host apply).

1. **Managed-Subset Comparison Only**:
   - Compares only repository-managed keys declared under `/upstreams`, `/mtls_allowlists`, `/rate_limits`, and `/feature_flags` in `deploy/desired-env.lkal`.
   - Unmanaged host secrets and `/local/*` operator keys in `.env` are preserved and ignored unless a line is syntactically malformed or duplicated.
2. **Raw Dotenv File Scope & Managed-vs-Unmanaged Interpolation Policy**:
   - Parses the raw `.env` file on disk: supports unquoted values (including apostrophes `'`), single-quoted literal values (`'...'`), and double-quoted values (`"..."`) with standard backslash escapes (`\"`, `\\`, `\$`, `\n`, `\r`, `\t`).
   - Validates all key names, duplicate keys across all entries, missing `=`, unclosed quotes, and control characters across the entire file.
   - **Managed keys** (`/upstreams`, `/mtls_allowlists`, `/rate_limits`, `/feature_flags`): explicitly rejects unescaped `$` variable interpolation (`$VAR` / `${VAR}`) so repository-managed literals are never ambiguously misparsed.
   - **Unmanaged keys** (host secrets and `/local/*` entries such as `APP_CLICKHOUSE_USER`, `APP_CLICKHOUSE_PASSWORD`, `APP_CLICKHOUSE_DATABASE`, `RECAP_DB_DSN`, `TAG_GENERATOR_SERVICE_TOKEN`): permits valid Compose variable reference syntax (`$VAR`, `${VAR}`, `${VAR:-default}`, `$$`) without attempting shell/Compose evaluation or emitting values, while still rejecting malformed `$` tokens or command substitution.
   - Does **not** evaluate shell environment overrides or Docker Compose effective interpolation.
3. **Does Not Inspect Running Containers (`[[PM-2026-050]]`)**:
   - Neither `kisage plan` (which compares `deploy/desired-env.lkal` against `.kisage/host-env.lkal`) nor `kisage-env.py preflight` (which compares `deploy/desired-env.lkal` against `.env` on disk) inspects running Docker containers.
   - Because `docker compose restart` does not re-read `env_file:` changes (`[[PM-2026-050]]`), always recreate affected containers with `docker compose -f compose/compose.yaml -p alt up -d <service>` and verify live container state via `docker inspect`:
     ```bash
     docker inspect <container> --format '{{range .Config.Env}}{{println .}}{{end}}' | grep -E '^(AUTH_HUB_INTERNAL_URL|RATE_LIMIT_EXTERNAL_API_INTERVAL)='
     ```
4. **Zero Secret Disclosure**:
   - Diagnostics output **only** key names, line numbers, rule IDs, and `LKAL` diagnostic codes (`L001`–`L403`). Neither actual `.env` values nor declaration values nor raw `lkal` CLI source snippets are ever printed.

---

## Seven Acceptance Cases

| Case | Scenario | Expected Behavior | Verified By |
| :--- | :--- | :--- | :--- |
| **1** | Official `lkal json -plain` normalization & `Quantity` literals (`7.5s`, `250ms`) | Block and labeled syntax normalize identically; duplicate keys (`L201`), bare words (`L103`), and missing/untrusted binaries fail closed without regex fallback. | `test-kisage-env.py` §2 |
| **2** | Isolated `init` -> `plan` (exit `2`) -> `apply` (`Converged`) -> second `plan` (exit `0` `NoOp`) -> `render-dotenv` | Converges managed sections in temp sandbox, renders deterministic escaped dotenv lines, rejects newline/control-char injection, and performs zero writes to repo `.kisage/`. | `test-kisage-env.py` §3, §4 |
| **3** | External managed drift (`K203 ModifiedExternally`) | Hand-editing a managed leaf in `host-env.lkal` causes `kisage apply` to halt `Blocked` with `K203`; resolved only after `kisage explain` + explicit `kisage adopt` with `conditionToken` and `ownerRevision`. | `test-kisage-env.py` §6 |
| **4** | Sibling `/local/*` & `###` comment preservation + Scope boundary | `/local/*` leaves and `###` doc comments survive `apply` and `prune`; any attempt to declare `/local/*` in `desired-env.lkal` is blocked by `verify` and by `kisage` with `K206` / `K401`. | `test-kisage-env.py` §4, §6 |
| **5** | Conservative deletion boundary (`apply` vs. `prune`) | Removing a key from `desired-env.lkal` causes `kisage apply` to report `NotConverged` (`K604`) without deleting it; only explicit `kisage prune` removes `Origin=Created` leaves. | `test-kisage-env.py` §6 |
| **6** | Alt semantic & schema invariants (`ADR-000982`) | `RATE_LIMIT_EXTERNAL_API_INTERVAL < 5s` (e.g. `1s`) and mTLS peer allowlist mismatches against `deploy/host-prereqs.yaml` fail `verify` and `preflight`. | `test-kisage-env.py` §3 |
| **7** | Read-only dotenv `preflight` | Detects missing managed keys, value drift, managed-key `$` interpolation, duplicate keys, malformed lines, and invalid declarations while preserving unmanaged secrets (including apostrophes, escaped quotes, and valid Compose `${VAR}` references) and emitting key names only. | `test-kisage-env.py` §5 |

---

## Baseline / Control & Pilot Evaluation Metrics

During staged evaluation, run `make kisage-preflight` alongside the existing baseline checks (`python3 scripts/check-env-overrides.py --env-file .env` and `./scripts/check-compose-variables.sh`) and record:

1. **Misses**: Count of deliberate or accidental managed-key drifts in `.env` or `deploy/desired-env.lkal` not caught by the gate (target: `0`).
2. **False Stops**: Count of valid operator `.env` configurations (unmanaged secrets, `/local/*` overrides, valid quoted/apostrophe syntax, or valid unmanaged Compose `${VAR}` references) erroneously rejected by `preflight` (target: `0`).
3. **Elapsed Time**: Wall-clock execution time of `make kisage-verify` and `make kisage-preflight` (target: `< 2s`).
4. **Manual Recovery Operations**: Count of host operator interventions (`explain` / `adopt` / state backup restores) required to recover from drift or rollback.
5. **Live Raw `.env` Shadow Observation (2026-11-06 JST)**:
   - Read-only `python3 scripts/kisage-env.py preflight --env-file .env` inspects only raw `.env` lines on disk, not Docker Compose effective runtime interpolation (`${VAR:-default}` fallbacks in `compose/*.yaml`).
   - After repairing unmanaged variable reference handling (`0` false stops on unmanaged `APP_CLICKHOUSE_USER`, `APP_CLICKHOUSE_PASSWORD`, `APP_CLICKHOUSE_DATABASE`, `RECAP_DB_DSN`, and `TAG_GENERATOR_SERVICE_TOKEN`), live read-only preflight exits `1` reporting only the `23` absent repository-managed keys in raw `.env` (emitted by key name only, with no values emitted). These keys may be supplied by Compose defaults; this raw-file check does not verify effective Compose or running-container settings and does not establish a live outage.

---

## Host-Side Reconciliation & Stateful Rollback Procedure

> **CRITICAL**: `git revert` on `deploy/desired-env.lkal` reverts **only** the Git declaration. **`git revert` alone does NOT revert `.env`, `.kisage/host-env.lkal`, or `.kisage/state/ownership.json`.** Stateful host rollback requires backing up and restoring the target files and ownership store together.

### 1. Pre-Apply State Backup (Host Operator)

Before running `kisage apply`, `kisage prune`, or `kisage adopt` on the host, **quiesce all `kisage` CLI and MCP processes** (ensure no active `kisage` process holds `.kisage/locks/*`), then archive the **complete `.kisage` directory** (excluding any backup subdirectory) together with `.env` into a `0700` gitignored backup directory:

```bash
umask 077
BACKUP_DIR=".kisage/backups/$(date -u +%Y%m%dT%H%M%SZ)"
mkdir -p "$BACKUP_DIR"
cp -p .env "$BACKUP_DIR/.env.bak"
tar -C .kisage --exclude='./backups' -cf "$BACKUP_DIR/kisage-state.tar" .
chmod 600 "$BACKUP_DIR/.env.bak" "$BACKUP_DIR/kisage-state.tar"
```

> **Note**: A backup archive is bound to the same host, filesystem `deviceId`, `host.json` configuration, and `kisage` version (`v0.1.2`). Do not restore a `.kisage` state archive across different hosts or `kisage` versions.

### 2. Inspect Plan & Apply (Host Operator)

```bash
export KISAGE_HOST_CONFIG="$PWD/.kisage/host.json"

# Inspect diff (exit 0 = NoOp, exit 2 = diff present, exit 1 = blocked/error)
kisage plan deploy/desired-env.lkal -detailed-exitcode

# Apply managed sections (/upstreams, /mtls_allowlists, /rate_limits, /feature_flags)
kisage apply deploy/desired-env.lkal

# Render managed + /local/* projection to a staging file (NOTE: render-dotenv outputs only
# LKAL-declared sections and is NOT a complete replacement for unmanaged secrets in .env;
# required_env completeness in .env remains governed by scripts/check-compose-variables.sh)
python3 scripts/kisage-env.py render-dotenv --target .kisage/host-env.lkal --output .kisage/rendered.env
python3 scripts/kisage-env.py preflight --env-file .env
```

### 3. Handling `K203` (`ModifiedExternally`) Without Auto-Adopt

If `kisage apply` halts with `K203`, **never** automate `adopt`. Inspect the path first:

```bash
kisage explain deploy/desired-env.lkal /rate_limits/RATE_LIMIT_EXTERNAL_API_INTERVAL -json
# Only if the operator confirms the repository declaration should overwrite the manual edit:
kisage adopt deploy/desired-env.lkal /rate_limits/RATE_LIMIT_EXTERNAL_API_INTERVAL \
  -expect <conditionToken> -owner-revision <ownerRevision>
```

### 4. Full Stateful Rollback (Host Operator)

To roll back a host reconciliation while all `kisage` CLI/MCP writers are quiesced:

```bash
# 1. Quiesce kisage CLI / MCP writers, then perform a consistent restore of the complete .kisage state + .env while all writers are quiesced
rm -rf .kisage/state .kisage/locks .kisage/host.json .kisage/host-env.lkal
tar -C .kisage -xf "$BACKUP_DIR/kisage-state.tar"
cp -p "$BACKUP_DIR/.env.bak" .env
chmod 600 .kisage/host.json .kisage/host-env.lkal .env

# 2. Verify declaration lockstep, kisage plan state, and dotenv preflight
make kisage-verify
KISAGE_HOST_CONFIG="$PWD/.kisage/host.json" kisage plan deploy/desired-env.lkal -detailed-exitcode
make kisage-preflight ENV_FILE=.env

# 3. Recreate affected Compose services so containers pick up restored .env
docker compose -f compose/compose.yaml -p alt up -d
```
