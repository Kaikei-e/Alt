# /session is the frontend-facing variant — returns the full
# sessionResponse JSON shape defined in
# internal/adapter/handler/session.go:23-43. JSON tags use camelCase
# (tenantId, createdAt, lastLoginAt).
GET {{auth_hub_url}}/session
Cookie: ory_kratos_session={{kratos_session_cookie}}
HTTP 200
[Asserts]
jsonpath "$.ok" == true
jsonpath "$.user.id" == "{{user_id}}"
jsonpath "$.user.tenantId" == "{{user_id}}"
jsonpath "$.user.email" == "{{email}}"
jsonpath "$.session.id" exists
jsonpath "$.session.active" == true
header "X-Alt-Backend-Token" matches "^ey[A-Za-z0-9_-]+\\.ey[A-Za-z0-9_-]+\\.[A-Za-z0-9_-]+$"
