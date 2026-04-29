# Admin: EmitArticleUrlBackfill (PR3 — ADR-000869).
#
# Verifies the new Connect-RPC route on KnowledgeHomeAdminService is
# wired and returns the expected response envelope. The staging slice
# boots with an empty `articles` table (Atlas migrator only creates
# schema), so dry_run=true must report all-zero counters and not
# touch the sovereign event log.
#
# Why dry_run only here: real-emit asserts belong to the unit test
# (alt-backend/app/usecase/knowledge_url_backfill_usecase/usecase_test.go)
# and to the CDC contract test (alt-backend/app/driver/sovereign_client/
# contract/article_url_backfill_consumer_test.go). The E2E proves only
# the boundary: handler is registered, request envelope decodes,
# response envelope encodes, no panic on the empty-table path.
#
# Auth: KnowledgeHomeAdminService is service-to-service (BFF -> backend
# at the admin-only edge) and stages mTLS at TLS termination. The
# alt-staging slice exposes the Connect-RPC port without TLS for the
# E2E, mirroring the pattern used by 26-knowledge-loop-canonical-contract.

POST {{connect_url}}/alt.knowledge_home.v1.KnowledgeHomeAdminService/EmitArticleUrlBackfill
X-Alt-Backend-Token: {{jwt}}
Content-Type: application/json
{
  "maxArticles": 0,
  "dryRun": true
}
HTTP 200
[Asserts]
jsonpath "$.articlesScanned" == 0
jsonpath "$.eventsAppended" == 0
jsonpath "$.skippedBlockedScheme" == 0
jsonpath "$.skippedDuplicate" == 0
jsonpath "$.moreRemaining" == false

# A second call with maxArticles set must round-trip the field without
# error. Counters stay zero because there are no articles to walk.
POST {{connect_url}}/alt.knowledge_home.v1.KnowledgeHomeAdminService/EmitArticleUrlBackfill
X-Alt-Backend-Token: {{jwt}}
Content-Type: application/json
{
  "maxArticles": 50,
  "dryRun": true
}
HTTP 200
[Asserts]
jsonpath "$.articlesScanned" == 0
jsonpath "$.moreRemaining" == false
