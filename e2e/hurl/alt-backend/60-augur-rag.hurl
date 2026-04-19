# Augur (RAG) surface. /v1/rag/context is a plain GET with auth; the
# /sse/v1/rag/answer POST is SSE — Hurl doesn't consume SSE streams
# meaningfully, so we only check it doesn't reject the request.

GET {{base_url}}/v1/csrf-token
HTTP 200
[Captures]
csrf: jsonpath "$.csrf_token"

GET {{base_url}}/v1/rag/context?query=ai
X-Alt-Backend-Token: {{jwt}}
HTTP *
[Asserts]
status >= 200
status < 500

POST {{base_url}}/sse/v1/rag/answer
X-Alt-Backend-Token: {{jwt}}
X-CSRF-Token: {{csrf}}
Content-Type: application/json
{
  "query": "summarize today's ai highlights",
  "conversation_id": "00000000-0000-0000-0000-000000000000"
}
HTTP *
[Asserts]
status >= 200
status < 500
