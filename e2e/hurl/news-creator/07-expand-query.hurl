# /api/v1/expand-query — rag-orchestrator (Ask Augur) calls this to
# diversify retrieval queries. The stub returns 3 newline-separated
# lines from /api/generate; expand_query_usecase splits by newline,
# dedupes, and filters instruction-leak preambles. With the stub's
# distinct ASCII lines (`stub expansion alpha keyword`, ...) all 3
# pass the filter, yielding 3 expanded_queries.
POST {{base_url}}/api/v1/expand-query
Content-Type: application/json
file,e2e/fixtures/news-creator/expand-query-request.json;
HTTP 200
[Asserts]
header "Content-Type" contains "application/json"
jsonpath "$.original_query" == "OpenAI 最新の研究"
jsonpath "$.expanded_queries" isCollection
jsonpath "$.expanded_queries" count >= 1
jsonpath "$.model" isString
jsonpath "$.model" matches ".+"
