# Non-UUID X-Alt-User-Id must be rejected with Unauthenticated. Only the
# prefix is locked because the suffix carries uuid.Parse's error string,
# which Go's stdlib is free to reword between releases.
# See augur/handler.go:71-75.
POST {{connect_url}}/alt.augur.v2.AugurService/ListConversations
Content-Type: application/json
Connect-Protocol-Version: 1
X-Alt-User-Id: not-a-uuid
{}
HTTP 401
[Asserts]
jsonpath "$.code" == "unauthenticated"
jsonpath "$.message" startsWith "invalid X-Alt-User-Id header:"
