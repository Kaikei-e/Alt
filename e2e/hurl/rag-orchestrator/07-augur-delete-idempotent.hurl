# DeleteConversation is idempotent: an unknown id returns 200 with an
# empty response, not NotFound. This is the handler's current contract
# (augur/handler.go:580-595) — if a future refactor tightens it to 404,
# this test flips to RED, which is the desired breadcrumb.
POST {{connect_url}}/alt.augur.v2.AugurService/DeleteConversation
Content-Type: application/json
Connect-Protocol-Version: 1
X-Alt-User-Id: {{empty_user_id}}
{
  "id": "99999999-9999-9999-9999-999999999998"
}
HTTP 200
[Asserts]
# Successful Connect unary responses carry the response message (empty {})
# as the whole body; no `code` / `message` field is present.
jsonpath "$.code" not exists
