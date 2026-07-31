# Public surface smoke — /v1/health is DOS-whitelisted and DB-backed;
# /v1/csrf-token mints a token for state-mutating calls; /security/csp-report
# accepts browser CSP violation reports.
#
# /metrics is asserted on {{ops_url}} (:9110), not on the REST port. The
# three-binary split gives alt-backend / alt-harvester / alt-data-hub one
# common operator listener, and observability/prometheus/prometheus.yml
# scrapes all three there. A Prometheus endpoint on the browser-facing
# listener was only ever incidental.

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

GET {{ops_url}}/metrics
HTTP 200
[Asserts]
header "Content-Type" contains "text/plain"
body contains "# HELP"
