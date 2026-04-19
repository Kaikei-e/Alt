# Image surface. /v1/images/fetch pulls an image via the SSRF-guarded
# fetcher; /v1/images/proxy/:sig/:url serves cached OGP thumbnails and is
# DOS-whitelisted so it can ride alongside SSR pages.

GET {{base_url}}/v1/csrf-token
HTTP 200
[Captures]
csrf: jsonpath "$.csrf_token"

POST {{base_url}}/v1/images/fetch
X-Alt-Backend-Token: {{jwt}}
X-CSRF-Token: {{csrf}}
Content-Type: application/json
{
  "url": "http://stub.invalid/alt-backend/e2e/image.png"
}
HTTP *
[Asserts]
status >= 200
status < 500

# Image proxy takes /:sig/:url where sig is an HMAC and url is URL-encoded.
# Bogus signature returns 4xx; handler validates before any outbound fetch.
GET {{base_url}}/v1/images/proxy/invalidsig/http%3A%2F%2Fstub.invalid%2Falt-backend%2Fe2e%2Fimage.png
HTTP *
[Asserts]
status >= 400
status < 500
