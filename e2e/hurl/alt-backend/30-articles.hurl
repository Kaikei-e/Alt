# Articles surface. /fetch/content takes a URL and returns extracted body;
# /fetch/cursor paginates; /by-tag filters by tag label; /search
# round-trips through search-indexer (stubbed). /archive is CSRF-protected.

GET {{base_url}}/v1/csrf-token
HTTP 200
[Captures]
csrf: jsonpath "$.csrf_token"

GET {{base_url}}/v1/articles/fetch/cursor?limit=10
X-Alt-Backend-Token: {{jwt}}
HTTP 200
[Asserts]
jsonpath "$" exists

GET {{base_url}}/v1/articles/fetch/content?url=http%3A%2F%2Fstub.invalid%2Falt-backend%2Fe2e%2Farticle-1.html
X-Alt-Backend-Token: {{jwt}}
HTTP *
[Asserts]
status >= 200
status < 500

GET {{base_url}}/v1/articles/by-tag?tag=ai&limit=5
X-Alt-Backend-Token: {{jwt}}
HTTP 200
[Asserts]
jsonpath "$" exists

GET {{base_url}}/v1/articles/00000000-0000-0000-0000-000000000000/tags
X-Alt-Backend-Token: {{jwt}}
HTTP *
[Asserts]
status >= 200
status < 500

GET {{base_url}}/v1/articles/search?query=ai&limit=5
X-Alt-Backend-Token: {{jwt}}
HTTP 200
[Asserts]
jsonpath "$" exists

POST {{base_url}}/v1/articles/archive
X-Alt-Backend-Token: {{jwt}}
X-CSRF-Token: {{csrf}}
Content-Type: application/json
{
  "article_id": "00000000-0000-0000-0000-000000000000",
  "archived": true
}
HTTP *
[Asserts]
status >= 200
status < 500
