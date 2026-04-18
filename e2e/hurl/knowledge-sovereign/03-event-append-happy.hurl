# AppendKnowledgeEvent happy path — seeds an ArticleCreated event scoped
# to the run's tenant_id/user_id so later scenarios can list it without
# cross-run bleed. The dedupe_key is UNIQUE in knowledge_event_dedupes,
# so it must embed run_id.
#
# Proto3-JSON:
#   - int64 event_seq serializes as a string
#   - bytes payload serializes as base64 ({"title":"hurl-e2e"} base64-encoded)
#   - occurred_at is RFC 3339; the 2026-04 partition covers today's date
POST {{base_url}}/services.sovereign.v1.KnowledgeSovereignService/AppendKnowledgeEvent
Content-Type: application/json
Connect-Protocol-Version: 1
{
  "event": {
    "eventId": "{{event_id}}",
    "occurredAt": "{{occurred_at}}",
    "tenantId": "{{tenant_id}}",
    "userId": "{{user_id}}",
    "actorType": "user",
    "actorId": "e2e-hurl",
    "eventType": "ArticleCreated",
    "aggregateType": "article",
    "aggregateId": "hurl-art-{{run_id}}",
    "dedupeKey": "hurl-e2e-{{run_id}}-ev1",
    "payload": "eyJ0aXRsZSI6Imh1cmwtZTJlIn0="
  }
}
HTTP 200
[Captures]
event_seq: jsonpath "$.eventSeq"
[Asserts]
jsonpath "$.eventSeq" matches "^[0-9]+$"
