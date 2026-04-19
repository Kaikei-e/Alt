# Public surface smoke — /v1/health is DOS-whitelisted and DB-backed;
# /v1/csrf-token mints a token for state-mutating calls; /metrics exposes
# Prometheus; /security/csp-report accepts browser CSP violation reports.

GET {{base_url}}/v1/health
HTTP 200
[Asserts]
jsonpath "$.status" == "healthy"
jsonpath "$.database" == "connected"
header "Cache-Control" contains "max-age=30"

GET {{base_url}}/v1/csrf-token
HTTP 200
[Asserts]
jsonpath "$.csrf_token" exists
jsonpath "$.csrf_token" matches "^[A-Za-z0-9_\\-=+/]+$"

GET {{base_url}}/metrics
HTTP 200
[Asserts]
header "Content-Type" contains "text/plain"
body contains "# HELP"
