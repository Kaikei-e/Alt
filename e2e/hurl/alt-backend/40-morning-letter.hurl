# Morning letter — dispatches to recap-worker (HTTP/REST).
#
# KNOWN BUG (recorded, not fixed — the deps-stub lives under e2e/stubs/,
# out of scope for this suite): GetOvernightUpdates's repository
# (morning_gateway.go:48-52) calls `GET {RECAP_WORKER_URL}/v1/morning/
# updates?since=...` and decodes the body as a JSON *array*
# ([]MorningArticleGroupResponse). The deps-stub only implements
# `GET /morning-letter/{user_id}` (a *document*, used by the unrelated
# MorningLetterGateway for /v1/morning/letters/*) — it has no route for
# `/v1/morning/updates`, so the request falls through to the stub's
# catch-all, which answers 200 with a JSON *object*
# ({"status":"stub-noop",...}). Decoding an object into a slice fails
# (encoding/json), morning_gateway.go:82-85 turns that into an error, and
# handleMorningUpdates (morning_handlers.go:35-38) maps it to a generic
# 500. This is exactly the "stub drift" scenario the deps-stub's own
# module docstring calls out ("shows up ... as a Hurl assertion
# failure") — previously masked here by `status >= 200; status < 600`.
# This assertion pins the real, current behavior so further regressions
# are still caught; see bugs_found in the test-quality report.
GET {{base_url}}/v1/morning-letter/updates
X-Alt-Backend-Token: {{jwt}}
HTTP 500
[Asserts]
jsonpath "$.error.code" == "UNKNOWN_ERROR"
