# /api/v1/summarize?stream=true — SSE-emitting path used by the
# alt-frontend-sv reading view. summarize_handler wraps each LLM
# token in `data: <json-encoded-token>\n\n` (handler/summarize_handler.py
# lines 86-306) and may interleave `:\n\n` heartbeats during idle gaps.
# The stub emits 3 NDJSON chunks back-to-back, so heartbeats won't
# fire and the body is purely `data:` lines.
#
# Hurl 7.1 reads chunked bodies to completion before asserting; we
# can't check per-chunk arrival timing, but we can match the SSE
# framing on the assembled body.
POST {{base_url}}/api/v1/summarize
Content-Type: application/json
file,e2e/fixtures/news-creator/article-streaming.json;
HTTP 200
[Asserts]
header "Content-Type" contains "text/event-stream"
header "Cache-Control" contains "no-cache"
header "X-Accel-Buffering" == "no"
body matches "data: \".*\"\n\n"
body contains "stub-token-alpha"
body contains "stub-token-gamma"
