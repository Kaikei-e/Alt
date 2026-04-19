# /validate with a well-formed but invalid session value — cookie
# parsing succeeds, usecase calls Kratos whoami, Kratos rejects the
# session, domain.ErrAuthFailed bubbles up, error_mapper.go:15-20 maps
# to 401. Exercises the Kratos-negative branch, unlike 03 which only
# exercises the short-circuit.
GET {{auth_hub_url}}/validate
Cookie: ory_kratos_session=deadbeef-not-a-valid-session-token-value
HTTP 401
