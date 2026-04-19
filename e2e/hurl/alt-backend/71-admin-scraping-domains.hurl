# Admin scraping-domain policies at /v1/admin/scraping-domains. List is
# always 200 (possibly empty); GET/:id and PATCH/:id behave sanely for
# unknown UUIDs (404 or 200 with default policy — accept both).

GET {{base_url}}/v1/csrf-token
HTTP 200
[Captures]
csrf: jsonpath "$.csrf_token"

GET {{base_url}}/v1/admin/scraping-domains
X-Alt-Backend-Token: {{jwt}}
HTTP 200
[Asserts]
jsonpath "$" exists

GET {{base_url}}/v1/admin/scraping-domains/00000000-0000-0000-0000-000000000000
X-Alt-Backend-Token: {{jwt}}
HTTP *
[Asserts]
status >= 200
status < 600

PATCH {{base_url}}/v1/admin/scraping-domains/00000000-0000-0000-0000-000000000000
X-Alt-Backend-Token: {{jwt}}
X-CSRF-Token: {{csrf}}
Content-Type: application/json
{
  "respect_robots_txt": true
}
HTTP *
[Asserts]
status >= 200
status < 600
