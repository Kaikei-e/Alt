# limit is permissive: 0, >1000, or non-numeric values are ignored and fall
# back to the default (50). The handler does not 400 on out-of-range limit.
# Matches are the 2 "rust" docs in fixtures.
GET http://search-indexer:9300/v1/search?q=rust&limit=0
HTTP 200
[Asserts]
jsonpath "$.total" == 2

GET http://search-indexer:9300/v1/search?q=rust&limit=1001
HTTP 200
[Asserts]
jsonpath "$.total" == 2

GET http://search-indexer:9300/v1/search?q=rust&limit=notanumber
HTTP 200
[Asserts]
jsonpath "$.total" == 2
