# Knowledge Loop Review-lane transitions.
#
# Backend E2E is Hurl for this stack. This scenario pins the Review actions
# introduced for fb.md §F: recheck / archive / mark_reviewed are same-stage
# transitions routed through TransitionTrigger values 6/7/8.

# Proto enum names are used here instead of numeric values so this file also
# catches enum-name contract drift at the Connect JSON boundary.

# --- 1. recheck same-stage is accepted --------------------------------------

POST {{connect_url}}/alt.knowledge.loop.v1.KnowledgeLoopService/TransitionKnowledgeLoop
X-Alt-Backend-Token: {{jwt}}
Content-Type: application/json
{
  "lensModeId": "default",
  "clientTransitionId": "{{loop_tx_review_recheck}}",
  "entryKey": "article:fixture-review-recheck",
  "fromStage": "LOOP_STAGE_OBSERVE",
  "toStage": "LOOP_STAGE_OBSERVE",
  "trigger": "TRANSITION_TRIGGER_RECHECK",
  "observedProjectionRevision": 1
}
HTTP 200
[Asserts]
jsonpath "$.accepted" == true

# --- 2. archive same-stage is accepted --------------------------------------

POST {{connect_url}}/alt.knowledge.loop.v1.KnowledgeLoopService/TransitionKnowledgeLoop
X-Alt-Backend-Token: {{jwt}}
Content-Type: application/json
{
  "lensModeId": "default",
  "clientTransitionId": "{{loop_tx_review_archive}}",
  "entryKey": "article:fixture-review-archive",
  "fromStage": "LOOP_STAGE_OBSERVE",
  "toStage": "LOOP_STAGE_OBSERVE",
  "trigger": "TRANSITION_TRIGGER_ARCHIVE",
  "observedProjectionRevision": 1
}
HTTP 200
[Asserts]
jsonpath "$.accepted" == true

# --- 3. mark_reviewed same-stage is accepted --------------------------------

POST {{connect_url}}/alt.knowledge.loop.v1.KnowledgeLoopService/TransitionKnowledgeLoop
X-Alt-Backend-Token: {{jwt}}
Content-Type: application/json
{
  "lensModeId": "default",
  "clientTransitionId": "{{loop_tx_review_mark_reviewed}}",
  "entryKey": "article:fixture-review-mark-reviewed",
  "fromStage": "LOOP_STAGE_OBSERVE",
  "toStage": "LOOP_STAGE_OBSERVE",
  "trigger": "TRANSITION_TRIGGER_MARK_REVIEWED",
  "observedProjectionRevision": 1
}
HTTP 200
[Asserts]
jsonpath "$.accepted" == true

# --- 4. review trigger with a cross-stage tuple is rejected ------------------

POST {{connect_url}}/alt.knowledge.loop.v1.KnowledgeLoopService/TransitionKnowledgeLoop
X-Alt-Backend-Token: {{jwt}}
Content-Type: application/json
{
  "lensModeId": "default",
  "clientTransitionId": "{{loop_tx_review_cross_stage}}",
  "entryKey": "article:fixture-review-cross-stage",
  "fromStage": "LOOP_STAGE_OBSERVE",
  "toStage": "LOOP_STAGE_ORIENT",
  "trigger": "TRANSITION_TRIGGER_RECHECK",
  "observedProjectionRevision": 1
}
HTTP 400
[Asserts]
jsonpath "$.code" == "invalid_argument"
