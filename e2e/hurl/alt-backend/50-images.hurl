# Image surface. /v1/images/fetch pulls an image via the SSRF-guarded
# fetcher; /v1/images/proxy/:sig/:url serves cached OGP thumbnails and is
# DOS-whitelisted so it can ride alongside SSR pages.

GET {{base_url}}/v1/csrf-token
HTTP 200
[Captures]
csrf: jsonpath "$.csrf_token"

# KNOWN BUG (recorded, not fixed — production code is out of scope for
# this suite): the *handler* SSRF check (IsAllowedURL, image_handlers.go
# :41-46) respects FEED_ALLOWED_HOSTS=stub.invalid and passes. But
# ImageFetchUsecase.Execute runs a *second*, unrelated domain check —
# domain.IsAllowedImageDomain (image_fetch_usecase.go:49-60) — against a
# small hardcoded allowlist of real news/CDN hostnames
# (domain/image_fetch.go:54-69) that has no notion of
# FEED_ALLOWED_HOSTS and does not include stub.invalid. So this request
# deterministically fails with a 400 "domain not allowed" validation
# error, never reaching the actual fetch — the two allowlists have
# silently diverged. This assertion pins the real, current behavior so
# a regression (e.g. a panic, or an accidental silent 200) is still
# caught; see bugs_found in the test-quality report for the suggested
# fix (route IMAGE_PROXY/image-fetch through the same allowlist source
# as IsAllowedURL, or extend IsAllowedImageDomain to accept an injected
# allowlist for non-prod environments).
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

# Image proxy takes /:sig/:url where sig is an HMAC and url is URL-encoded.
# In staging, IMAGE_PROXY_SECRET(_FILE) is never set
# (compose.staging.yaml's alt-backend service has no such env var, unlike
# compose/core.yaml in production), so cfg.ImageProxy.Secret is empty and
# registerImageProxyRoutes never registers the route at all
# (image_proxy_handlers.go:25-27) — every request under /v1/images/proxy
# is a plain Echo 404, regardless of signature validity. This is
# deterministic and does not depend on the "invalidsig" value.
GET {{base_url}}/v1/images/proxy/invalidsig/http%3A%2F%2Fstub.invalid%2Falt-backend%2Fe2e%2Fimage.png
HTTP 404
