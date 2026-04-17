# Empty q → 400 with a plaintext error body.
GET http://search-indexer:9300/v1/search
HTTP 400
[Asserts]
body contains "query parameter required"

# Explicit empty value behaves the same.
GET http://search-indexer:9300/v1/search?q=
HTTP 400
[Asserts]
body contains "query parameter required"
