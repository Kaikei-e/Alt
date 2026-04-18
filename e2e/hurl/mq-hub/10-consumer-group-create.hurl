# Create a fresh consumer group scoped to this run via {{run_id}}.
# Contract: success=true with a human-readable message field.
POST {{base_url}}/services.mqhub.v1.MQHubService/CreateConsumerGroup
Content-Type: application/json
Connect-Protocol-Version: 1
{
  "stream": "alt:events:articles",
  "group": "hurl-e2e-cg-{{run_id}}",
  "startId": "0"
}
HTTP 200
[Asserts]
jsonpath "$.success" == true
jsonpath "$.message" isString
