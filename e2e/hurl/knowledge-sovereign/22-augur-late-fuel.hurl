# Augur link as late fuel — ADR-000939 (replaces the retired surface_planner_cron).
#
# Before ADR-000939 a cron re-emitted knowledge_loop.surface_plan_recomputed.v1
# for augur links and a projector branch applied it. That detour is retired: the
# projector now consumes augur.conversation_linked.v1 DIRECTLY as late fuel and
# re-derives the target entry's relation-set (Inquiry) + lens from the evidence
# accumulator. The flow:
#
#   1. Seed a SummaryVersionCreated so a fresh Knowledge Loop entry exists (NOW).
#   2. Append augur.conversation_linked.v1 referencing that entry_key.
#   3. Re-fetch the entry and assert the late fuel applied: surface_planner_version
#      is V2 (HasAugurLink is real evidence) and the entry moved to CONTINUE (an
#      unfinished augur thread). A lone baseline summary must NOT produce a
#      Contradiction, so the bucket is CONTINUE, not CHANGED.
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

# --- 3. assert the projector applied the augur link as late fuel --------------
#
# The projector ticks every 5 s; both events land in one batch (entry created,
# then patched by the augur late fuel). Retry covers the worst-case wait. The
# entry must carry the V2 planner version (HasAugurLink is real evidence) and
# sit in CONTINUE — an unfinished augur thread, NOT CHANGED (a lone baseline
# summary is not version drift, so no false Contradiction).
POST {{base_url}}/services.sovereign.v1.KnowledgeSovereignService/GetKnowledgeLoopEntries
Content-Type: application/json
Connect-Protocol-Version: 1
[Options]
retry: 25
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
jsonpath "$.entries[?(@.entryKey=='article:art-planner-cron')].surfaceBucket" includes "SURFACE_BUCKET_CONTINUE"
