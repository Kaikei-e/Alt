# State-mutating feed endpoints. Both mark-as-read and register-favorite
# accept a feed URL; neither is idempotent from the caller's perspective
# but the DB enforces uniqueness via upsert.

GET {{base_url}}/v1/csrf-token
HTTP 200
[Captures]
csrf: jsonpath "$.csrf_token"

POST {{base_url}}/v1/feeds/read
X-Alt-Backend-Token: {{jwt}}
X-CSRF-Token: {{csrf}}
Content-Type: application/json
{
  "feed_url": "http://stub.invalid/alt-backend/e2e/feed-register-1.xml"
}
HTTP *
[Asserts]
status >= 200
status < 500

POST {{base_url}}/v1/feeds/register/favorite
X-Alt-Backend-Token: {{jwt}}
X-CSRF-Token: {{csrf}}
Content-Type: application/json
{
  "feed_url": "http://stub.invalid/alt-backend/e2e/feed-register-1.xml"
}
HTTP *
[Asserts]
status >= 200
status < 500
