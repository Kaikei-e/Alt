# GetStreamInfo against alt:events:articles after earlier files have
# published events and created a consumer group.
#
# Proto3 JSON note: int64 fields (length, radixTreeKeys, radixTreeNodes)
# are serialized as JSON *strings* to preserve precision; that's why we
# use isString + numeric regex rather than isNumber / >= 1.
POST {{base_url}}/services.mqhub.v1.MQHubService/GetStreamInfo
Content-Type: application/json
Connect-Protocol-Version: 1
{
  "stream": "alt:events:articles"
}
HTTP 200
[Asserts]
jsonpath "$.length" isString
jsonpath "$.length" matches "^[1-9][0-9]*$"
jsonpath "$.firstEntryId" isString
jsonpath "$.firstEntryId" matches "^[0-9]+-[0-9]+$"
jsonpath "$.lastEntryId" isString
jsonpath "$.lastEntryId" matches "^[0-9]+-[0-9]+$"
jsonpath "$.groups" isCollection
jsonpath "$.groups[*].name" includes "hurl-e2e-cg-{{run_id}}"
