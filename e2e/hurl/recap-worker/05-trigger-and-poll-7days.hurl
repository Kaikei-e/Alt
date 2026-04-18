# Full happy-path drive of the 7-day window. The trigger handler
# (api/generate.rs:42-85) returns 202 synchronously after spawning the
# scheduler::run_job tokio task. The pipeline then walks
# alt-backend (ListRecapArticles) → recap-subworker (classify-runs +
# clustering) → news-creator (summary/generate) → recap-db
# (recap_outputs row) — every upstream is served by the
# recap-pipeline-stub container under its respective network alias.
#
# Pipeline poll budget: pipeline runs end-to-end against instant stubs,
# so 60 retries × 1s ≈ 1 min should comfortably cover even cold
# scheduler / first-poll latency. Tighten if the suite settles faster.
POST {{base_url}}/v1/generate/recaps/7days
Content-Type: application/json
file,e2e/fixtures/recap-worker/trigger-7days-default.json;
HTTP 202
[Captures]
job_id: jsonpath "$.job_id"
[Asserts]
header "Content-Type" contains "application/json"
jsonpath "$.status" == "accepted"
jsonpath "$.job_id" matches /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/
jsonpath "$.genres" contains "ai"

GET {{base_url}}/v1/recaps/7days
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
