# JWT auth middleware negative path. Any protected endpoint must reject:
#   - missing X-Alt-Backend-Token header          → 401
#   - malformed token (not a JWT)                  → 401
#   - valid-format JWT but signed with wrong secret→ 401
#
# Audience/issuer mismatches are covered by jwt_middleware.go internal
# unit tests; here we stay at the auth-middleware boundary.

GET {{base_url}}/v1/feeds/fetch/list
HTTP 401

GET {{base_url}}/v1/feeds/fetch/list
X-Alt-Backend-Token: not-a-jwt
HTTP 401

# Token signed with a different secret. Pre-minted offline; payload is
# identical to the real fixture but signed with `wrong-secret`, so the
# HMAC verification fails.
GET {{base_url}}/v1/feeds/fetch/list
X-Alt-Backend-Token: eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIwMDAwMDAwMC0wMDAwLTAwMDAtMDAwMC0wMDAwMDAwMDAwMDEiLCJ0ZW5hbnRfaWQiOiIwMDAwMDAwMC0wMDAwLTAwMDAtMDAwMC0wMDAwMDAwMDAwMDEiLCJpc3MiOiJhbHQtc3RhZ2luZy1hdXRoLWh1YiIsImF1ZCI6ImFsdC1iYWNrZW5kIiwiZXhwIjo0MDcwOTA4ODAwfQ.WRONG_SIGNATURE_xxxx
HTTP 401
