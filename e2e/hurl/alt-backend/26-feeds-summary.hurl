# Article summary endpoints. /fetch/summary/provided reads a pre-generated
# Inoreader summary from DB; /fetch/summary triggers the pre-processor
# Connect-RPC path (stubbed).

GET {{base_url}}/v1/csrf-token
HTTP 200
[Captures]
csrf: jsonpath "$.csrf_token"

POST {{base_url}}/v1/feeds/fetch/summary/provided
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

POST {{base_url}}/v1/feeds/fetch/summary
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
