# Publish with empty event_type must fail. Validation lives in the gateway
# (mq-hub/app/gateway/stream_gateway.go:40 — event.Validate()), so the
# handler returns a bare Go error that Connect wraps to a non-2xx response.
# We don't pin the exact Connect code — only assert the request did not
# succeed and the server surfaced a validation message.
POST {{base_url}}/services.mqhub.v1.MQHubService/Publish
Content-Type: application/json
Connect-Protocol-Version: 1
file,e2e/fixtures/mq-hub/event-invalid-missing-type.json;
HTTP *
[Asserts]
status >= 400
status < 600
jsonpath "$.code" exists
jsonpath "$.message" contains "event_type"
