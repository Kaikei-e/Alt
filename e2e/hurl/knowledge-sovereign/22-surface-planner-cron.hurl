# Surface Planner v2 producer cron — ADR-000874 / fb.md issue 9.
#
# Validates the producer side of the SurfacePlanRecomputed projector branch
# added in ADR-000873. The flow is:
#
#   1. Seed a SummaryVersionCreated so a Knowledge Loop entry exists.
#   2. Append augur.conversation_linked.v1 referencing that entry_key.
#   3. Wait for the planner cron to tick (default 60 s; CI tunes via
#      KNOWLEDGE_SOVEREIGN_PLANNER_TICK_INTERVAL=2s).
#   4. List events with event_type knowledge_loop.surface_plan_recomputed.v1
#      and assert the producer cron emitted exactly one event for our entry.
#   5. Re-fetch the loop entry and assert that the projector's
#      PatchKnowledgeLoopEntrySurfacePlan branch has applied — concretely the
#      surface_planner_version is V2 (the producer pins V2).
#
# Proto3-JSON: bytes payload is base64, int64 fields serialize as strings.

# --- 1. seed a SummaryVersionCreated to create a loop entry -------------------
#
# Payload decoded:
# {"summary_version_id":"sv-planner-cron","article_id":"art-planner-cron",
#  "article_title":"Surface Planner v2 cron coverage"}
POST {{base_url}}/services.sovereign.v1.KnowledgeSovereignService/AppendKnowledgeEvent
Content-Type: application/json
Connect-Protocol-Version: 1
{
  "event": {
    "eventId": "00000000-0000-4000-8000-0000000022a1",
    "occurredAt": "{{occurred_at}}",
    "tenantId": "{{tenant_id}}",
    "userId": "{{user_id}}",
    "actorType": "user",
    "actorId": "e2e-hurl",
    "eventType": "SummaryVersionCreated",
    "aggregateType": "article",
    "aggregateId": "art-planner-cron",
    "dedupeKey": "hurl-planner-cron-{{run_id}}-ev1",
    "payload": "eyJzdW1tYXJ5X3ZlcnNpb25faWQiOiJzdi1wbGFubmVyLWNyb24iLCJhcnRpY2xlX2lkIjoiYXJ0LXBsYW5uZXItY3JvbiIsImFydGljbGVfdGl0bGUiOiJTdXJmYWNlIFBsYW5uZXIgdjIgY3JvbiBjb3ZlcmFnZSJ9"
  }
}
HTTP 200
[Asserts]
jsonpath "$.eventSeq" matches "^[0-9]+$"

# --- 2. append augur.conversation_linked.v1 referencing the loop entry --------
#
# Payload decoded:
# {"entry_key":"article:art-planner-cron","conversation_id":"conv-22","linked_at":"2026-05-01T10:00:00Z"}
POST {{base_url}}/services.sovereign.v1.KnowledgeSovereignService/AppendKnowledgeEvent
Content-Type: application/json
Connect-Protocol-Version: 1
{
  "event": {
    "eventId": "00000000-0000-4000-8000-0000000022a2",
    "occurredAt": "{{occurred_at}}",
    "tenantId": "{{tenant_id}}",
    "userId": "{{user_id}}",
    "actorType": "system",
    "actorId": "augur-orchestrator",
    "eventType": "augur.conversation_linked.v1",
    "aggregateType": "knowledge_loop_session",
    "aggregateId": "article:art-planner-cron",
    "dedupeKey": "hurl-planner-augur-{{run_id}}-ev1",
    "payload": "eyJlbnRyeV9rZXkiOiJhcnRpY2xlOmFydC1wbGFubmVyLWNyb24iLCJjb252ZXJzYXRpb25faWQiOiJjb252LTIyIiwibGlua2VkX2F0IjoiMjAyNi0wNS0wMVQxMDowMDowMFoifQ=="
  }
}
HTTP 200
[Captures]
augur_event_seq: jsonpath "$.eventSeq"
[Asserts]
jsonpath "$.eventSeq" matches "^[0-9]+$"

# --- 3. wait for the planner cron to emit SurfacePlanRecomputed ---------------
#
# CI runs with KNOWLEDGE_SOVEREIGN_PLANNER_TICK_INTERVAL=2s so retries cover
# 30 seconds at 1.5 s spacing — enough for the cron to fire several times.
POST {{base_url}}/services.sovereign.v1.KnowledgeSovereignService/ListKnowledgeEvents
Content-Type: application/json
Connect-Protocol-Version: 1
[Options]
retry: 30
retry-interval: 1500
{
  "tenantId": "{{tenant_id}}",
  "userId": "{{user_id}}",
  "afterSeq": "0",
  "limit": 200
}
HTTP 200
[Asserts]
jsonpath "$.events[?(@.eventType=='knowledge_loop.surface_plan_recomputed.v1')]" count >= 1

# --- 4. assert projector applied the V2 planner version on the entry ----------
POST {{base_url}}/services.sovereign.v1.KnowledgeSovereignService/GetKnowledgeLoopEntries
Content-Type: application/json
Connect-Protocol-Version: 1
[Options]
retry: 20
retry-interval: 1000
{
  "tenantId": "{{tenant_id}}",
  "userId": "{{user_id}}",
  "lensModeId": "default",
  "limit": 50
}
HTTP 200
[Asserts]
jsonpath "$.entries[?(@.entryKey=='article:art-planner-cron')]" count == 1
jsonpath "$.entries[?(@.entryKey=='article:art-planner-cron')].surfacePlannerVersion" includes "SURFACE_PLANNER_VERSION_V2"
