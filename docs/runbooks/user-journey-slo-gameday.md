---
title: "User Journey SLO GameDay"
date: 2026-08-18
tags:
  - runbook
  - slo
  - gameday
  - user-journey
---

# User Journey SLO GameDay

Checklist to verify feeds / login / search burn-rate alerting and Knowledge Home `status=` labeling.

Related: [[knowledge-home-gameday-checklist]], [[health-deep-contract]], [[synthetic-monitoring]], [[000418]], [[000420]], [[000969]], [[000980]]

Canonical alerts are Prometheus (`observability/prometheus/rules/user-journey-slo-alerts.yml` and `knowledge-home-slo-alerts.yml`). Grafana Unified Alerting copies of Knowledge Home SLOs are not provisioned (empty groups) so they cannot double-evaluate. Do not re-add them with `noDataState: OK`.

## Instrumentation (edge choice)

Counters are emitted at **alt-butterfly-facade** (BFF), the hop closest to user impact. That process sees JWT 401s, circuit-breaker 503s and upstream 5xx that alt-backend handlers never observe.

- `journey=feeds` — `/alt.feeds.v2.FeedService/*` and REST `/v1/feeds*`
- `journey=search` — `/alt.search.v2.SearchService/*` and `/alt.search.v2.GlobalSearchService/*`
- `journey=login` — session-validation events only (`/sessions/`, `/whoami`, `/ory/…`, plus 401 on any wrapped path). A successful feeds/search/article call does **not** increment login. 401 on feeds/search also records that journey as error so product outages are not hidden. This is **not** the Kratos password form.

Knowledge Home `alt_home_requests_total` is still emitted by alt-backend and now carries `status="ok|error"` so `KnowledgeHomeAvailabilityBurnRateHigh` can match `status="error"`.

Recap is **not** a ratio SLO. Freshness uses `knowledge_event_last_occurrence_age_seconds{event_type="recap.topic_snapshotted.v1"}` (`KnowledgeLoopRecapTopicSnapshotProducerDead`) plus `RecapTopicSnapshotHeartbeatMissing` (`absent()`). Do not invent recap request counters.

## Prerequisites

- `docker compose -f compose/compose.yaml -p alt ps` shows `alt-butterfly-facade`, `prometheus`, `alertmanager` healthy.
- **Rebuild BFF before reloading Prometheus.** Reloading Prometheus first loads `UserJourneyRequestsAbsent` on `alt_user_journey_instrumented`, which pages Emergency after 15m of an empty vector. Order: rebuild `alt-butterfly-facade` → confirm `alt_user_journey_instrumented 1` → `POST http://127.0.0.1:9090/-/reload` (or restart prometheus).
- `curl -s http://127.0.0.1:9250/metrics | grep alt_user_journey_` shows the instrumented gauge; request series appear after traffic, not as boot zeros.
- `curl -s http://127.0.0.1:9090/-/healthy` and `curl -s http://127.0.0.1:9093/-/healthy` succeed.

## Scenario A: Counter scrape

```bash
curl -s http://127.0.0.1:9250/metrics | grep '^alt_user_journey_requests_total'
```

- [ ] `alt_user_journey_instrumented 1` is present (counter is wired). Request series appear only after real Record samples — zeros at boot are not coverage.
- [ ] Prometheus target `alt-butterfly-facade` is UP at `http://127.0.0.1:9090/targets`.

## Scenario B: Burn-rate page (synthetic)

Prefer `promtool test rules observability/prometheus/tests/user-journey-slo-alerts_test.yml` for the 14.4× / spike / recover / observed-failure / absence / 6× / synthetic-gated no-observation contract. A live 14.4× page needs sustained ≥7.2% errors on a journey for both 5m and 1h plus **at least one observed event** — do not force that against a shared environment without a scheduled window.

Live optional (dedicated window only):

1. Stop alt-backend so BFF returns 502 on feeds.
2. Generate failing feeds requests over an hour (or use recording-rule inspection).
3. Confirm `UserJourneyAvailabilityBurnRateHigh{journey="feeds"}` in the Prometheus UI.

- [ ] promtool tests green via `./scripts/observability-validate.sh`.
- [ ] Live 14.4× page: skipped unless a GameDay window is scheduled.

## Scenario C: Absence page

```bash
# Confirm the rule exists, then (GameDay only) pause the BFF scrape target
# or stop alt-butterfly-facade for >15m.
docker compose -f compose/compose.yaml -p alt stop alt-butterfly-facade
```

- [ ] After 15m+2m, `UserJourneyRequestsAbsent` is firing in Prometheus (`alt_user_journey_instrumented` gone, not request zeros).
- [ ] Restart: `docker compose -f compose/compose.yaml -p alt start alt-butterfly-facade`
- [ ] Alert resolves once scrapes resume.

## Scenario D: Knowledge Home status label

```bash
curl -s http://127.0.0.1:9110/metrics | grep alt_home_requests_total
```

(From inside compose: `alt-backend:9110`. Host publish may be absent — use `docker compose exec` if needed.)

- [ ] Series include `status="ok"` and, after a forced handler error, `status="error"`.
- [ ] `KnowledgeHomeAvailabilityBurnRateHigh` can therefore match `status="error"` (the previous unlabeled increment made the page structurally silent).

## Scenario E: Pushover delivery

Prometheus → Alertmanager → Pushover. Solo-ops: `severity: page` is Emergency; `severity: ticket` (6× burn, recap heartbeat missing) is Normal priority and must not break quiet hours.

```bash
# Pipeline liveness (always-firing Watchdog, not a human page)
curl -s http://127.0.0.1:9093/api/v2/alerts | python3 -c \
  'import json,sys; print([a["labels"].get("alertname") for a in json.load(sys.stdin)])'

# Confirm routing without sending a real Emergency
curl -s http://127.0.0.1:9093/api/v2/status
```

To drill delivery without burning a 14.4× window: temporarily fire `UserJourneyRequestsAbsent` (Scenario C) and confirm a Pushover Emergency arrives, then restore the BFF.

- [ ] Alertmanager API reachable.
- [ ] Watchdog is in the Alertmanager payload (deadman route, not the phone).
- [ ] Live Pushover send: **procedure ready, live drill pending** unless this GameDay confirmed a page on the phone. Do not reload Prometheus onto these rules before the BFF rebuild: `UserJourneyRequestsAbsent` would Emergency-page on an empty vector.

## Cleanup

```bash
docker compose -f compose/compose.yaml -p alt start alt-butterfly-facade alt-backend
```

- [ ] No leftover firing journey pages.
- [ ] Grafana KH SLO groups remain empty (Prometheus is the only evaluator).

## Residual: synthetic activation

External probes are an **ops gate** (ADR-000980). Prometheus does **not** arm no-observation from generic exporter liveness. The contract is `alt_synthetic_probe_result{journey,probe}` (1=success, 0=failure; any sample is a heartbeat) with freshness **15m**, declared in `observability/synthetic/probes.yaml`.

- No fresh heartbeat for a journey: `UserJourneyNoObservation` stays quiet for that journey. Idle nights must not Emergency-page. Honest: coverage is pending until the provider exports the heartbeat.
- Fresh **feeds** or **search** heartbeat + zero BFF `increase[15m]` on that journey: page. Pre-created zeros are not coverage. A failing probe (`result=0`) still arms the journey.
- **Login** whoami is Kratos via `/ory/sessions/whoami` and never increments the BFF login counter. A login heartbeat (or a live synthetic scrape job) must not false-page `UserJourneyNoObservation{journey=login}`. Provider-native consecutive-failure paging for login may still fire.

Do not treat a green in-cluster scrape as user-journey coverage. BFF login samples are JWT 401s and session-shaped paths only.
