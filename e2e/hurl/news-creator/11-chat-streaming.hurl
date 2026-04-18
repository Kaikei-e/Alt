# /api/chat?stream=true — NDJSON-emitting chat-proxy path used by Ask
# Augur for live chat. chat_handler proxies through gateway.chat_stream
# which forwards Ollama's NDJSON chunks verbatim, re-serializing each
# as `json.dumps(chunk) + "\n"` (chat_handler.py:95-107).
#
# The stub emits 3 NDJSON chunks; we assert the body shape contains
# at least one `{"message":...,"done":...}` line and the final
# `"done":true` chunk.
POST {{base_url}}/api/chat
Content-Type: application/json
file,e2e/fixtures/news-creator/chat-streaming-request.json;
HTTP 200
[Asserts]
header "Content-Type" contains "application/x-ndjson"
header "Cache-Control" contains "no-cache"
header "X-Accel-Buffering" == "no"
# chat_handler re-serializes each chunk via json.dumps, which uses
# the default `(", ", ": ")` separators — so we assert the
# space-after-colon form.
body contains "\"message\":"
body contains "\"content\":"
body contains "stub-token-alpha"
body contains "stub-token-gamma"
body contains "\"done\": true"
