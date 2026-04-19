# Feed search dispatches to search-indexer (Connect-RPC). In staging the
# deps-stub is aliased as `search-indexer` and serves a fixed result set
# at /alt.search.v2.SearchService/SearchFeeds and /SearchArticles.

GET {{base_url}}/v1/csrf-token
HTTP 200
[Captures]
csrf: jsonpath "$.csrf_token"

POST {{base_url}}/v1/feeds/search
X-Alt-Backend-Token: {{jwt}}
X-CSRF-Token: {{csrf}}
Content-Type: application/json
{
  "query": "ai"
}
HTTP 200
[Asserts]
jsonpath "$" exists
