# /queue/status contract — pinned to handler/health_handler.py and
# gateway/hybrid_priority_semaphore.py. Used by pre-processor's
# exponential backoff (`accepting=false` is the back-off signal) and
# by Phase 4 queue-saturation scenarios.
#
# Idle-state expectations on Phase 1 startup:
# - rt_queue / be_queue == 0 (no in-flight requests)
# - total_slots == OLLAMA_NUM_PARALLEL (compose default = 1; env
#   may override, so assert >= 1)
# - available_slots == total_slots (nothing held)
# - accepting == true (queue depth 0 < MAX_QUEUE_DEPTH)
# - max_queue_depth == MAX_QUEUE_DEPTH (compose sets 10)
GET {{base_url}}/queue/status
HTTP 200
[Asserts]
header "Content-Type" contains "application/json"
jsonpath "$.rt_queue" == 0
jsonpath "$.be_queue" == 0
jsonpath "$.total_slots" >= 1
jsonpath "$.available_slots" >= 1
jsonpath "$.accepting" == true
jsonpath "$.max_queue_depth" == 10
