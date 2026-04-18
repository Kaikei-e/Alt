# acolyte-orchestrator readiness probe — must run first. Retries
# /health on :8090 until uvicorn binds the port and the Starlette
# application factory finishes wiring AcolyteConnectService against
# the Atlas-migrated acolyte-db. Python cold-start + DB pool warm-up
# typically resolves in a few seconds.
GET {{base_url}}/health
[Options]
retry: 30
retry-interval: 500
HTTP 200
[Asserts]
jsonpath "$.status" == "ok"
jsonpath "$.service" == "acolyte-orchestrator"
