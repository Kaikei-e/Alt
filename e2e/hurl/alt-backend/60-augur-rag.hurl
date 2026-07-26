# Augur (RAG) surface. /v1/rag/context is a plain GET with auth; the
# /sse/v1/rag/answer POST is SSE — Hurl doesn't consume SSE streams
# meaningfully, so we only check it doesn't reject the request.
#
# Request shapes fixed against the real Go code: the alt-backend handler
# reads query param `q` (augur_handler.go:30), not `query`; AnswerRequest
# binds `messages: [{role, content}]` + `stream` (augur_handler.go:50-53),
# not a bespoke `query`/`conversation_id` shape.

GET {{base_url}}/v1/csrf-token
HTTP 200
[Captures]
csrf: jsonpath "$.csrf_token"

# KNOWN BUG (recorded, not fixed — the deps-stub lives under e2e/stubs/,
# out of scope for this suite): the alt-backend->rag-orchestrator REST
# client calls `POST /v1/rag/retrieve` (rag_gateway's generated
# operationPath), but the deps-stub only implements `/v1/context` (REST)
# and the Connect-RPC `/alt.augur.v2.AugurService/RetrieveContext` — no
# route for `/v1/rag/retrieve`. The request falls through to the stub's
# catch-all (200 + `{"status":"stub-noop",...}`), which decodes into a
# zero-value RetrieveResponse; augur_adapter.go:72-74 explicitly rejects
# a nil `Contexts` field as "rag-orchestrator returned empty response",
# and RetrieveContext (augur_handler.go:35-38) maps that to a generic
# 500. This is the same class of stub-path drift as
# 40-morning-letter.hurl; see bugs_found in the test-quality report.
GET {{base_url}}/v1/rag/context?q=ai
X-Alt-Backend-Token: {{jwt}}
HTTP 500
[Asserts]
jsonpath "$.error.code" == "UNKNOWN_ERROR"

# Same stub-path drift as above: the real client calls
# `POST /v1/rag/answer` (not the stub's `/v1/answer`) and also misses.
# Unlike RetrieveContext, Answer's non-streaming branch treats a missing
# `JSON200.Answer` as "no answer" rather than an error
# (augur_adapter.go:157-164 closes the channel with nothing sent instead
# of returning an error), so the handler still responds 200 with an
# empty answer rather than 500.
POST {{base_url}}/sse/v1/rag/answer
X-Alt-Backend-Token: {{jwt}}
X-CSRF-Token: {{csrf}}
Content-Type: application/json
{
  "messages": [
    {"role": "user", "content": "summarize today's ai highlights"}
  ],
  "stream": false
}
HTTP 200
[Asserts]
jsonpath "$.answer" == ""
