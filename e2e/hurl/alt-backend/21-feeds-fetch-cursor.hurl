# Cursor-paginated reads. All three cursor endpoints share the same
# response envelope: {data, has_more, next_cursor}. `limit` is clamped to
# [1, 100]; above 100 it is silently reduced. Invalid RFC3339 cursor
# values return 400.

GET {{base_url}}/v1/feeds/fetch/cursor?limit=20
X-Alt-Backend-Token: {{jwt}}
HTTP 200
[Asserts]
jsonpath "$.data" isCollection
jsonpath "$.has_more" isBoolean
jsonpath "$.next_cursor" exists

GET {{base_url}}/v1/feeds/fetch/viewed/cursor?limit=10
X-Alt-Backend-Token: {{jwt}}
HTTP 200
[Asserts]
jsonpath "$.data" isCollection
jsonpath "$.has_more" isBoolean

GET {{base_url}}/v1/feeds/fetch/favorites/cursor?limit=10
X-Alt-Backend-Token: {{jwt}}
HTTP 200
[Asserts]
jsonpath "$.data" isCollection

GET {{base_url}}/v1/feeds/fetch/cursor?cursor=not-rfc3339
X-Alt-Backend-Token: {{jwt}}
HTTP 400

GET {{base_url}}/v1/feeds/count/unreads
X-Alt-Backend-Token: {{jwt}}
HTTP 200
[Asserts]
jsonpath "$.count" isInteger
