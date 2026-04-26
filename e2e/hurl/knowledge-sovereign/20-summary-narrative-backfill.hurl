# Knowledge Loop — SummaryNarrativeBackfilled discovered-event regression (ADR-000846).
#
# Drives the patch-only-why projector branch. The flow:
#
#   1. Append a SummaryVersionCreated event WITHOUT article_title in payload
#      (mimicking historic events emitted before the producer fix).
#   2. Wait for the projector tick — entry's why_text falls back to the generic
#      "A new summary is ready in one of your feeds." narrative.
#   3. Append a SummaryNarrativeBackfilled event with article_title in payload
#      (what the new alt-backend backfill job emits).
#   4. Wait for the projector tick — entry's why_text MUST be patched to the
#      title-bearing narrative WITHOUT touching dismiss_state or other fields.
#
# Reproject-safety contract:
#   - The patch event lives in the append-only knowledge_events log.
#   - replay convergence: original (lower seq) creates entry with fallback,
#     patch (higher seq) overwrites why_text only.
#   - dismiss_state preservation is asserted by the integration / unit tests at
#     the driver layer; this Hurl pins the wire-level shape and projector
#     responsiveness.

# --- 1. append historic SummaryVersionCreated (no article_title) -------------
#
# Payload decoded:
#   {"summary_version_id":"00000000-0000-4000-8000-00000000bf01",
#    "article_id":"00000000-0000-4000-8000-00000000bf02"}
#
# Notably article_title is absent — this is the regression we are repairing.
POST {{base_url}}/services.sovereign.v1.KnowledgeSovereignService/AppendKnowledgeEvent
Content-Type: application/json
Connect-Protocol-Version: 1
{
  "event": {
    "eventId": "00000000-0000-4000-8000-00000000bf10",
    "occurredAt": "{{occurred_at}}",
    "tenantId": "{{tenant_id}}",
    "userId": "{{user_id}}",
    "actorType": "user",
    "actorId": "e2e-hurl-historic",
    "eventType": "SummaryVersionCreated",
    "aggregateType": "article",
    "aggregateId": "art-bf-canonical-{{run_id}}",
    "dedupeKey": "hurl-bf-canonical-{{run_id}}-historic",
    "payload": "eyJzdW1tYXJ5X3ZlcnNpb25faWQiOiIwMDAwMDAwMC0wMDAwLTQwMDAtODAwMC0wMDAwMDAwMGJmMDEiLCJhcnRpY2xlX2lkIjoiMDAwMDAwMDAtMDAwMC00MDAwLTgwMDAtMDAwMDAwMDBiZjAyIn0="
  }
}
HTTP 200
[Asserts]
jsonpath "$.eventSeq" matches "^[0-9]+$"

# --- 2. wait for projector tick + assert fallback narrative ------------------
#
# The projector ticks every 5 s. Polling with retry handles the worst-case
# wait without sleeping the suite blindly. Body assertion catches the
# fallback substring anywhere in the JSON response (the entries array
# contains exactly one entry from this aggregateId).
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
  "limit": 10
}
HTTP 200
[Asserts]
jsonpath "$.entries" count > 0
body contains "A new summary is ready in one of your feeds."

# --- 3. append SummaryNarrativeBackfilled (with article_title) --------------
#
# Payload decoded:
#   {"summary_version_id":"00000000-0000-4000-8000-00000000bf01",
#    "article_id":"00000000-0000-4000-8000-00000000bf02",
#    "article_title":"Discovered Title - Backfilled"}
POST {{base_url}}/services.sovereign.v1.KnowledgeSovereignService/AppendKnowledgeEvent
Content-Type: application/json
Connect-Protocol-Version: 1
{
  "event": {
    "eventId": "00000000-0000-4000-8000-00000000bf20",
    "occurredAt": "{{occurred_at}}",
    "tenantId": "{{tenant_id}}",
    "userId": "{{user_id}}",
    "actorType": "service",
    "actorId": "summary-narrative-backfill",
    "eventType": "SummaryNarrativeBackfilled",
    "aggregateType": "article",
    "aggregateId": "art-bf-canonical-{{run_id}}",
    "dedupeKey": "hurl-bf-canonical-{{run_id}}-patch",
    "payload": "eyJzdW1tYXJ5X3ZlcnNpb25faWQiOiIwMDAwMDAwMC0wMDAwLTQwMDAtODAwMC0wMDAwMDAwMGJmMDEiLCJhcnRpY2xlX2lkIjoiMDAwMDAwMDAtMDAwMC00MDAwLTgwMDAtMDAwMDAwMDBiZjAyIiwiYXJ0aWNsZV90aXRsZSI6IkRpc2NvdmVyZWQgVGl0bGUgLSBCYWNrZmlsbGVkIn0="
  }
}
HTTP 200
[Asserts]
jsonpath "$.eventSeq" matches "^[0-9]+$"

# --- 4. wait for projector tick + assert patched narrative -------------------
#
# After the patch event is projected, the entry's why_text must include the
# discovered article_title. If the projector has not registered the
# SummaryNarrativeBackfilled case yet, the patch falls through to the default
# no-op branch and this assertion fails — exactly the RED state we want before
# adding the projector branch.
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
  "limit": 10
}
HTTP 200
[Asserts]
jsonpath "$.entries" count > 0
# The patched entry must inline the discovered article_title in why_text.
body contains "Discovered Title - Backfilled"
body contains "fresh summary ready to read"
