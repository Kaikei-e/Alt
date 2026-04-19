# OPML import — multipart upload with three new feed URLs. Exercises the
# body-size limit (2MB), multipart parsing, and the same validateAndFetch
# path as the JSON register handler, one call per outline.

GET {{base_url}}/v1/csrf-token
HTTP 200
[Captures]
csrf: jsonpath "$.csrf_token"

POST {{base_url}}/v1/rss-feed-link/import/opml
X-Alt-Backend-Token: {{jwt}}
X-CSRF-Token: {{csrf}}
[MultipartFormData]
file: file,e2e/fixtures/alt-backend/sample-feeds.opml; application/xml
HTTP 200
[Asserts]
jsonpath "$" exists
