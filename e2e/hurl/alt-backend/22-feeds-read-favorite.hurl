# State-mutating feed endpoints. Both mark-as-read and register-favorite
# accept a feed URL; neither is idempotent from the caller's perspective
# but the DB enforces uniqueness via upsert.

GET {{base_url}}/v1/csrf-token
HTTP 200
[Captures]
csrf: jsonpath "$.csrf_token"

# ReadStatus{FeedURL} binds `feed_url` (not `url`).
#
# The happy path is unreachable in staging: MarkFeedAsRead resolves the
# body's URL to a stored feed item, and the fixture set seeds zero article
# rows (`Detailed feed stats: total_articles_count=0`), so this URL is
# never a known item. The status band excludes 2xx — a success here would
# mean the fixture changed and this scenario stopped testing what it
# claims — and excludes 4xx, which would mean the request shape regressed.
# 500 is the current answer for an unresolvable feed URL; 404 would be the
# correct one, so both are accepted rather than pinning today's bug.
POST {{base_url}}/v1/feeds/read
X-Alt-Backend-Token: {{jwt}}
X-CSRF-Token: {{csrf}}
Content-Type: application/json
{
  "feed_url": "http://stub.invalid/alt-backend/e2e/feed-register-1.xml"
}
HTTP *
[Asserts]
status toString matches "^(404|500)$"

# RssFeedLink{URL} binds `url`, not `feed_url`. The original body used
# `feed_url`, which always bound to an empty URL and only ever exercised
# RestHandleRegisterFavoriteFeed's 400 validation-error branch — masked by
# the previous `status >= 200; status < 600` assertion that accepted any
# outcome.
#
# Same reachability limit as above: the driver looks the URL up as
# `feeds.website_url`, which holds the site URL from the RSS document, not
# the .xml feed URL registered by 10-rss-feed-link-register — so no row
# matches and this cannot reach its 200. 4xx is still excluded so a
# regression back to the empty-URL binding fails the run.
POST {{base_url}}/v1/feeds/register/favorite
X-Alt-Backend-Token: {{jwt}}
X-CSRF-Token: {{csrf}}
Content-Type: application/json
{
  "url": "http://stub.invalid/alt-backend/e2e/feed-register-1.xml"
}
HTTP *
[Asserts]
status toString matches "^(404|500)$"
