# alt-backend readiness probe — must run first. Retries the REST /v1/health
# endpoint until the compose healthcheck settles. The DB healthcheck is
# declared on alt-backend-db, but alt-backend's own pool may still be
# warming up when compose reports the container healthy.
GET {{base_url}}/v1/health
[Options]
retry: 60
retry-interval: 1000
HTTP 200
[Asserts]
jsonpath "$.status" == "healthy"
jsonpath "$.database" == "connected"

# Connect-RPC port (:9101) readiness — POST to an invalid RPC path so the
# server returns a Connect-formatted error. "Connection refused" from curl
# would fail retry; a 4xx/404 from the server proves the listener is up.
POST {{connect_url}}/alt.health.v1.HealthService/Ping
Content-Type: application/json
[Options]
retry: 30
retry-interval: 500
{}
HTTP *
[Asserts]
status >= 400
status < 600
