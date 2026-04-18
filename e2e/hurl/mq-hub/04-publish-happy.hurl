# Single-event Publish happy path. Asserts the Redis Stream message_id
# format (`<ms>-<seq>`) and captures it for audit logging.
POST {{base_url}}/services.mqhub.v1.MQHubService/Publish
Content-Type: application/json
Connect-Protocol-Version: 1
file,e2e/fixtures/mq-hub/event-article-created.json;
HTTP 200
[Captures]
message_id: jsonpath "$.messageId"
[Asserts]
jsonpath "$.success" == true
jsonpath "$.messageId" matches "^[0-9]+-[0-9]+$"
