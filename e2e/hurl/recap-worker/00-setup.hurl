# recap-worker liveness gate. Uses /health/live (dependency-free, see
# recap-worker/recap-worker/src/api/health.rs:55-61) rather than
# /health/ready so the suite still gates correctly while we are about
# to validate the readiness probe in 01.
GET {{base_url}}/health/live
[Options]
retry: 30
retry-interval: 500
HTTP 200
[Asserts]
header "Content-Type" contains "application/json"
jsonpath "$.status" == "live"
