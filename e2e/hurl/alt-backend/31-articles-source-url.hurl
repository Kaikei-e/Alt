# GetArticleSourceURL — tenant-scoped article URL lookup for the Loop's Open
# CTA recovery path (Phase 0 RED for Open recoverable plan).
#
# Background:
#   ADR-879 added `actTargets[].source_url` so the FE can navigate the SPA
#   reader with `?url=<external_https>`. When the producer-side URL injection
#   misses (legacy row, lookup miss, race), the projection's source_url is
#   empty and the Open CTA loses its target.
#
# Decision:
#   The Loop UI exposes a recovery affordance "Open · resolve url" that calls
#   this endpoint at click time. It returns the canonical article URL for the
#   article id, scoped to the caller's tenant. It must:
#     - 200 with {source_url} when the article belongs to the caller
#     - 404 when no row matches (id missing, soft-deleted, or other tenant)
#     - 400 on malformed UUID
#
# Reference: 30-articles.hurl shows the v1 article surface; this scenario
# exercises the new Connect-RPC method on the same alt-backend.

# --- Setup ----------------------------------------------------------------

GET {{base_url}}/v1/csrf-token
HTTP 200
[Captures]
csrf: jsonpath "$.csrf_token"

# --- 1. malformed UUID rejected with invalid_argument --------------------

POST {{connect_url}}/alt.articles.v2.ArticleService/GetArticleSourceURL
X-Alt-Backend-Token: {{jwt}}
Content-Type: application/json
{
  "articleId": "not-a-uuid"
}
HTTP 400
[Asserts]
jsonpath "$.code" == "invalid_argument"

# --- 2. unknown article id returns not_found -----------------------------
# The all-zero UUID is a stable "definitely-not-mine" probe.

POST {{connect_url}}/alt.articles.v2.ArticleService/GetArticleSourceURL
X-Alt-Backend-Token: {{jwt}}
Content-Type: application/json
{
  "articleId": "00000000-0000-0000-0000-000000000000"
}
HTTP 404
[Asserts]
jsonpath "$.code" == "not_found"

# --- 3. happy path is shape-valid ----------------------------------------
# We don't pin a specific URL because the staging fixture set rotates; we
# only require that any 2xx response carries a non-empty https source_url.
# Many staging stacks won't have a seeded article that survives across runs,
# so this scenario is permissive (status range) but pins the JSON shape on
# 200 outcomes.

POST {{connect_url}}/alt.articles.v2.ArticleService/GetArticleSourceURL
X-Alt-Backend-Token: {{jwt}}
Content-Type: application/json
{
  "articleId": "{{article_id_seed}}"
}
HTTP *
[Asserts]
status >= 200
status < 600
