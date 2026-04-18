# knowledge-sovereign readiness probe — must run first. Retries /health on
# the main RPC port until Compose healthcheck settles. The metrics port
# (:9501) is already probed by the container healthcheck, but the RPC
# server starts independently and may still be warming up.
GET {{base_url}}/health
[Options]
retry: 30
retry-interval: 500
HTTP 200
[Asserts]
jsonpath "$.status" == "ok"
jsonpath "$.service" == "knowledge-sovereign"
