# Happy path for /validate — the nginx auth_request target. Auth-hub
# delegates to Kratos whoami, caches the identity, sets the four
# response headers nginx forwards upstream, and returns 204/200 with no
# body (internal/adapter/handler/validate.go:46-49).
#
# Single-tenant architecture: X-Alt-Tenant-Id == X-Alt-User-Id
# (validate.go:47 sets them from the same identity.UserID).
GET {{auth_hub_url}}/validate
Cookie: ory_kratos_session={{kratos_session_cookie}}
HTTP 200
[Asserts]
header "X-Alt-User-Id" == "{{user_id}}"
header "X-Alt-Tenant-Id" == "{{user_id}}"
header "X-Alt-User-Email" == "{{email}}"
header "X-Alt-Backend-Token" matches "^ey[A-Za-z0-9_-]+\\.ey[A-Za-z0-9_-]+\\.[A-Za-z0-9_-]+$"
