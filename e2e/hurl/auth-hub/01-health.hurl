# /health is unauthenticated and its body is defined by the wired
# handler at internal/adapter/handler/health.go — `{"status":"healthy"}`
# only. The legacy handler/health_handler.go adds a `service` field but
# isn't the one main.go mounts.
GET {{auth_hub_url}}/health
HTTP 200
[Asserts]
jsonpath "$.status" == "healthy"
