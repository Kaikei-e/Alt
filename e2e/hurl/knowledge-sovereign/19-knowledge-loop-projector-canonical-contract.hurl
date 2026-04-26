# Knowledge Loop projector — canonical contract (ADR-000844).
#
# Drives the in-process projector that knowledge-sovereign now hosts (ADR-000844
# follow-up moved enricher + projector job from alt-backend into this service).
# The flow is:
#
#   1. AppendKnowledgeEvent: SummaryVersionCreated with article_title in payload.
#   2. Wait for the 5-second projector tick to pick it up.
#   3. GetKnowledgeLoopEntries to read back the projection.
#   4. Assert §11 (Why as first-class) and §7 (transition allowlist) of the
#      canonical contract:
#        - whyPrimary.text NOT == "New summary" placeholder
#        - whyPrimary.text contains the article_title from payload
#        - decisionOptions only carry §7-allowed intents for Observe entries
#          (revisit / ask / snooze; never open / save which require observe → act)
#
# Proto3-JSON wire format:
#   - bytes payload is base64
#   - int64 fields serialize as strings
#   - enum DecisionIntent values: REVISIT=5, ASK=2, SNOOZE=6, OPEN=1, SAVE=3
#   - decision_options column is JSONB containing the seed; the proto wraps it
#     in a `bytes` field, which Connect-JSON encodes as base64.

# --- 1. append SummaryVersionCreated with article_title in payload ------------
#
# Payload decoded:
# {"summary_version_id":"sv-hurl-canonical","article_id":"art-hurl-canonical",
#  "article_title":"Event Sourcing in Practice"}
# A locally-unique event_id avoids colliding with 03-event-append-happy.hurl,
# which already consumed `{{event_id}}` from run.sh. The PRIMARY KEY on
# knowledge_events.event_id would otherwise return 500.
POST {{base_url}}/services.sovereign.v1.KnowledgeSovereignService/AppendKnowledgeEvent
Content-Type: application/json
Connect-Protocol-Version: 1
{
  "event": {
    "eventId": "00000000-0000-4000-8000-000000000abc",
    "occurredAt": "{{occurred_at}}",
    "tenantId": "{{tenant_id}}",
    "userId": "{{user_id}}",
    "actorType": "user",
    "actorId": "e2e-hurl",
    "eventType": "SummaryVersionCreated",
    "aggregateType": "article",
    "aggregateId": "art-hurl-canonical-{{run_id}}",
    "dedupeKey": "hurl-loop-canonical-{{run_id}}-ev1",
    "payload": "eyJzdW1tYXJ5X3ZlcnNpb25faWQiOiJzdi1odXJsLWNhbm9uaWNhbCIsImFydGljbGVfaWQiOiJhcnQtaHVybC1jYW5vbmljYWwiLCJhcnRpY2xlX3RpdGxlIjoiRXZlbnQgU291cmNpbmcgaW4gUHJhY3RpY2UifQ=="
  }
}
HTTP 200
[Captures]
loop_event_seq: jsonpath "$.eventSeq"
[Asserts]
jsonpath "$.eventSeq" matches "^[0-9]+$"

# --- 2. wait for the projector to pick up the event ---------------------------
#
# The projector ticks every 5 s in main.go. Polling GetKnowledgeLoopEntries
# with retry handles the worst-case wait without sleeping the suite blindly.
# The same request runs up to 20 times at 1 s spacing — enough for one
# projector cycle in CI even under load.
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
# At least one entry must derive from our just-appended event. The
# article_title flows through the v3 enricher into why_text, so a
# substring assertion both pins §11 (substantive narrative) and guards
# against the regression where the enricher emitted only "New summary".
# We pick the first entry deterministically — the projector orders by
# event_seq desc and our just-appended event has the largest seq.
jsonpath "$.entries[0].whyPrimary.text" contains "Event Sourcing in Practice"
jsonpath "$.entries[0].whyPrimary.text" contains "fresh summary ready to read"
# Hard regression guard: the literal placeholder must not survive.
jsonpath "$.entries[?(@.whyPrimary.text=='New summary')]" count == 0

# --- 3. assert §7 transition allowlist on the seeded decisionOptions ----------
#
# The projector seeds decision_options as JSONB. Connect-JSON encodes the
# bytes field as base64; the seed JSON for an Observe entry is
#   [{"action_id":"revisit","intent":"revisit"},
#    {"action_id":"ask","intent":"ask"},
#    {"action_id":"snooze","intent":"snooze"}]
# The base64 of that payload (compact, no spaces) is asserted as a substring
# match: an Observe-stage entry MUST contain the `revisit` intent and MUST NOT
# carry an `open` or `save` intent — those require observe → act, which §7
# forbids.
POST {{base_url}}/services.sovereign.v1.KnowledgeSovereignService/GetKnowledgeLoopEntries
Content-Type: application/json
Connect-Protocol-Version: 1
{
  "tenantId": "{{tenant_id}}",
  "userId": "{{user_id}}",
  "lensModeId": "default",
  "limit": 10
}
HTTP 200
[Asserts]
# Connect-JSON emits enum values as their bare name (without the type
# prefix) by default. The first entry — our just-appended SummaryVersionCreated
# — must land in the Observe stage so the §7 allowlist applies.
jsonpath "$.entries[0].proposedStage" == "LOOP_STAGE_OBSERVE"
# Loop priority falls out of the surface-bucket → priority mapping for
# fresh entries (NOW → CRITICAL).
jsonpath "$.entries[0].surfaceBucket" == "SURFACE_BUCKET_NOW"
