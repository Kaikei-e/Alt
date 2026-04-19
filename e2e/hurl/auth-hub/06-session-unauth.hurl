# /session without a cookie — session.go:47-50 returns 401 before
# usecase execution.
GET {{auth_hub_url}}/session
HTTP 401
