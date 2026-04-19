# Read-back of the three feeds registered in 10. The list handler returns
# health metadata per link (ConsecutiveFailures, IsActive) in addition to
# id/url. /random picks one at random — we only assert that the response
# body parses as JSON with a non-empty url field.

GET {{base_url}}/v1/rss-feed-link/list
X-Alt-Backend-Token: {{jwt}}
HTTP 200
[Asserts]
jsonpath "$" isCollection
jsonpath "$" count >= 3
jsonpath "$[0].url" exists
jsonpath "$[0].id" matches "^[0-9a-f\\-]{36}$"
jsonpath "$[0].health_status" exists

GET {{base_url}}/v1/rss-feed-link/random
X-Alt-Backend-Token: {{jwt}}
HTTP 200
[Asserts]
jsonpath "$.url" exists

GET {{base_url}}/v1/rss-feed-link/export/opml
X-Alt-Backend-Token: {{jwt}}
HTTP 200
[Asserts]
header "Content-Type" contains "xml"
body contains "<opml"
body contains "stub.invalid"
