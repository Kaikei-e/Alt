# /healthz is the always-200 liveness endpoint on the Echo REST port
# (main.go:89-91). It must not depend on DB state.
GET {{base_url}}/healthz
HTTP 200
[Asserts]
header "Content-Type" contains "application/json"
jsonpath "$.status" == "ok"
