# PublishBatch with > MAX_BATCH_SIZE events (1001, default limit 1000).
# publish_usecase.go:103 rejects with ErrBatchTooLarge before touching Redis.
# Fixture is generated at run time by run.sh (see gen-batch-oversize.py).
POST {{base_url}}/services.mqhub.v1.MQHubService/PublishBatch
Content-Type: application/json
Connect-Protocol-Version: 1
file,e2e/fixtures/mq-hub/batch-oversize.json;
HTTP *
[Asserts]
status >= 400
status < 600
jsonpath "$.code" exists
jsonpath "$.message" contains "batch size exceeds"
