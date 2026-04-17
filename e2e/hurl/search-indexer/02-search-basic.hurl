# Unfiltered search path (no user_id) — hits SearchArticlesUsecase, used by
# internal RAG / BM25 callers. Two fixture docs match "rust".
GET http://search-indexer:9300/v1/search?q=rust&limit=10
[Options]
retry: 10
retry-interval: 500
HTTP 200
[Asserts]
jsonpath "$.query" == "rust"
jsonpath "$.hits" count >= 2
jsonpath "$.total" >= 2
jsonpath "$.hits[0].id" exists
jsonpath "$.hits[0].title" isString
jsonpath "$.hits[0].content" isString
jsonpath "$.hits[0].tags" isCollection
jsonpath "$.hits[0].score" isNumber
header "Content-Type" contains "application/json"
