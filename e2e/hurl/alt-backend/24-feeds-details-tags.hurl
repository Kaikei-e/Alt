# Feed details + tag lookups. POST /fetch/details binds
# FeedUrlPayload{FeedURL} — a single `feed_url` string, NOT a `feed_urls`
# list (schema.go:11-13; that plural shape belongs to the unrelated
# /fetch/summary endpoints' FeedSummaryRequest). POST /tags binds
# FeedTagsPayload{FeedURL}, same singular convention (types.go:50-51).
# GET /:id/tags reads by DB UUID.
#
# The previous `feed_urls: [...]` body on both POST calls always bound to
# an empty FeedURL, so both requests only ever exercised the 400
# validation branch — masked by a `status >= 200; status < 600`
# assertion that accepted literally any outcome, success or failure.

GET {{base_url}}/v1/csrf-token
HTTP 200
[Captures]
csrf: jsonpath "$.csrf_token"

# KNOWN BUG (recorded, not fixed — production code is out of scope for
# this suite): FetchFeedSummary's SQL joins article_summaries to
# `articles a ON a.url = $1`, i.e. it expects an *article* URL, not an
# RSS *feed* URL — feed URLs are never rows in `articles.url`, so this
# join is guaranteed zero rows regardless of any other test's ordering.
# The driver returns pgx.ErrNoRows unwrapped
# (fetch_feed_details_driver.go:9-32), which HandleError has no
# not-found branch for (utils/errors/app_context_error.go:42-61 has no
# NOT_FOUND case) and so falls through to a generic 500
# UNKNOWN_ERROR — a "just wasn't found" case surfaced as a server error
# instead of 404. This assertion pins the real, current behavior so a
# regression (e.g. a panic, or an accidental silent 200) is still
# caught; see bugs_found in the test-quality report for the suggested
# fix (map pgx.ErrNoRows to a typed not-found error, same pattern
# scraping_domain_driver.go already uses for GetScrapingDomainByID).
POST {{base_url}}/v1/feeds/fetch/details
X-Alt-Backend-Token: {{jwt}}
X-CSRF-Token: {{csrf}}
Content-Type: application/json
{
  "feed_url": "http://stub.invalid/alt-backend/e2e/feed-register-1.xml"
}
HTTP 500
[Asserts]
jsonpath "$.error.code" == "UNKNOWN_ERROR"

# feed-register-1.xml is registered by 10-rss-feed-link-register.hurl.
# FetchFeedTagsGateway.FetchFeedTags is a list query (unlike the
# single-row QueryRow above), so zero tags on a real feed is a normal
# empty result, not an error.
POST {{base_url}}/v1/feeds/tags
X-Alt-Backend-Token: {{jwt}}
X-CSRF-Token: {{csrf}}
Content-Type: application/json
{
  "feed_url": "http://stub.invalid/alt-backend/e2e/feed-register-1.xml"
}
HTTP 200
[Asserts]
jsonpath "$.feed_url" == "http://stub.invalid/alt-backend/e2e/feed-register-1.xml"
jsonpath "$.tags" count == 0

# Nonexistent feed ID — FetchFeedTagsByIDUsecase does no existence
# check before the list query, so this is 200 with an empty array, not
# a 404 (mirrors handleFetchArticleTags's by-ID behavior in
# article_handlers.go:339-375).
GET {{base_url}}/v1/feeds/00000000-0000-0000-0000-000000000000/tags
X-Alt-Backend-Token: {{jwt}}
HTTP 200
[Asserts]
jsonpath "$.feed_id" == "00000000-0000-0000-0000-000000000000"
jsonpath "$.tags" count == 0
