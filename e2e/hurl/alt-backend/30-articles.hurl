# Articles surface. /fetch/content takes a URL and returns extracted body;
# /fetch/cursor paginates; /by-tag filters by tag label; /search
# round-trips through search-indexer (stubbed). /archive is CSRF-protected.
#
# Request shapes below are pinned against the real Go structs, not
# guessed: `by-tag` reads `tag_name`/`tag_id` (article_handlers.go:251-252,
# not a generic `tag`), `/search` reads `q` (article_handlers.go:142, not
# `query`), and `/archive` binds `ArchiveArticleRequest{FeedURL, Title}`
# (schema.go:15-18, not `article_id`/`archived`). Previously this file
# used the wrong names/shapes for all three and only ever exercised the
# 400 validation-error branch, which a blanket `status >= 200; status <
# 600` assertion silently accepted as "pass".

GET {{base_url}}/v1/csrf-token
HTTP 200
[Captures]
csrf: jsonpath "$.csrf_token"

GET {{base_url}}/v1/articles/fetch/cursor?limit=10
X-Alt-Backend-Token: {{jwt}}
HTTP 200
[Asserts]
jsonpath "$" exists

# stub.invalid/.../article-1.html is allowlisted (FEED_ALLOWED_HOSTS) and
# served by the deps-stub, so this is a deterministic 200 with the fetched
# content echoed back (article_handlers.go:118-124 returnArticleResponse).
GET {{base_url}}/v1/articles/fetch/content?url=http%3A%2F%2Fstub.invalid%2Falt-backend%2Fe2e%2Farticle-1.html
X-Alt-Backend-Token: {{jwt}}
HTTP 200
[Asserts]
jsonpath "$.url" == "http://stub.invalid/alt-backend/e2e/article-1.html"
jsonpath "$.content" contains "Synthetic article body"
jsonpath "$.article_id" isString
jsonpath "$.article_id" != ""

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

# ArchiveArticleRequest requires `feed_url` (schema.go:15-18); `title` is
# optional. A well-formed, allowlisted URL archives successfully.
POST {{base_url}}/v1/articles/archive
X-Alt-Backend-Token: {{jwt}}
X-CSRF-Token: {{csrf}}
Content-Type: application/json
{
  "feed_url": "http://stub.invalid/alt-backend/e2e/article-archive-{{run_id}}.html",
  "title": "hurl-e2e archive test"
}
HTTP 200
[Asserts]
jsonpath "$.message" == "article archived"
