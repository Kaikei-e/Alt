# The REST endpoint surfaces `total = len(hits)` (see search-indexer/app/rest/handler.go)
# — not Meilisearch's estimatedTotalHits. Clients that need a true total
# should call the Connect-RPC path. This test pins the observable behavior
# so the contract doesn't drift silently.
GET http://search-indexer:9300/v1/search?q=rust&limit=1
HTTP 200
[Asserts]
jsonpath "$.hits" count == 1
jsonpath "$.total" == 1
