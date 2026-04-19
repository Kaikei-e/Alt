# A user who has never created a conversation must see an empty list
# with no pagination token. test-empty-user-id.txt is held disjoint from
# the seeded user so this stays deterministic across reruns of the suite.
#
# Connect's protojson encoder omits empty repeated fields by default, so
# the wire response here is literally `{}` (no "conversations" key). The
# assertion shape matches that: both `conversations` and `nextPageToken`
# must be absent, and the body must equal the empty-object canonical form.
POST {{connect_url}}/alt.augur.v2.AugurService/ListConversations
Content-Type: application/json
Connect-Protocol-Version: 1
X-Alt-User-Id: {{empty_user_id}}
{}
HTTP 200
[Asserts]
jsonpath "$.conversations" not exists
jsonpath "$.nextPageToken" not exists
body == "{}"
