# Aggregate feed stats. All three endpoints read from the alt_db and do
# not call out to external services, so they should be fast and
# deterministic against the seeded DB.

GET {{base_url}}/v1/feeds/stats
X-Alt-Backend-Token: {{jwt}}
HTTP 200
[Asserts]
jsonpath "$" exists

GET {{base_url}}/v1/feeds/stats/detailed
X-Alt-Backend-Token: {{jwt}}
HTTP 200
[Asserts]
jsonpath "$" exists

GET {{base_url}}/v1/feeds/stats/trends
X-Alt-Backend-Token: {{jwt}}
HTTP 200
[Asserts]
jsonpath "$" exists
