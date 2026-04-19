# The HS256 JWT auth-hub mints in /validate must advertise the correct
# iss / aud / sub and a valid exp. This scenario re-calls /validate
# (cache is warm from scenario 02; the header is re-emitted every call),
# then decodes the middle segment with Hurl's base64UrlSafeDecode filter
# chain and substring-asserts each claim. Hurl has no JSON-parse filter
# over a decoded string, so we match the serialized claims directly —
# that is what's on the wire anyway.
GET {{auth_hub_url}}/validate
Cookie: ory_kratos_session={{kratos_session_cookie}}
HTTP 200
[Captures]
jwt: header "X-Alt-Backend-Token"
[Asserts]
header "X-Alt-Backend-Token" split "." nth 1 base64UrlSafeDecode decode "utf-8" contains "\"iss\":\"auth-hub\""
header "X-Alt-Backend-Token" split "." nth 1 base64UrlSafeDecode decode "utf-8" contains "\"alt-backend\""
header "X-Alt-Backend-Token" split "." nth 1 base64UrlSafeDecode decode "utf-8" contains "\"sub\":\"{{user_id}}\""
header "X-Alt-Backend-Token" split "." nth 1 base64UrlSafeDecode decode "utf-8" matches "\"exp\":[0-9]{10}"
