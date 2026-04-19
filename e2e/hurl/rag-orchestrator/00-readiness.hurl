# /readyz on the Echo REST port. rag-orchestrator's readiness pings the DB
# pool (main.go:92-97), so a green /readyz implies rag-db is healthy AND
# rag-db-migrator has completed — a race the per-request [Options] retry
# handles without pulling in an external wait-for-it.sh.
GET {{base_url}}/readyz
[Options]
retry: 60
retry-interval: 1s
HTTP 200
[Asserts]
jsonpath "$.status" == "ready"
