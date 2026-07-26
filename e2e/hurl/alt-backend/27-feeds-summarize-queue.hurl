# Summarization queue — enqueue an async job, poll its status. The /stream
# endpoint is deliberately excluded: NDJSON chunked-transfer framing is
# out of scope for the initial suite.
#
# Both handlers only bind `feed_url` (the anonymous struct in
# request_handlers.go:27-29/84-86 has no `content` field — the literal
# below keeps `content` for readability but it is ignored server-side).

GET {{base_url}}/v1/csrf-token
HTTP 200
[Captures]
csrf: jsonpath "$.csrf_token"

# Neither summarize path reaches the pre-processor in staging. Both call
# EnsureArticle first, which fetches the article body, and the SSRF guard
# rejects any stub.invalid URL there — it resolves to a Docker-internal
# address, which the guard reports as DNS_REBINDING_BLOCKED. The handler
# maps that to 500 "Failed to check article existence" via
# echo.NewHTTPError, so the body is not the AppError envelope the other
# scenarios assert and is not reliably JSON at all — only the status is
# pinned here.
#
# The band excludes 2xx (a success would mean the guard or the stub host
# changed and this scenario needs revisiting) and 4xx (which would mean
# the request shape regressed).
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
status toString matches "^(500|503|504)$"

# Same EnsureArticle prerequisite, same outcome — the queue path is never
# reached either.
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
status toString matches "^(500|503|504)$"

# Status polling does not fetch anything, so it does reach the
# pre-processor client. For an unknown job id a real pre-processor answers
# 404 and the handler surfaces that; the deps-stub's catch-all answers 200
# instead, which the client accepts. Both are legitimate depending on what
# is behind PRE_PROCESSOR_URL, so the band covers both rather than
# encoding which one the stub happens to be.
GET {{base_url}}/v1/feeds/summarize/status/00000000-0000-0000-0000-000000000000
X-Alt-Backend-Token: {{jwt}}
HTTP *
[Asserts]
status toString matches "^(200|404)$"
