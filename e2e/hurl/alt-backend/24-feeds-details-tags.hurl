# Feed details + tag lookups. POST /fetch/details and POST /tags both use
# the `feed_urls` list; GET /:id/tags reads by DB UUID.

GET {{base_url}}/v1/csrf-token
HTTP 200
[Captures]
csrf: jsonpath "$.csrf_token"

POST {{base_url}}/v1/feeds/fetch/details
X-Alt-Backend-Token: {{jwt}}
X-CSRF-Token: {{csrf}}
Content-Type: application/json
{
  "feed_urls": ["http://stub.invalid/alt-backend/e2e/feed-register-1.xml"]
}
HTTP *
[Asserts]
status >= 200
status < 600

POST {{base_url}}/v1/feeds/tags
X-Alt-Backend-Token: {{jwt}}
X-CSRF-Token: {{csrf}}
Content-Type: application/json
{
  "feed_urls": ["http://stub.invalid/alt-backend/e2e/feed-register-1.xml"]
}
HTTP *
[Asserts]
status >= 200
status < 600

GET {{base_url}}/v1/feeds/00000000-0000-0000-0000-000000000000/tags
X-Alt-Backend-Token: {{jwt}}
HTTP *
[Asserts]
status >= 200
status < 500
