# RSS feed link registration. Each register call fetches the feed URL via
# the validateAndFetchPort, which round-trips through alt-backend-deps-stub
# serving a minimal RSS 2.0 document at /alt-backend/e2e/feed-register-*.xml.
# FEED_ALLOWED_HOSTS=stub.invalid bypasses the SSRF private-IP check.

GET {{base_url}}/v1/csrf-token
HTTP 200
[Captures]
csrf: jsonpath "$.csrf_token"

POST {{base_url}}/v1/rss-feed-link/register
X-Alt-Backend-Token: {{jwt}}
X-CSRF-Token: {{csrf}}
Content-Type: application/json
file,e2e/fixtures/alt-backend/register-feed-1.json;
HTTP 200
[Asserts]
jsonpath "$.message" contains "registered"

POST {{base_url}}/v1/rss-feed-link/register
X-Alt-Backend-Token: {{jwt}}
X-CSRF-Token: {{csrf}}
Content-Type: application/json
file,e2e/fixtures/alt-backend/register-feed-2.json;
HTTP 200

POST {{base_url}}/v1/rss-feed-link/register
X-Alt-Backend-Token: {{jwt}}
X-CSRF-Token: {{csrf}}
Content-Type: application/json
file,e2e/fixtures/alt-backend/register-feed-3.json;
HTTP 200

# Negative: SSRF guard should reject a loopback URL even with a valid JWT.
POST {{base_url}}/v1/rss-feed-link/register
X-Alt-Backend-Token: {{jwt}}
X-CSRF-Token: {{csrf}}
Content-Type: application/json
{
  "url": "http://127.0.0.1:9000/evil-feed.xml"
}
HTTP 400
