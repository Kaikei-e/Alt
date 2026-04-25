# Knowledge Loop §7 transition allowlist — broader coverage for the canonical
# contract pinned by ADR-000844.
#
# 25-knowledge-loop-transition.hurl covers observe → act (forbidden) and
# observe → orient (allowed). This file fills out the rest of the §7 matrix
# so the projector-level seedDecisionOptions rewrite cannot be undone at the
# handler boundary without a test going red.
#
# Allowed (§7):
#   observe → orient   ← covered by 25
#   observe → decide
#   orient  → decide   ← canonical compare-CTA path
#   decide  → act      ← canonical open/save commit path
#   act     → observe
#
# Forbidden (§7):
#   observe → act      ← covered by 25
#   orient  → act
#   decide  → observe  (without explicit cancel/return event)
#   act     → act
#
# All scenarios use a stable client_transition_id per scenario so re-runs
# either replay deterministically or hit invalid_argument cleanly. The
# alt-backend-deps-stub auto-accepts ReserveKnowledgeLoopTransition and
# AppendKnowledgeEvent on the sovereign side.

# Proto enum values (for clarity; the wire format is numeric):
#   LoopStage: OBSERVE=1, ORIENT=2, DECIDE=3, ACT=4
#   TransitionTrigger: USER_TAP=1

# --- 1. observe → decide (allowed, skips the orient hop) ---------------------

POST {{connect_url}}/alt.knowledge.loop.v1.KnowledgeLoopService/TransitionKnowledgeLoop
X-Alt-Backend-Token: {{jwt}}
Content-Type: application/json
{
  "lensModeId": "default",
  "clientTransitionId": "{{loop_tx_obs_decide}}",
  "entryKey": "article:fixture-canonical-1",
  "fromStage": 1,
  "toStage": 3,
  "trigger": 1,
  "observedProjectionRevision": 1
}
HTTP 200
[Asserts]
jsonpath "$.accepted" == true

# --- 2. orient → decide (canonical compare-CTA path) -------------------------

POST {{connect_url}}/alt.knowledge.loop.v1.KnowledgeLoopService/TransitionKnowledgeLoop
X-Alt-Backend-Token: {{jwt}}
Content-Type: application/json
{
  "lensModeId": "default",
  "clientTransitionId": "{{loop_tx_orient_decide}}",
  "entryKey": "article:fixture-canonical-2",
  "fromStage": 2,
  "toStage": 3,
  "trigger": 1,
  "observedProjectionRevision": 1
}
HTTP 200
[Asserts]
jsonpath "$.accepted" == true

# --- 3. decide → act (canonical open/save commit path) ----------------------

POST {{connect_url}}/alt.knowledge.loop.v1.KnowledgeLoopService/TransitionKnowledgeLoop
X-Alt-Backend-Token: {{jwt}}
Content-Type: application/json
{
  "lensModeId": "default",
  "clientTransitionId": "{{loop_tx_decide_act}}",
  "entryKey": "article:fixture-canonical-3",
  "fromStage": 3,
  "toStage": 4,
  "trigger": 1,
  "observedProjectionRevision": 1
}
HTTP 200
[Asserts]
jsonpath "$.accepted" == true

# --- 4. act → observe (loop-back to a fresh observation) ---------------------

POST {{connect_url}}/alt.knowledge.loop.v1.KnowledgeLoopService/TransitionKnowledgeLoop
X-Alt-Backend-Token: {{jwt}}
Content-Type: application/json
{
  "lensModeId": "default",
  "clientTransitionId": "{{loop_tx_act_observe}}",
  "entryKey": "article:fixture-canonical-4",
  "fromStage": 4,
  "toStage": 1,
  "trigger": 1,
  "observedProjectionRevision": 1
}
HTTP 200
[Asserts]
jsonpath "$.accepted" == true

# --- 5. orient → act (forbidden: only allowed via decide) --------------------

POST {{connect_url}}/alt.knowledge.loop.v1.KnowledgeLoopService/TransitionKnowledgeLoop
X-Alt-Backend-Token: {{jwt}}
Content-Type: application/json
{
  "lensModeId": "default",
  "clientTransitionId": "{{loop_tx_orient_act}}",
  "entryKey": "article:fixture-canonical-5",
  "fromStage": 2,
  "toStage": 4,
  "trigger": 1,
  "observedProjectionRevision": 1
}
HTTP 400
[Asserts]
jsonpath "$.code" == "invalid_argument"

# --- 6. decide → observe (forbidden without explicit cancel/return event) ----

POST {{connect_url}}/alt.knowledge.loop.v1.KnowledgeLoopService/TransitionKnowledgeLoop
X-Alt-Backend-Token: {{jwt}}
Content-Type: application/json
{
  "lensModeId": "default",
  "clientTransitionId": "{{loop_tx_decide_observe}}",
  "entryKey": "article:fixture-canonical-6",
  "fromStage": 3,
  "toStage": 1,
  "trigger": 1,
  "observedProjectionRevision": 1
}
HTTP 400
[Asserts]
jsonpath "$.code" == "invalid_argument"

# --- 7. act → act (forbidden self-loop) --------------------------------------

POST {{connect_url}}/alt.knowledge.loop.v1.KnowledgeLoopService/TransitionKnowledgeLoop
X-Alt-Backend-Token: {{jwt}}
Content-Type: application/json
{
  "lensModeId": "default",
  "clientTransitionId": "{{loop_tx_act_act}}",
  "entryKey": "article:fixture-canonical-7",
  "fromStage": 4,
  "toStage": 4,
  "trigger": 1,
  "observedProjectionRevision": 1
}
HTTP 400
[Asserts]
jsonpath "$.code" == "invalid_argument"
