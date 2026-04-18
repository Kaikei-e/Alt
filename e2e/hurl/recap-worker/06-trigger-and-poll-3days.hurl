# Symmetric 3-day window — proves both windows are wired through the
# same scheduler with the correct window_days propagation
# (JobContext::new_with_window in scheduler module). The 3-day fetch
# returns its own latest job; scenario 05's 7-day write is filtered out
# by the window_days = 3 predicate in get_latest_completed_job.
POST {{base_url}}/v1/generate/recaps/3days
Content-Type: application/json
file,e2e/fixtures/recap-worker/trigger-3days-default.json;
HTTP 202
[Captures]
job_id: jsonpath "$.job_id"
[Asserts]
header "Content-Type" contains "application/json"
jsonpath "$.status" == "accepted"
jsonpath "$.job_id" matches /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/
jsonpath "$.genres" contains "ai"

GET {{base_url}}/v1/recaps/3days
[Options]
retry: 60
retry-interval: 1000
HTTP 200
[Asserts]
header "Content-Type" contains "application/json"
jsonpath "$.job_id" matches /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/
jsonpath "$.window_start" exists
jsonpath "$.window_end" exists
jsonpath "$.executed_at" exists
jsonpath "$.genres" isCollection
jsonpath "$.genres" count >= 1
