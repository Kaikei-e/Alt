# user_id switches to SearchByUserUsecase, which applies a Meilisearch
# filter on user_id. Fixtures: alice owns doc-rust-tokio, bob owns doc-rust-borrow.
GET http://search-indexer:9300/v1/search?q=rust&user_id=alice
[Options]
retry: 10
retry-interval: 500
HTTP 200
[Asserts]
jsonpath "$.query" == "rust"
jsonpath "$.hits" count == 1
jsonpath "$.hits[0].id" == "doc-rust-tokio"
jsonpath "$.total" == 1

GET http://search-indexer:9300/v1/search?q=rust&user_id=bob
HTTP 200
[Asserts]
jsonpath "$.hits" count == 1
jsonpath "$.hits[0].id" == "doc-rust-borrow"
jsonpath "$.total" == 1

# Unknown user → empty result set, not an error.
GET http://search-indexer:9300/v1/search?q=rust&user_id=nobody
HTTP 200
[Asserts]
jsonpath "$.hits" count == 0
jsonpath "$.total" == 0
