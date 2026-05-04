# Knowledge Loop transition — `trigger=dwell` is rejected at the classifier
# (Phase 0 RED for the Auto-OODA suppression plan).
#
# Rationale:
#   - Pre-fix: `(observe → orient, dwell)` was accepted as
#     `EventKnowledgeLoopObserved` (transition_knowledge_loop_usecase.go
#     classifier line 93). The frontend's IntersectionObserver routinely
#     leaked these into the projection, advancing stage without explicit user
#     intent — flagged in fb.md and the ADR-831 §1.3 "stage axes are
#     independent" invariant audit.
#   - Post-fix: dwell is removed from the FE entirely. The BE classifier
#     additionally refuses `trigger=dwell` for any (from_stage, to_stage)
#     pair as a defensive guard against regression — we never want a future
#     consumer to silently re-introduce passive stage advancement.
#
# Trigger enum: USER_TAP=1, DWELL=2 (proto/alt/knowledge/loop/v1).

# --- Setup ----------------------------------------------------------------

GET {{base_url}}/v1/csrf-token
HTTP 200
[Captures]
csrf: jsonpath "$.csrf_token"

# --- 1. dwell on cross-stage observe→orient is REJECTED -------------------

POST {{connect_url}}/alt.knowledge.loop.v1.KnowledgeLoopService/TransitionKnowledgeLoop
X-Alt-Backend-Token: {{jwt}}
Content-Type: application/json
{
  "lensModeId": "default",
  "clientTransitionId": "{{loop_tx_dwell_cross}}",
  "entryKey": "article:fixture-dwell-cross",
  "fromStage": 1,
  "toStage": 2,
  "trigger": 2,
  "observedProjectionRevision": 1
}
HTTP 400
[Asserts]
jsonpath "$.code" == "invalid_argument"

# --- 2. dwell on same-stage observe→observe is REJECTED -------------------
# (defensive: even the legitimate "I saw it" ack should not be carried by
#  dwell anymore — the FE no longer sends it.)

POST {{connect_url}}/alt.knowledge.loop.v1.KnowledgeLoopService/TransitionKnowledgeLoop
X-Alt-Backend-Token: {{jwt}}
Content-Type: application/json
{
  "lensModeId": "default",
  "clientTransitionId": "{{loop_tx_dwell_same}}",
  "entryKey": "article:fixture-dwell-same",
  "fromStage": 1,
  "toStage": 1,
  "trigger": 2,
  "observedProjectionRevision": 1
}
HTTP 400
[Asserts]
jsonpath "$.code" == "invalid_argument"

# --- 3. dwell on orient→observe (return path) is also REJECTED ------------

POST {{connect_url}}/alt.knowledge.loop.v1.KnowledgeLoopService/TransitionKnowledgeLoop
X-Alt-Backend-Token: {{jwt}}
Content-Type: application/json
{
  "lensModeId": "default",
  "clientTransitionId": "{{loop_tx_dwell_return}}",
  "entryKey": "article:fixture-dwell-return",
  "fromStage": 2,
  "toStage": 1,
  "trigger": 2,
  "observedProjectionRevision": 1
}
HTTP 400
[Asserts]
jsonpath "$.code" == "invalid_argument"

# --- 4. user_tap on observe→orient remains ACCEPTED -----------------------
# (regression guard: rejecting dwell must not break the explicit-tap path.)

POST {{connect_url}}/alt.knowledge.loop.v1.KnowledgeLoopService/TransitionKnowledgeLoop
X-Alt-Backend-Token: {{jwt}}
Content-Type: application/json
{
  "lensModeId": "default",
  "clientTransitionId": "{{loop_tx_user_tap}}",
  "entryKey": "article:fixture-user-tap",
  "fromStage": 1,
  "toStage": 2,
  "trigger": 1,
  "observedProjectionRevision": 1
}
HTTP 200
[Asserts]
jsonpath "$.accepted" == true
