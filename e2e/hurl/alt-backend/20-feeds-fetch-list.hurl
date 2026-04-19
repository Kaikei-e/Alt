# Feed fetch endpoints (list, limit, page). Feeds come from the three RSS
# registrations in 10; the stub serves each with a single <item>, so the
# list has at least three items. The page endpoint uses 0-based indexing
# and caps at 1000.

GET {{base_url}}/v1/feeds/fetch/list
X-Alt-Backend-Token: {{jwt}}
HTTP 200
[Asserts]
jsonpath "$" isCollection
jsonpath "$[*].link" exists
header "Cache-Control" contains "max-age="
header "ETag" contains "feeds-list"

GET {{base_url}}/v1/feeds/fetch/limit/5
X-Alt-Backend-Token: {{jwt}}
HTTP 200
[Asserts]
jsonpath "$" isCollection
jsonpath "$" count <= 5

GET {{base_url}}/v1/feeds/fetch/page/0
X-Alt-Backend-Token: {{jwt}}
HTTP 200
[Asserts]
jsonpath "$" isCollection

GET {{base_url}}/v1/feeds/fetch/page/-1
X-Alt-Backend-Token: {{jwt}}
HTTP 400

GET {{base_url}}/v1/feeds/fetch/single
X-Alt-Backend-Token: {{jwt}}
HTTP *
[Asserts]
status >= 200
status < 500
