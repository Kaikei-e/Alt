# Pin both health probes:
#   /health/live  — dependency-free, always 200 (api/health.rs:55-61)
#   /health/ready — pings subworker.ping() + news_creator.health_check()
#                   (api/health.rs:31-53). The recap-pipeline-stub answers
#                   GET /health on both aliases (subworker_client::ping
#                   hits /health, NewsCreatorClient::health_check joins
#                   "health" against base_url), so a healthy stack returns
#                   200 with {"status":"ready"}.
GET {{base_url}}/health/live
HTTP 200
[Asserts]
header "Content-Type" contains "application/json"
jsonpath "$.status" == "live"

GET {{base_url}}/health/ready
HTTP 200
[Asserts]
header "Content-Type" contains "application/json"
jsonpath "$.status" == "ready"
