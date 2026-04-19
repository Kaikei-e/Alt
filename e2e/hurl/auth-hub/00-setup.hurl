# auth-hub readiness probe + Kratos identity seed + session capture.
# Runs first; every later scenario depends on the captures here.
#
# Flow:
#   1. Wait for auth-hub /health.
#   2. Wait for Kratos AdminAPI readiness.
#   3. Idempotently create an identity (200/201 on fresh DB, 409 on
#      compose-up with a persistent volume — run.sh's `down -v`
#      cleanup keeps CI on the 200 branch).
#   4. Init the self-service **browser** flow with
#      `Accept: application/json`: Kratos returns the flow JSON and
#      sets a per-flow `csrf_token_<hash>` cookie that Hurl's cookie
#      store carries to the submit step. The browser flow is the only
#      path in Kratos v1.3 that issues the HMAC-signed
#      `ory_kratos_session` cookie the auth-hub handlers parse — the
#      api flow's `session_token` is opaque and is NOT a valid cookie
#      value for self-hosted Kratos.
#   5. Submit the password. Kratos sets `ory_kratos_session`; Hurl
#      captures that cookie value and the identity/session identifiers.
GET {{auth_hub_url}}/health
[Options]
retry: 60
retry-interval: 1000
HTTP 200
[Asserts]
jsonpath "$.status" == "healthy"

GET {{kratos_admin_url}}/admin/health/ready
[Options]
retry: 30
retry-interval: 1000
HTTP 200

POST {{kratos_admin_url}}/admin/identities
Content-Type: application/json
{
  "schema_id": "default",
  "traits": {
    "email": "{{email}}"
  },
  "credentials": {
    "password": {
      "config": {
        "password": "{{password}}"
      }
    }
  },
  "state": "active"
}
HTTP *
[Asserts]
status toString matches "^(200|201|409)$"

# Init browser flow. `Accept: application/json` makes Kratos respond
# with JSON instead of a 303 redirect to the UI stub, and the response
# still sets the csrf_token_<hash> cookie Hurl needs to include on the
# submit step.
GET {{kratos_public_url}}/self-service/login/browser
Accept: application/json
HTTP 200
[Captures]
login_action: jsonpath "$.ui.action"
csrf_token: jsonpath "$.ui.nodes[0].attributes.value"

# Submit login. Hurl's cookie jar auto-appends the csrf_token_<hash>
# cookie captured on the previous response to the same host. On
# success Kratos sets `ory_kratos_session` (HMAC-signed) — that's the
# cookie value auth-hub's /validate, /session, /csrf all read.
POST {{login_action}}
Accept: application/json
Content-Type: application/json
{
  "method": "password",
  "identifier": "{{email}}",
  "password": "{{password}}",
  "csrf_token": "{{csrf_token}}"
}
HTTP 200
[Captures]
kratos_session_cookie: cookie "ory_kratos_session"
user_id: jsonpath "$.session.identity.id"
session_id: jsonpath "$.session.id"
[Asserts]
jsonpath "$.session.identity.traits.email" == "{{email}}"
jsonpath "$.session.active" == true
