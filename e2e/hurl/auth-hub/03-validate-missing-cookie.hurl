# /validate without a session cookie — echo's c.Cookie("ory_kratos_session")
# returns http.ErrNoCookie, handler short-circuits with 401 before any
# Kratos round-trip (internal/adapter/handler/validate.go:26-29).
GET {{auth_hub_url}}/validate
HTTP 401
