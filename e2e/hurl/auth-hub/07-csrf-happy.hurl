# /csrf issues a double-submit token for subsequent state-mutating
# requests. Handler reads the raw Cookie header (csrf.go:34) and
# extracts ory_kratos_session=VALUE manually; usecase validates the
# session with Kratos before minting the HMAC token.
POST {{auth_hub_url}}/csrf
Cookie: ory_kratos_session={{kratos_session_cookie}}
HTTP 200
[Asserts]
jsonpath "$.data.csrf_token" matches "^[A-Za-z0-9_+/=.-]{16,}$"
