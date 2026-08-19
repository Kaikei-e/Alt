---
title: Operational surface budget — P2-14 OSU freeze
date: 2026-08-18
tags:
  - runbook
  - compose
  - ci
  - alt
---

# Operational surface budget

P2-14: a new long-running compose unit is inventory debt. CI freezes
accidental operational surface and requires an offset or a dated
exception before anything new lands. Compose is the source of names;
`scripts/ops-surface-inventory.yaml` holds only metadata.

Sibling of [[compose-bind-mount-policy]] (PM-036 / P2-13). Wave 1b moved
two chown-only inits onto `pre_start` (Compose v5.5.0 green). Wave 4 added
14 parent cert chowns on the same hook (16 `pre_start` total). Remaining
`service_completed_successfully` edges stay on the
`scripts/ops-surface-baseline.json` allowlist — migrators, bootstrap,
and oauth-token-init are not Wave 1b.

## TL;DR

```bash
python3 scripts/tests/test-compose-ops-surface-audit.py
python3 scripts/compose-ops-surface-audit.py
```

Baseline 2026-08-19 (final in-process PKI cutover, 0 workload sidecars):
**77** declared / **63** long-running OSU / **10** ephemeral / **4**
profiled. Accidental OSU cap **16** (`*-logs` only; pki-agent fleet 0).
Caps do not rise. The previous mixed-mode pin was 88 / 74 / 27.

## Unit of account

**OSU** = one long-running service key in the production include chain
(`compose/compose.yaml`). Sidecar instances count even when they share
an image. Excluded: networks / volumes / secrets, class `ephemeral`,
profiled opt-in services.

| Class | Meaning | Seed |
|---|---|---|
| essential | Removing it would break an accepted invariant (app or store) | 37 |
| accidental | Per-app sidecar or leftover after a cutover | 16 |
| platform | Shared X-as-a-Service (prometheus, step-ca, pgbouncer, …) | 10 |
| ephemeral | Runs to completion; not on-call surface | 14 (10 oneshots + 4 profiled) |

## Fitness functions (CI)

Wired in `.github/workflows/compose-audit.yaml` job `ops-surface-invariant`.

| ID | Fail when |
|---|---|
| F1 | Long-running compose service missing from inventory, or stale inventory row |
| F2 | Accidental OSU without a live exception exceeds cap 16 |
| F3 | New long-running unit has neither `offset_of` (a removed unit) nor an unexpired exception + sunset |
| F4 | `exception` set and sunset missing, malformed, or `≤ today` |
| F5 | `kind: sidecar-*` whose parent is not a live compose service (`network_mode: service:X`, `TARGET_SERVICE`, or `CERT_SUBJECT`) |

`--write-baseline` refreshes mechanical name counts. It does **not**
raise `accidental_osu_baseline` or `long_running_osu_baseline`.

## Adding a service

1. Prefer not adding a process. Inverse ISH: which existing unit could own this as a module?
2. If it must exist: add the compose service, then a metadata row in `scripts/ops-surface-inventory.yaml`.
3. Either remove another long-running unit and set `offset_of: <removed>`, or set a closed exception + `sunset:` (ISO date).
4. Accidental multiplier (`*-logs`; leftover pki-agent is forbidden) counts against the cap even when the app itself is essential.
5. Run the two commands in TL;DR. Also update `scripts/ops-surface-baseline.json` only after F1–F5 pass.

Closed exception codes: `X-SECURITY-BOUNDARY`, `X-REPLACE-IN-FLIGHT`,
`X-TRIAL`, `X-PLATFORM-SHARED`, or an ADR number (`000954` / `ADR-000954`).

## How `pki-agent-example` fails

A PR that only adds `pki-agent-example` to compose:

- **F1** — not in the inventory
- **F3** — new long-running unit, no `offset_of` / exception
- Wave 0 name drift vs `ops-surface-baseline.json`

If the PR also adds an accidental inventory row without exception:

- **F2** — accidental OSU 17 > cap 16
- **F5** — `sidecar-cert` / `sidecar-netns` whose parent `example` is not in compose

The merge-unblocking path is retiring an accidental sidecar in the same
PR (`offset_of`) or attaching an unexpired exception with a sunset, not
raising the cap.

## Related

- [[pki-agent-recovery]] — leftover `pki-agent-*` is a dual-writer **incident**, not OSU budget. Accidental OSU is the 16 `*-logs` sidecars
- `scripts/compose-init-edge-audit.py` — PM-037 init-edge allowlist（**22** remaining after Wave 1b + PKI `step-ca-bootstrap` edges）
- `scripts/compose-pre-start-audit.py` — **16** `pre_start` blocks: 2 Wave 1b model chowns + 14 Wave 4 cert chowns
- `docs/review/postmortem-structural-analysis-2026-08-16.md` P2-14
- [[000954]] — essential process split that *also* added accidental multipliers
- [[000747]] — pki-agent sidecar introduction（現行の実行場所は [[000978]]）
- [[000978]] — in-process enrollment、sidecar 0、deploy/rollback ロック
- [[000979]] — file-bind / pre_start / OSU freeze の CI 契約
