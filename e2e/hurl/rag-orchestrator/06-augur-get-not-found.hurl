# Fetching a conversation id that doesn't exist (yet is syntactically a
# UUID) must surface NotFound, not Internal. The handler wraps a nil error
# (augur/handler.go:550-552), so Connect's JSON body may carry an empty
# message string — code is the contract, message is best-effort.
POST {{connect_url}}/alt.augur.v2.AugurService/GetConversation
Content-Type: application/json
Connect-Protocol-Version: 1
X-Alt-User-Id: {{empty_user_id}}
{
  "id": "99999999-9999-9999-9999-999999999999"
}
HTTP 404
[Asserts]
jsonpath "$.code" == "not_found"
