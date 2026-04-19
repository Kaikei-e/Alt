# Delete a registered feed link. First list to capture an ID, grab a CSRF
# token, then DELETE. The DB DELETE is tenant-scoped (validated by the
# JWT), so deleting another tenant's link returns 404, not 403.

GET {{base_url}}/v1/csrf-token
HTTP 200
[Captures]
csrf: jsonpath "$.csrf_token"

GET {{base_url}}/v1/rss-feed-link/list
X-Alt-Backend-Token: {{jwt}}
HTTP 200
[Captures]
target_id: jsonpath "$[0].id"
[Asserts]
jsonpath "$" isCollection
jsonpath "$[0].id" matches "^[0-9a-f\\-]{36}$"

DELETE {{base_url}}/v1/rss-feed-link/{{target_id}}
X-Alt-Backend-Token: {{jwt}}
X-CSRF-Token: {{csrf}}
HTTP 200
[Asserts]
jsonpath "$.message" contains "unsubscribed"
