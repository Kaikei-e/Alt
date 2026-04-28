# Knowledge Loop projector: Reviewed trigger semantics, surface projection,
# per-entry current stage, and article act target.
#
# Backend E2E for services is Hurl. This scenario drives sovereign directly:
# append immutable knowledge_events, wait for the in-process projector, then
# read the Loop projection back through Connect-RPC.

# Payloads decoded:
# seed recheck:
# {"summary_version_id":"sv-review-recheck","article_id":"art-review-recheck",
#  "article_title":"Review Recheck Candidate","entry_key":"article:review-recheck"}
# seed archive:
# {"summary_version_id":"sv-review-archive","article_id":"art-review-archive",
#  "article_title":"Review Archive Candidate","entry_key":"article:review-archive"}
# orient:
# {"entry_key":"article:review-recheck","lens_mode_id":"default",
#  "from_stage":"LOOP_STAGE_OBSERVE","to_stage":"LOOP_STAGE_ORIENT",
#  "trigger":"TRANSITION_TRIGGER_USER_TAP","observed_projection_revision":1,
#  "client_transition_id":"hurl-current-orient"}
# recheck:
# {"entry_key":"article:review-recheck","lens_mode_id":"default",
#  "from_stage":"LOOP_STAGE_ORIENT","to_stage":"LOOP_STAGE_ORIENT",
#  "trigger":"TRANSITION_TRIGGER_RECHECK","observed_projection_revision":1,
#  "client_transition_id":"hurl-review-recheck"}
# archive:
# {"entry_key":"article:review-archive","lens_mode_id":"default",
#  "from_stage":"LOOP_STAGE_OBSERVE","to_stage":"LOOP_STAGE_OBSERVE",
#  "trigger":"TRANSITION_TRIGGER_ARCHIVE","observed_projection_revision":1,
#  "client_transition_id":"hurl-review-archive"}

# --- 1. seed two active article entries -------------------------------------

POST {{base_url}}/services.sovereign.v1.KnowledgeSovereignService/AppendKnowledgeEvent
Content-Type: application/json
Connect-Protocol-Version: 1
{
  "event": {
    "eventId": "00000000-0000-4000-8000-000000000d01",
    "occurredAt": "{{occurred_at}}",
    "tenantId": "{{tenant_id}}",
    "userId": "{{user_id}}",
    "actorType": "user",
    "actorId": "e2e-hurl",
    "eventType": "SummaryVersionCreated",
    "aggregateType": "article",
    "aggregateId": "art-review-recheck-{{run_id}}",
    "dedupeKey": "hurl-loop-review-{{run_id}}-seed-recheck",
    "payload": "eyJzdW1tYXJ5X3ZlcnNpb25faWQiOiJzdi1yZXZpZXctcmVjaGVjayIsImFydGljbGVfaWQiOiJhcnQtcmV2aWV3LXJlY2hlY2siLCJhcnRpY2xlX3RpdGxlIjoiUmV2aWV3IFJlY2hlY2sgQ2FuZGlkYXRlIiwiZW50cnlfa2V5IjoiYXJ0aWNsZTpyZXZpZXctcmVjaGVjayJ9"
  }
}
HTTP 200

POST {{base_url}}/services.sovereign.v1.KnowledgeSovereignService/AppendKnowledgeEvent
Content-Type: application/json
Connect-Protocol-Version: 1
{
  "event": {
    "eventId": "00000000-0000-4000-8000-000000000d02",
    "occurredAt": "{{occurred_at}}",
    "tenantId": "{{tenant_id}}",
    "userId": "{{user_id}}",
    "actorType": "user",
    "actorId": "e2e-hurl",
    "eventType": "SummaryVersionCreated",
    "aggregateType": "article",
    "aggregateId": "art-review-archive-{{run_id}}",
    "dedupeKey": "hurl-loop-review-{{run_id}}-seed-archive",
    "payload": "eyJzdW1tYXJ5X3ZlcnNpb25faWQiOiJzdi1yZXZpZXctYXJjaGl2ZSIsImFydGljbGVfaWQiOiJhcnQtcmV2aWV3LWFyY2hpdmUiLCJhcnRpY2xlX3RpdGxlIjoiUmV2aWV3IEFyY2hpdmUgQ2FuZGlkYXRlIiwiZW50cnlfa2V5IjoiYXJ0aWNsZTpyZXZpZXctYXJjaGl2ZSJ9"
  }
}
HTTP 200

# --- 2. append stage progress + Review trigger events -----------------------

POST {{base_url}}/services.sovereign.v1.KnowledgeSovereignService/AppendKnowledgeEvent
Content-Type: application/json
Connect-Protocol-Version: 1
{
  "event": {
    "eventId": "00000000-0000-4000-8000-000000000d03",
    "occurredAt": "{{occurred_at}}",
    "tenantId": "{{tenant_id}}",
    "userId": "{{user_id}}",
    "actorType": "user",
    "actorId": "e2e-hurl",
    "eventType": "knowledge_loop.oriented.v1",
    "aggregateType": "loop_session",
    "aggregateId": "article:review-recheck",
    "dedupeKey": "hurl-loop-review-{{run_id}}-orient",
    "payload": "eyJlbnRyeV9rZXkiOiJhcnRpY2xlOnJldmlldy1yZWNoZWNrIiwibGVuc19tb2RlX2lkIjoiZGVmYXVsdCIsImZyb21fc3RhZ2UiOiJMT09QX1NUQUdFX09CU0VSVkUiLCJ0b19zdGFnZSI6IkxPT1BfU1RBR0VfT1JJRU5UIiwidHJpZ2dlciI6IlRSQU5TSVRJT05fVFJJR0dFUl9VU0VSX1RBUCIsIm9ic2VydmVkX3Byb2plY3Rpb25fcmV2aXNpb24iOjEsImNsaWVudF90cmFuc2l0aW9uX2lkIjoiaHVybC1jdXJyZW50LW9yaWVudCJ9"
  }
}
HTTP 200

POST {{base_url}}/services.sovereign.v1.KnowledgeSovereignService/AppendKnowledgeEvent
Content-Type: application/json
Connect-Protocol-Version: 1
{
  "event": {
    "eventId": "00000000-0000-4000-8000-000000000d04",
    "occurredAt": "{{occurred_at}}",
    "tenantId": "{{tenant_id}}",
    "userId": "{{user_id}}",
    "actorType": "user",
    "actorId": "e2e-hurl",
    "eventType": "knowledge_loop.reviewed.v1",
    "aggregateType": "loop_session",
    "aggregateId": "article:review-recheck",
    "dedupeKey": "hurl-loop-review-{{run_id}}-recheck",
    "payload": "eyJlbnRyeV9rZXkiOiJhcnRpY2xlOnJldmlldy1yZWNoZWNrIiwibGVuc19tb2RlX2lkIjoiZGVmYXVsdCIsImZyb21fc3RhZ2UiOiJMT09QX1NUQUdFX09SSUVOVCIsInRvX3N0YWdlIjoiTE9PUF9TVEFHRV9PUklFTlQiLCJ0cmlnZ2VyIjoiVFJBTlNJVElPTl9UUklHR0VSX1JFQ0hFQ0siLCJvYnNlcnZlZF9wcm9qZWN0aW9uX3JldmlzaW9uIjoxLCJjbGllbnRfdHJhbnNpdGlvbl9pZCI6Imh1cmwtcmV2aWV3LXJlY2hlY2sifQ=="
  }
}
HTTP 200

POST {{base_url}}/services.sovereign.v1.KnowledgeSovereignService/AppendKnowledgeEvent
Content-Type: application/json
Connect-Protocol-Version: 1
{
  "event": {
    "eventId": "00000000-0000-4000-8000-000000000d05",
    "occurredAt": "{{occurred_at}}",
    "tenantId": "{{tenant_id}}",
    "userId": "{{user_id}}",
    "actorType": "user",
    "actorId": "e2e-hurl",
    "eventType": "knowledge_loop.reviewed.v1",
    "aggregateType": "loop_session",
    "aggregateId": "article:review-archive",
    "dedupeKey": "hurl-loop-review-{{run_id}}-archive",
    "payload": "eyJlbnRyeV9rZXkiOiJhcnRpY2xlOnJldmlldy1hcmNoaXZlIiwibGVuc19tb2RlX2lkIjoiZGVmYXVsdCIsImZyb21fc3RhZ2UiOiJMT09QX1NUQUdFX09CU0VSVkUiLCJ0b19zdGFnZSI6IkxPT1BfU1RBR0VfT0JTRVJWRSIsInRyaWdnZXIiOiJUUkFOU0lUSU9OX1RSSUdHRVJfQVJDSElWRSIsIm9ic2VydmVkX3Byb2plY3Rpb25fcmV2aXNpb24iOjEsImNsaWVudF90cmFuc2l0aW9uX2lkIjoiaHVybC1yZXZpZXctYXJjaGl2ZSJ9"
  }
}
HTTP 200

# --- 3. read entries after projector catches up -----------------------------

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
  "includeDismissed": true,
  "limit": 20
}
HTTP 200
[Asserts]
jsonpath "$.entries[?(@.entryKey=='article:review-recheck')]" count == 1
jsonpath "$.entries[?(@.entryKey=='article:review-recheck')].dismissState" contains "DISMISS_STATE_ACTIVE"
jsonpath "$.entries[?(@.entryKey=='article:review-recheck')].proposedStage" contains "LOOP_STAGE_OBSERVE"
jsonpath "$.entries[?(@.entryKey=='article:review-recheck')].currentEntryStage" contains "LOOP_STAGE_ORIENT"
jsonpath "$.entries[?(@.entryKey=='article:review-recheck')].actTargets" contains "W3sicm91dGUiOiAiL2FydGljbGVzL2FydC1yZXZpZXctcmVjaGVjayIsICJ0YXJnZXRfcmVmIjogImFydC1yZXZpZXctcmVjaGVjayIsICJ0YXJnZXRfdHlwZSI6ICJhcnRpY2xlIn1d"
jsonpath "$.entries[?(@.entryKey=='article:review-archive')]" count == 1
jsonpath "$.entries[?(@.entryKey=='article:review-archive')].dismissState" contains "DISMISS_STATE_COMPLETED"

# --- 4. surface recompute uses active entries only --------------------------

POST {{base_url}}/services.sovereign.v1.KnowledgeSovereignService/GetKnowledgeLoopSurfaces
Content-Type: application/json
Connect-Protocol-Version: 1
[Options]
retry: 20
retry-interval: 1000
{
  "tenantId": "{{tenant_id}}",
  "userId": "{{user_id}}",
  "lensModeId": "default"
}
HTTP 200
[Asserts]
jsonpath "$.surfaces[?(@.surfaceBucket=='SURFACE_BUCKET_NOW')]" count == 1
jsonpath "$.surfaces[?(@.surfaceBucket=='SURFACE_BUCKET_NOW')].primaryEntryKey" contains "article:review-recheck"
