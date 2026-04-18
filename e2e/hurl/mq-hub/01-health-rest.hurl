# Plain HTTP /health — full JSON shape and content-type assertions.
# Source: mq-hub/app/main.go:68 registers this handler outside the Connect-RPC
# mux so non-RPC clients (compose healthcheck, ops tools) can probe liveness.
GET {{base_url}}/health
HTTP 200
[Asserts]
jsonpath "$.healthy" == true
jsonpath "$.redis_status" == "connected"
jsonpath "$.uptime_seconds" isNumber
jsonpath "$.uptime_seconds" >= 0
