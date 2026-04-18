# Publish with the `event` field omitted entirely. Handler short-circuits
# before gateway: mq-hub/app/connect/v1/mqhub/handler.go:38 returns
# connect.CodeInvalidArgument → HTTP 400 in the Connect protocol.
POST {{base_url}}/services.mqhub.v1.MQHubService/Publish
Content-Type: application/json
Connect-Protocol-Version: 1
file,e2e/fixtures/mq-hub/event-no-event-field.json;
HTTP 400
[Asserts]
jsonpath "$.code" == "invalid_argument"
