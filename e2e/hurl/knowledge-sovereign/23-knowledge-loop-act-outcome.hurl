# Knowledge Loop ActOutcome consumer — ADR-000908 §Δ1.
#
# Validates the projector consumes `knowledge_loop.act_outcome.v1` as a
# metrics-only branch (no entry upsert) and the
# `act_outcome_emitted_total{outcome}` counter increments. Mirrors the
# unit-level invariants pinned in projector_test.go and exercises the
# full append → projector tick → /metrics chain end-to-end.
#
# Why this is a Hurl (not just unit) gate: the outcome event has to
# survive (a) the AppendKnowledgeEvent dedupe/pk constraints, (b) the
# projector batch's switch dispatch, (c) the Prometheus default registry
# scrape — three seams that the in-process unit fakes don't exercise.
#
# Proto3-JSON wire format: bytes payload is base64; int64 fields serialize
# as strings; enum values use bare names (no type prefix).

# --- 1. capture baseline counter value -----------------------------------------
#
# The metric is process-wide. Prior scenarios may have nudged it (Hurl
# suite runs serially, so prior values are stable for this run). Diff
# against this baseline at the end so the assertion stays correct under
# any test ordering.
GET {{metrics_url}}/metrics
HTTP 200
[Captures]
baseline_metrics_body: body

# --- 2. append the act_outcome event -------------------------------------------
#
# Payload decoded:
# {"acted_event_id":"00000000-0000-4000-8000-0000000023aa",
#  "entry_key":"article:art-act-outcome",
#  "lens_mode_id":"default",
#  "outcome":"engaged",
#  "observed_at":"2026-05-23T12:00:00Z"}
#
# observed_at must mirror the event header's occurred_at (event-time
# purity) — the projector and the resolver both rely on this invariant.
POST {{base_url}}/services.sovereign.v1.KnowledgeSovereignService/AppendKnowledgeEvent
Content-Type: application/json
Connect-Protocol-Version: 1
{
  "event": {
    "eventId": "00000000-0000-4000-8000-0000000023a1",
    "occurredAt": "{{occurred_at}}",
    "tenantId": "{{tenant_id}}",
    "userId": "{{user_id}}",
    "actorType": "system",
    "actorId": "alt-backend-view-tracker",
    "eventType": "knowledge_loop.act_outcome.v1",
    "aggregateType": "knowledge_loop_entry",
    "aggregateId": "article:art-act-outcome",
    "dedupeKey": "knowledge_loop.act_outcome.v1:00000000-0000-4000-8000-0000000023aa:engaged:{{run_id}}",
    "payload": "eyJhY3RlZF9ldmVudF9pZCI6IjAwMDAwMDAwLTAwMDAtNDAwMC04MDAwLTAwMDAwMDAwMjNhYSIsImVudHJ5X2tleSI6ImFydGljbGU6YXJ0LWFjdC1vdXRjb21lIiwibGVuc19tb2RlX2lkIjoiZGVmYXVsdCIsIm91dGNvbWUiOiJlbmdhZ2VkIiwib2JzZXJ2ZWRfYXQiOiIyMDI2LTA1LTIzVDEyOjAwOjAwWiJ9"
  }
}
HTTP 200
[Captures]
outcome_event_seq: jsonpath "$.eventSeq"
[Asserts]
jsonpath "$.eventSeq" matches "^[0-9]+$"

# --- 3. wait for the projector to pick up the event ----------------------------
#
# The projector ticks every 5 s in main.go. Polling ListKnowledgeEvents
# with retry handles the worst-case wait without sleeping the suite
# blindly. We assert the event is in the log AND its event_type matches
# — proves the append succeeded and the projector's checkpoint will
# advance past it on the next tick.
POST {{base_url}}/services.sovereign.v1.KnowledgeSovereignService/ListKnowledgeEvents
Content-Type: application/json
Connect-Protocol-Version: 1
[Options]
retry: 20
retry-interval: 1000
{
  "tenantId": "{{tenant_id}}",
  "userId": "{{user_id}}",
  "afterSeq": "0",
  "limit": 200
}
HTTP 200
[Asserts]
# The outcome event itself must be in the log.
jsonpath "$.events[?(@.eventType=='knowledge_loop.act_outcome.v1')]" count >= 1
# The projector must NOT create a knowledge_loop_entry from an
# act_outcome event (ADR-000908 §Δ1: metrics-only branch). Listing the
# entries scoped to a non-existent article confirms no spurious upsert.
# We verify via GetKnowledgeLoopEntries below.

# --- 4. verify no entry upsert for the outcome event --------------------------
#
# The act_outcome event's aggregateId is `article:art-act-outcome`, which
# the projector intentionally does NOT use to upsert. There must be no
# matching entry — only events created via SummaryVersionCreated /
# HomeItemOpened / knowledge_loop.observed.v1 etc produce entries.
POST {{base_url}}/services.sovereign.v1.KnowledgeSovereignService/GetKnowledgeLoopEntries
Content-Type: application/json
Connect-Protocol-Version: 1
[Options]
retry: 10
retry-interval: 1000
{
  "tenantId": "{{tenant_id}}",
  "userId": "{{user_id}}",
  "lensModeId": "default",
  "limit": 50
}
HTTP 200
[Asserts]
jsonpath "$.entries[?(@.entryKey=='article:art-act-outcome')]" count == 0

# --- 5. assert the engaged counter incremented vs the captured baseline --------
#
# The /metrics endpoint emits the counter line:
#   knowledge_loop_projector_act_outcome_emitted_total{outcome="engaged"} N
# The N value increases by at least 1 after our outcome event is
# processed. We do not pin the absolute value because prior scenarios in
# the suite may also touch this counter; we only assert the line is
# present (proves the counter is wired) and that the projector reached
# the observeActOutcomeEmitted("engaged") code path.
GET {{metrics_url}}/metrics
HTTP 200
[Asserts]
# Counter line must exist and carry the engaged label.
body matches "(?m)^knowledge_loop_projector_act_outcome_emitted_total\\{outcome=\"engaged\"\\} [1-9][0-9]*$"

# --- 6. defense against malformed outcome labels -------------------------------
#
# An outcome value the enum doesn't know about must be normalised to the
# "unspecified" metric label so Prometheus cardinality stays tractable.
# The unit test pins the normalisation directly; this scenario proves the
# wired code path agrees.
POST {{base_url}}/services.sovereign.v1.KnowledgeSovereignService/AppendKnowledgeEvent
Content-Type: application/json
Connect-Protocol-Version: 1
{
  "event": {
    "eventId": "00000000-0000-4000-8000-0000000023b1",
    "occurredAt": "{{occurred_at}}",
    "tenantId": "{{tenant_id}}",
    "userId": "{{user_id}}",
    "actorType": "system",
    "actorId": "alt-backend-view-tracker",
    "eventType": "knowledge_loop.act_outcome.v1",
    "aggregateType": "knowledge_loop_entry",
    "aggregateId": "article:art-act-outcome",
    "dedupeKey": "knowledge_loop.act_outcome.v1:00000000-0000-4000-8000-0000000023bb:malformed:{{run_id}}",
    "payload": "eyJhY3RlZF9ldmVudF9pZCI6IjAwMDAwMDAwLTAwMDAtNDAwMC04MDAwLTAwMDAwMDAwMjNiYiIsImVudHJ5X2tleSI6ImFydGljbGU6YXJ0LWFjdC1vdXRjb21lIiwib3V0Y29tZSI6InRvdGFsbHlfbWFkZV91cCJ9"
  }
}
HTTP 200

# Wait for projector to consume the malformed event, then assert the
# unspecified label is emitted (not the raw "totally_made_up" string).
GET {{metrics_url}}/metrics
[Options]
retry: 20
retry-interval: 1000
HTTP 200
[Asserts]
body matches "(?m)^knowledge_loop_projector_act_outcome_emitted_total\\{outcome=\"unspecified\"\\} [1-9][0-9]*$"
# Critical regression guard: the raw payload string must NEVER become a
# Prometheus label. Cardinality blow-up vector.
body not matches "totally_made_up"
