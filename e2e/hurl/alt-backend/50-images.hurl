# Image surface. /v1/images/fetch pulls an image via the SSRF-guarded
# fetcher; /v1/images/proxy/:sig/:url serves cached OGP thumbnails and is
# DOS-whitelisted so it can ride alongside SSR pages.

GET {{base_url}}/v1/csrf-token
HTTP 200
[Captures]
csrf: jsonpath "$.csrf_token"

# image_fetch requires an HTTPS target, so the plain-http stub URL is
# rejected before any outbound request ("only HTTPS URLs are allowed").
# That makes this a deterministic 400 rather than a fetch test — the stub
# serves http only, so the fetch path itself is not reachable from here.
POST {{base_url}}/v1/images/fetch
X-Alt-Backend-Token: {{jwt}}
X-CSRF-Token: {{csrf}}
Content-Type: application/json
{
  "url": "http://stub.invalid/alt-backend/e2e/image.png"
}
HTTP 400
[Asserts]
jsonpath "$.error.code" == "VALIDATION_ERROR"

# Image proxy takes /:sig/:url where sig is an HMAC and url is
# URL-encoded. This request carries no X-Alt-Backend-Token, so the auth
# middleware answers 401 and the signature is never examined — what this
# pins is that the proxy route is behind auth like every other /v1 route,
# not that a bogus signature is rejected. Signature validation stays
# uncovered here: staging never sets IMAGE_PROXY_SECRET (the backend logs
# image_proxy_misconfigured at startup), so there is no secret to sign a
# valid request with.
GET {{base_url}}/v1/images/proxy/invalidsig/http%3A%2F%2Fstub.invalid%2Falt-backend%2Fe2e%2Fimage.png
HTTP 401
