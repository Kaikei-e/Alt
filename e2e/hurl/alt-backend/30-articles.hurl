# Articles surface. /fetch/content takes a URL and returns extracted body;
# /fetch/cursor paginates; /by-tag filters by tag label; /search
# round-trips through search-indexer (stubbed). /archive is CSRF-protected.
#
# Request shapes below are pinned against the real Go structs: `by-tag`
# reads `tag_name`/`tag_id`, not a generic `tag`; `/search` reads `q`, not
# `query`; `/archive` binds `{feed_url, title}`, not `{article_id,
# archived}`. This file previously used the wrong names for all three and
# only ever exercised the 400 validation-error branch, which a blanket
# `status >= 200; status < 600` assertion silently accepted as "pass".
#
# The two entries that fetch an article body cannot succeed against the
# staging stack (SSRF guard, see below) and assert a bounded error band
# instead of a shape.

GET {{base_url}}/v1/csrf-token
HTTP 200
[Captures]
csrf: jsonpath "$.csrf_token"

GET {{base_url}}/v1/articles/fetch/cursor?limit=10
X-Alt-Backend-Token: {{jwt}}
HTTP 200
[Asserts]
jsonpath "$" exists

# The handler-level allowlist (FEED_ALLOWED_HOSTS) admits stub.invalid,
# but the outbound fetch is guarded a second time by the SSRF checker,
# which rejects it: stub.invalid resolves to a Docker-internal address and
# the guard reports DNS_REBINDING_BLOCKED. So the fetch never happens in
# staging and the handler answers 500 {"error":"Failed to fetch article"}.
# Retries then hit the per-host rate limiter and fail the same way.
#
# The band excludes 2xx (a success would mean the guard or the stub host
# changed and the content assertions below should come back) and 4xx
# (which would mean the URL param or allowlist regressed).
GET {{base_url}}/v1/articles/fetch/content?url=http%3A%2F%2Fstub.invalid%2Falt-backend%2Fe2e%2Farticle-1.html
X-Alt-Backend-Token: {{jwt}}
HTTP *
[Asserts]
status toString matches "^(500|503|504)$"

# tag_name is the correct query param (article_handlers.go:251); a tag
# that was never registered against any feed just yields an empty,
# non-error result set (ArticlesByTagResponse{Articles: []}).
GET {{base_url}}/v1/articles/by-tag?tag_name=ai&limit=5
X-Alt-Backend-Token: {{jwt}}
HTTP 200
[Asserts]
jsonpath "$.articles" isCollection
jsonpath "$.has_more" == false

# Nonexistent article id — FetchArticleTagsUsecase does no existence
# check, so this is a 200 with an empty tags array, not a 404
# (fetch_article_tags_usecase/usecase.go:34-41).
GET {{base_url}}/v1/articles/00000000-0000-0000-0000-000000000000/tags
X-Alt-Backend-Token: {{jwt}}
HTTP 200
[Asserts]
jsonpath "$.article_id" == "00000000-0000-0000-0000-000000000000"
jsonpath "$.tags" count == 0

# `q` is the correct query param (article_handlers.go:142). The deps-stub
# always answers SearchArticles with an empty hit set
# (alt-backend-deps-stub/main.py _EMPTY_SEARCH_RESPONSE), so the search
# usecase deterministically returns `[]`.
GET {{base_url}}/v1/articles/search?q=ai&limit=5
X-Alt-Backend-Token: {{jwt}}
HTTP 200
[Asserts]
jsonpath "$" isCollection
jsonpath "$" count == 0

# ArchiveArticleRequest requires `feed_url`, not `article_id`/`archived`;
# `title` is optional. Archiving fetches the article body before saving,
# so it hits the same SSRF guard as /fetch/content above and cannot reach
# its 200 in staging. 4xx stays excluded so a regression back to the wrong
# request shape (which only ever reached the 400 branch) fails the run.
POST {{base_url}}/v1/articles/archive
X-Alt-Backend-Token: {{jwt}}
X-CSRF-Token: {{csrf}}
Content-Type: application/json
{
  "feed_url": "http://stub.invalid/alt-backend/e2e/article-archive-{{run_id}}.html",
  "title": "hurl-e2e archive test"
}
HTTP *
[Asserts]
status toString matches "^(500|503|504)$"
