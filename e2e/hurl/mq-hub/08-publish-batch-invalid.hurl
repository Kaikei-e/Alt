# PublishBatch where one event is invalid (empty event_type at index 2).
# The gateway validates ALL events before hitting Redis
# (stream_gateway.go:58-65), so the whole batch is rejected — there is no
# partial-success path at the usecase layer. Contract: non-2xx with a
# validation message, and none of the valid events get persisted.
POST {{base_url}}/services.mqhub.v1.MQHubService/PublishBatch
Content-Type: application/json
Connect-Protocol-Version: 1
file,e2e/fixtures/mq-hub/batch-with-invalid.json;
HTTP *
[Asserts]
status >= 400
status < 600
jsonpath "$.code" exists
jsonpath "$.message" contains "event_type"
