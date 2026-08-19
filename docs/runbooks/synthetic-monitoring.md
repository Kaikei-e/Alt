---
title: Provider-agnostic synthetic monitoring — P2-11
date: 2026-08-18
tags:
  - runbook
  - synthetic
  - observability
  - alt
---

# Provider-agnostic synthetic monitoring

Deep health ([[health-deep-contract]]) answers "can this process reach
its dependencies?". Synthetic monitoring answers "can a user complete an
edge journey?". They are complementary. **In-host blackbox (Prometheus
blackbox_exporter hitting loopback `/health`) does not complete P2-11.**

Machine-readable probe spec: `observability/synthetic/probes.yaml`.
Provider connection is an **ops gate**. This repository does not create
cloud accounts, store probe credentials, or pin a vendor.

Related: [[health-deep-contract]] [[admin-observability]] [[ops-surface-budget]] [[user-journey-slo-gameday]] [[000980]]

## TL;DR

```bash
# The spec is the contract. No provider is wired from this tree.
python3 -c "import yaml; yaml.safe_load(open('observability/synthetic/probes.yaml'))"
```

Status of this wave: **spec + activation checklist only**. External
activation is pending. Do not treat a green in-cluster scrape as
user-journey coverage.

## Why in-host blackbox is insufficient

A probe that originates inside `alt-network` never exercises:

- the published edge (TLS terminator, nginx allowlists, IAP)
- session minting (Kratos flow → cookie → auth-hub `/validate`)
- BFF translation of Connect-RPC / REST for the browser
- body-level assertions ("home returned items", "search returned hits")

Those are exactly the failures that cheap `/health` and even `/health/deep`
stay green through.

## Probe set (edge journeys)

Defined in `observability/synthetic/probes.yaml`. Each probe is an
HTTP journey with status + JSON/body assertions.

| ID | Journey | Success |
|---|---|---|
| `login_session` | session whoami through the edge | 200 and a session / identity object, not an empty 200 |
| `feeds_cursor` | authenticated feeds list | 200 and a JSON array/envelope with `data` or item list |
| `knowledge_home` | Knowledge Home payload | 200 and `items` (or equivalent) present even if empty — schema, not "any 200" |
| `knowledge_search` | search through the edge | 200 and a hits/results field present |

Credentials: **min-privilege**. A dedicated synthetic user with no admin
role, no `/admin/*`, no reproject. Store the secret in the provider, not
in this repo, not in compose env that every service can read.

## Consecutive-failure paging

Do not page on a single miss (deploy blip, one TLS handshake). Page when
**three consecutive** runs of the same probe fail, or when the provider's
availability drops below the probe SLO for one evaluation window.
Journey SLO **alert YAML has landed** (`observability/prometheus/rules/user-journey-slo-alerts.yml`, [[000980]]). This runbook only states the threshold the
spec encodes (`paging.consecutive_failures: 3`). Provider **activation** is still the ops gate (`probes.yaml` `status: spec_only`).

## Activation checklist (ops gate)

Work through this **outside the merge**. Connecting a provider is a
credential and billing decision.

1. Choose a provider (or keep a self-hosted runner **outside** the
   compose network — a second host, not a sidecar). Record the choice
   in the ops vault, not here.
2. Create a min-privilege synthetic identity in Kratos. No admin traits.
   Rotate independently of human accounts.
3. Store the password / session secret in the provider's secret store.
   Never commit it. Never put it in `.env.template`.
4. Point probes at the **public edge** base URL via the provider's
   `{{EDGE_BASE_URL}}` substitution. Do not point them at
   `http://alt-backend:9000` — that is in-host blackbox again.
5. Import `observability/synthetic/probes.yaml` (or the provider's
   translation of it). Confirm body assertions, not status-only.
6. Fire a deliberate fail (wrong password, or a path that 404s) and
   confirm consecutive-failure paging does **not** fire on the first
   miss and **does** fire on the third.
7. Document the on-call route (which provider page, which Slack/mail
   receiver). Journey SLO alerts already live in Prometheus
   (`user-journey-slo-alerts.yml`, [[000980]]); do not
   duplicate those here as YAML.
8. Mark P2-11 synthetic **complete** only after a provider (or
   out-of-host runner) has run green against production-like edge for
   one evaluation window.

Until step 8 is done, P2-11 synthetic coverage is **pending**.

## Heartbeat metric (Prometheus no-observation)

Prometheus `UserJourneyNoObservation` does **not** arm from generic
exporter liveness. The provider-independent contract is:

| Field | Value |
|---|---|
| metric | `alt_synthetic_probe_result` |
| labels | `journey`, `probe` (required) |
| values | `1` success, `0` failure — **any sample is a heartbeat** |
| freshness | 15m (`present_over_time`, not `last_over_time and`) |
| BFF observation journeys | `feeds`, `search` only |

Login whoami is Kratos (`/ory/sessions/whoami`) and never increments the
BFF login counter. A live scrape job must not false-page
`UserJourneyNoObservation{journey="login"}`. Provider-native consecutive-failure
paging for login may still fire.

Export the heartbeat from the runner/provider. Spec:
`heartbeat` + per-probe `bff_observation` in `observability/synthetic/probes.yaml`.

## What this repository will not do

- Create Datadog / Checkly / Grafana Cloud / Pingdom accounts
- Commit API tokens, probe passwords, or production hostnames
- Point compose healthchecks at these journeys
