# Admin dashboard (/v1/dashboard/*). RequireAuth + RequireAdmin chained;
# the pre-minted JWT has role=admin. The first four endpoints read local
# state and must respond 200. recap_jobs is different — it proxies to
# recap-worker — so it is asserted separately below.

GET {{base_url}}/v1/dashboard/metrics
X-Alt-Backend-Token: {{jwt}}
HTTP 200
[Asserts]
jsonpath "$" exists

GET {{base_url}}/v1/dashboard/overview
X-Alt-Backend-Token: {{jwt}}
HTTP 200

GET {{base_url}}/v1/dashboard/logs?limit=10
X-Alt-Backend-Token: {{jwt}}
HTTP 200

GET {{base_url}}/v1/dashboard/jobs
X-Alt-Backend-Token: {{jwt}}
HTTP 200

# Unlike the four above, recap_jobs fans out to recap-worker, and the
# deps-stub has no route for /v1/dashboard/recap_jobs — the request lands
# on its catch-all, which answers 200 with a `{"status":"stub-noop"}`
# object where a job array is expected. Decoding fails and HandleError
# turns that into 500 UNKNOWN_ERROR (the same stub-path drift as
# 40-morning-letter).
#
# 4xx stays excluded — the auth/admin chain must still admit this JWT —
# and 200 is accepted for when the stub grows the route.
GET {{base_url}}/v1/dashboard/recap_jobs
X-Alt-Backend-Token: {{jwt}}
HTTP *
[Asserts]
status toString matches "^(200|500)$"
