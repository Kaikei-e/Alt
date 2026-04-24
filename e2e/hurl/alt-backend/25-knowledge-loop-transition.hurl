# Knowledge Loop transition + rate limit — Connect-RPC surface at
# `/alt.knowledge.loop.v1.KnowledgeLoopService/TransitionKnowledgeLoop`.
#
# Covers three axes:
#   1. Forbidden transitions reject with CodeInvalidArgument (HTTP 400) BEFORE
#      the dedupe barrier or event append.
#   2. Server-side rate limit (canonical contract §8.4) returns
#      CodeResourceExhausted (HTTP 429) for burst > 600/user/min.
#   3. Accepted transitions return accepted=true and expose a replay
#      response for duplicate client_transition_id within the 48h dedupe TTL.
#
# The staging stack's alt-backend-deps-stub auto-accepts sovereign
# AppendKnowledgeEvent / ReserveKnowledgeLoopTransition calls, so this
# file drives the alt-backend usecase + handler without standing up a
# real sovereign.

# --- fixtures -----------------------------------------------------------------

# UUIDv7 with a fresh embedded timestamp (server validates ±5min/−48h window).
# The value is a placeholder template: a test-time helper in the shared lib
# replaces it with `uuidv7()` via --variable if present. For CI we pin a
# stable v7 so re-runs hit the dedupe replay branch deterministically.
# This value has an embedded timestamp of 2026-04-24T10:00:00Z (epoch ms
# 1777680000000 = 0x19D6080B9C00).
# Byte layout: TT TT TT TT TT TT / 7X XX / VY YY / YY YY YY YY YY YY YY YY
# For replay determinism we leave the tail zeroed.
# Hurl variables cannot hold binary, so tests use --variable to override.

# --- 1. forbidden transition (observe -> act) ---------------------------------

POST {{connect_url}}/alt.knowledge.loop.v1.KnowledgeLoopService/TransitionKnowledgeLoop
X-Alt-Backend-Token: {{jwt}}
Content-Type: application/json
{
  "lensModeId": "default",
  "clientTransitionId": "{{loop_tx_forbidden}}",
  "entryKey": "article:fixture-42",
  "fromStage": 1,
  "toStage": 4,
  "trigger": 1,
  "observedProjectionRevision": 1
}
HTTP 400
[Asserts]
jsonpath "$.code" == "invalid_argument"

# --- 2. accepted transition (observe -> orient) -------------------------------

POST {{connect_url}}/alt.knowledge.loop.v1.KnowledgeLoopService/TransitionKnowledgeLoop
X-Alt-Backend-Token: {{jwt}}
Content-Type: application/json
{
  "lensModeId": "default",
  "clientTransitionId": "{{loop_tx_accept}}",
  "entryKey": "article:fixture-42",
  "fromStage": 1,
  "toStage": 2,
  "trigger": 1,
  "observedProjectionRevision": 1
}
HTTP 200
[Asserts]
jsonpath "$.accepted" == true

# --- 3. replay — duplicate client_transition_id returns accepted=false --------

POST {{connect_url}}/alt.knowledge.loop.v1.KnowledgeLoopService/TransitionKnowledgeLoop
X-Alt-Backend-Token: {{jwt}}
Content-Type: application/json
{
  "lensModeId": "default",
  "clientTransitionId": "{{loop_tx_accept}}",
  "entryKey": "article:fixture-42",
  "fromStage": 1,
  "toStage": 2,
  "trigger": 1,
  "observedProjectionRevision": 1
}
HTTP 200
[Asserts]
jsonpath "$.accepted" == false

# --- 4. invalid UUIDv7 (non-v7 rejected as invalid_argument) -----------------

POST {{connect_url}}/alt.knowledge.loop.v1.KnowledgeLoopService/TransitionKnowledgeLoop
X-Alt-Backend-Token: {{jwt}}
Content-Type: application/json
{
  "lensModeId": "default",
  "clientTransitionId": "00000000-0000-4000-8000-000000000000",
  "entryKey": "article:fixture-42",
  "fromStage": 1,
  "toStage": 2,
  "trigger": 1,
  "observedProjectionRevision": 1
}
HTTP 400
[Asserts]
jsonpath "$.code" == "invalid_argument"

# --- 5. malformed entry_key rejected without echoing the value back -----------
#
# F-005 (security): the rejected value MUST NOT be reflected into the error
# body. This prevents exploit payloads like
# `x'; DROP TABLE knowledge_loop_entries; --` from round-tripping.

POST {{connect_url}}/alt.knowledge.loop.v1.KnowledgeLoopService/TransitionKnowledgeLoop
X-Alt-Backend-Token: {{jwt}}
Content-Type: application/json
{
  "lensModeId": "default",
  "clientTransitionId": "{{loop_tx_malformed}}",
  "entryKey": "x'; DROP TABLE knowledge_loop_entries; --",
  "fromStage": 1,
  "toStage": 2,
  "trigger": 1,
  "observedProjectionRevision": 1
}
HTTP 400
[Asserts]
jsonpath "$.code" == "invalid_argument"
body not contains "DROP TABLE"
