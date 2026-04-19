# Missing X-Internal-Auth header — middleware short-circuits with 401
# "missing internal auth header" (internal_auth.go:19-21).
GET {{auth_hub_url}}/internal/system-user
HTTP 401
