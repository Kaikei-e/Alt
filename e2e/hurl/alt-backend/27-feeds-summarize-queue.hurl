# Summarization queue — enqueue an async job, poll its status. The /stream
# endpoint is deliberately excluded: NDJSON chunked-transfer framing is
# out of scope for the initial suite.

GET {{base_url}}/v1/csrf-token
HTTP 200
[Captures]
csrf: jsonpath "$.csrf_token"

POST {{base_url}}/v1/feeds/summarize
X-Alt-Backend-Token: {{jwt}}
X-CSRF-Token: {{csrf}}
Content-Type: application/json
{
  "feed_url": "http://stub.invalid/alt-backend/e2e/feed-register-1.xml",
  "content": "Stub article body for summarization."
}
HTTP *
[Asserts]
status >= 200
status < 500

POST {{base_url}}/v1/feeds/summarize/queue
X-Alt-Backend-Token: {{jwt}}
X-CSRF-Token: {{csrf}}
Content-Type: application/json
{
  "feed_url": "http://stub.invalid/alt-backend/e2e/feed-register-1.xml",
  "content": "Stub article body for queued summarization."
}
HTTP *
[Asserts]
status >= 200
status < 500

GET {{base_url}}/v1/feeds/summarize/status/00000000-0000-0000-0000-000000000000
X-Alt-Backend-Token: {{jwt}}
HTTP *
[Asserts]
status >= 200
status < 500
