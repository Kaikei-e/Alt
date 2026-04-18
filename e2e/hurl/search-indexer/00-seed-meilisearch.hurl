# Seeds the Meilisearch `articles` index with the fixture corpus.
# Must run (serially) before any NN-search-*.hurl file.
#
# The `articles` index is created + configured (filterable: tags, user_id)
# by search-indexer's bootstrap.EnsureIndex on process start
# (search-indexer/app/bootstrap/app.go), so we assume search-indexer is
# already healthy when this file runs.

# Meilisearch liveness — retry a few times in case compose up --wait raced.
GET http://meilisearch:7700/health
[Options]
retry: 20
retry-interval: 500
HTTP 200
[Asserts]
jsonpath "$.status" == "available"

# Push the fixture docs. Bearer auth uses the staging master key.
POST http://meilisearch:7700/indexes/articles/documents
Authorization: Bearer {{meili_master_key}}
Content-Type: application/json
file,../../fixtures/search-indexer/seed-docs.json;
HTTP 202
[Captures]
task_uid: jsonpath "$.taskUid"

# Block until the indexing task settles so downstream files see a stable index.
GET http://meilisearch:7700/tasks/{{task_uid}}
Authorization: Bearer {{meili_master_key}}
[Options]
retry: 40
retry-interval: 250
HTTP 200
[Asserts]
jsonpath "$.status" == "succeeded"
