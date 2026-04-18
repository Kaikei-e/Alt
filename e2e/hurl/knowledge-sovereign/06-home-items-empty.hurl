# GetKnowledgeHomeItems for a fresh user returns an empty projection.
# Phase 1 scope validates the structure contract without requiring
# projector wiring — the raw event from 03 has not been projected into
# knowledge_home_items. proto3-JSON omits default scalars and empty
# repeated fields, so a fresh-user response is literally `{}`.
POST {{base_url}}/services.sovereign.v1.KnowledgeSovereignService/GetKnowledgeHomeItems
Content-Type: application/json
Connect-Protocol-Version: 1
{
  "userId": "{{user_id}}",
  "limit": 10
}
HTTP 200
[Asserts]
jsonpath "$.items" not exists
jsonpath "$.hasMore" not exists
jsonpath "$.nextCursor" not exists
