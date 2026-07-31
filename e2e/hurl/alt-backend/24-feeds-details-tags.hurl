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

# No summary exists for this feed URL (the summary query keys on
# *article* URLs, and feed URLs are never rows in `articles.url`), so
# this is the miss branch. History: the pre-split driver surfaced the
# miss as an unwrapped pgx.ErrNoRows that fell through to a generic 500
# UNKNOWN_ERROR, and this scenario pinned that wart to catch an
# accidental silent 200. The Wave 3 data-hub migration (ADR-000954)
# then made the gateway report a miss as (nil, nil) — exactly the
# silent 200 this pin existed to catch — so the suggested fix was
# implemented for real: FeedsSummaryUsecase maps a nil summary to the
# typed FEED_NOT_FOUND, which HTTPStatusCode maps to 404.
POST {{base_url}}/v1/feeds/fetch/details
X-Alt-Backend-Token: {{jwt}}
X-CSRF-Token: {{csrf}}
Content-Type: application/json
{
  "feed_url": "http://stub.invalid/alt-backend/e2e/feed-register-1.xml"
}
HTTP 404
[Asserts]
jsonpath "$.error.code" == "FEED_NOT_FOUND"

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
