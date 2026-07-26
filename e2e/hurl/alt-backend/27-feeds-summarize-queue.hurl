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

# KNOWN BUG (recorded, not fixed — the deps-stub lives under e2e/stubs/,
# out of scope for this suite): the pre-processor client actually wired
# via DI (preprocessor_client.Client, not the Connect one) is a plain
# REST client — Summarize POSTs to `{PRE_PROCESSOR_URL}/api/v1/summarize`
# (preprocessor_client/client.go:80). The deps-stub only implements the
# Connect-RPC-style `/alt.preprocessor.v2.PreProcessorService/
# SummarizeArticle` path, so this request falls through to the stub's
# catch-all (200 + `{"status":"stub-noop",...}`). Decoding that into
# `{Success, Summary, ArticleID}` yields `Success=false` (the key is
# absent), and client.go:105-107 turns "not success" into an error —
# EnsureSummary propagates it (usecase.go:94-97) and the handler answers
# 500 (request_handlers.go:60-64). EnsureArticle (the DB save) runs
# first and still succeeds/commits despite the later failure — that
# matters for the next request below.
POST {{base_url}}/v1/feeds/summarize
X-Alt-Backend-Token: {{jwt}}
X-CSRF-Token: {{csrf}}
Content-Type: application/json
{
  "feed_url": "http://stub.invalid/alt-backend/e2e/feed-register-1.xml",
  "content": "Stub article body for summarization."
}
HTTP 500
[Asserts]
jsonpath "$.detail" == "Failed to generate summary"

# Same feed_url as above, same file (Hurl runs entries within one file
# strictly in order): EnsureArticle finds the article the previous
# request already saved (existed=true) — that part isn't stub-drift
# affected. But no summary was ever persisted (the previous request
# errored out before reaching SaveArticleSummary), so GetCachedSummary
# misses and this falls to QueueSummary, which POSTs to
# `/api/v1/summarize/queue` and requires exactly HTTP 202
# (preprocessor_client/client.go:170-185) — the stub's catch-all 200
# fails that check the same way. request_handlers.go:126-128 maps the
# error to 500, same as /summarize above.
POST {{base_url}}/v1/feeds/summarize/queue
X-Alt-Backend-Token: {{jwt}}
X-CSRF-Token: {{csrf}}
Content-Type: application/json
{
  "feed_url": "http://stub.invalid/alt-backend/e2e/feed-register-1.xml",
  "content": "Stub article body for queued summarization."
}
HTTP 500
[Asserts]
jsonpath "$.detail" == "Failed to queue summarization job"

# GetSummarizeStatus (status_handlers.go:24) is the one preprocessor_client
# REST call that only special-cases exactly 200 vs 404
# (preprocessor_client/client.go:217-224) — no "must be 202" style check
# like QueueSummarize above — so the stub catch-all's 200 slips through
# undetected as "success" instead of erroring like its two siblings
# above. The decoded body still deserializes into a non-nil
# SummarizeStatus with Status="stub-noop", so the handler's 404 branch
# (status_handlers.go:30-32) is never reached for this job id either.
GET {{base_url}}/v1/feeds/summarize/status/00000000-0000-0000-0000-000000000000
X-Alt-Backend-Token: {{jwt}}
HTTP 200
[Asserts]
jsonpath "$.status" == "stub-noop"
