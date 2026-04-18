# /api/chat (stream=false) — chat-proxy non-streaming path used by the
# Morning Letter usecase and by smoke checks. chat_handler.chat_endpoint
# forwards through gateway.chat_generate which proxies the stub's
# /api/chat. Stub returns a JSON-string content (QueryPlan-shaped); the
# proxy passes the whole Ollama response through verbatim, so we assert
# on the envelope shape only.
POST {{base_url}}/api/chat
Content-Type: application/json
file,e2e/fixtures/news-creator/chat-non-stream-request.json;
HTTP 200
[Asserts]
header "Content-Type" contains "application/json"
jsonpath "$.model" isString
jsonpath "$.message.role" == "assistant"
jsonpath "$.message.content" isString
jsonpath "$.message.content" matches ".+"
jsonpath "$.done" == true
