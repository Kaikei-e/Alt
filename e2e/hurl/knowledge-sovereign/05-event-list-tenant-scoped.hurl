# ListKnowledgeEvents with tenant_id + user_id must return our event
# (03) and nothing else: the run uses freshly-generated UUIDs so no
# other events in the DB can match this (tenant, user) pair.
#
# after_seq: 0 lists from the beginning. int64 in proto3-JSON accepts
# either string or number on input, but we send as number which the
# Connect codec decodes identically.
POST {{base_url}}/services.sovereign.v1.KnowledgeSovereignService/ListKnowledgeEvents
Content-Type: application/json
Connect-Protocol-Version: 1
{
  "afterSeq": 0,
  "limit": 10,
  "tenantId": "{{tenant_id}}",
  "userId": "{{user_id}}"
}
HTTP 200
[Asserts]
jsonpath "$.events" count == 1
jsonpath "$.events[0].eventId" == "{{event_id}}"
jsonpath "$.events[0].eventType" == "ArticleCreated"
jsonpath "$.events[0].tenantId" == "{{tenant_id}}"
jsonpath "$.events[0].userId" == "{{user_id}}"
jsonpath "$.events[0].dedupeKey" == "hurl-e2e-{{run_id}}-ev1"
