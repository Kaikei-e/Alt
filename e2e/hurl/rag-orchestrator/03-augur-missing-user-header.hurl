# Connect unary RPC without X-Alt-User-Id must fail closed with
# Unauthenticated. The message is locked to the handler's literal string so
# a silent relaxation of the auth contract trips this test.
# See augur/handler.go:66-70 (extractUserID) and ListConversations caller
# at augur/handler.go:490-492.
POST {{connect_url}}/alt.augur.v2.AugurService/ListConversations
Content-Type: application/json
Connect-Protocol-Version: 1
{}
HTTP 401
[Asserts]
header "Content-Type" contains "application/json"
jsonpath "$.code" == "unauthenticated"
jsonpath "$.message" == "missing X-Alt-User-Id header"
