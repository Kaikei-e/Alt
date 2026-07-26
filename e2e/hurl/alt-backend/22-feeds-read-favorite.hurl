# State-mutating feed endpoints. Both mark-as-read and register-favorite
# accept a feed URL; neither is idempotent from the caller's perspective
# but the DB enforces uniqueness via upsert.

GET {{base_url}}/v1/csrf-token
HTTP 200
[Captures]
csrf: jsonpath "$.csrf_token"

# ReadStatus{FeedURL} binds `feed_url` (schema.go:7-9).
POST {{base_url}}/v1/feeds/read
X-Alt-Backend-Token: {{jwt}}
X-CSRF-Token: {{csrf}}
Content-Type: application/json
{
  "feed_url": "http://stub.invalid/alt-backend/e2e/feed-register-1.xml"
}
HTTP 200
[Asserts]
jsonpath "$.message" == "Feed read status updated"

# RssFeedLink{URL} binds `url`, not `feed_url` (schema.go:3-5). The
# original body used `feed_url`, which always bound to an empty URL and
# only ever exercised RestHandleRegisterFavoriteFeed's 400
# validation-error branch (operations.go:51-52) — masked by the previous
# `status >= 200; status < 600` assertion that accepted any outcome.
POST {{base_url}}/v1/feeds/register/favorite
X-Alt-Backend-Token: {{jwt}}
X-CSRF-Token: {{csrf}}
Content-Type: application/json
{
  "url": "http://stub.invalid/alt-backend/e2e/feed-register-1.xml"
}
HTTP 200
[Asserts]
jsonpath "$.message" == "favorite feed registered"
