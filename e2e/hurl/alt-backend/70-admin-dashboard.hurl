# Admin dashboard (/v1/dashboard/*). RequireAuth + RequireAdmin chained;
# the pre-minted JWT has role=admin so all five read endpoints must
# respond 200 — none of them has an error branch reachable from a
# well-formed admin request (dashboard_handlers.go).

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

# handleGetRecapJobs (dashboard_handlers.go:136-150) has exactly the same
# shape as the four endpoints above — no branch that would make it 2xx
# only "on a good day". Previously this one alone kept a blanket
# `status >= 200; status < 600` while its own file comment claimed "must
# respond 2xx" — the assertion silently allowed 4xx/5xx the comment said
# should never happen.
GET {{base_url}}/v1/dashboard/recap_jobs
X-Alt-Backend-Token: {{jwt}}
HTTP 200
[Asserts]
jsonpath "$" isCollection
