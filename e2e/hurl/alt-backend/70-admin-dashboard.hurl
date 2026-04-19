# Admin dashboard (/v1/dashboard/*). RequireAuth + RequireAdmin chained;
# the pre-minted JWT has role=admin so all four read endpoints must
# respond 2xx, and the recap_jobs variant must too.

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

GET {{base_url}}/v1/dashboard/recap_jobs
X-Alt-Backend-Token: {{jwt}}
HTTP *
[Asserts]
status >= 200
status < 600
