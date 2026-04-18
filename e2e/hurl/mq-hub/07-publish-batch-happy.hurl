# PublishBatch with 3 valid events — pipeline-backed path
# (redis_driver.go:118 XAdd pipeline). All succeed, errors array is empty.
POST {{base_url}}/services.mqhub.v1.MQHubService/PublishBatch
Content-Type: application/json
Connect-Protocol-Version: 1
file,e2e/fixtures/mq-hub/batch-happy.json;
HTTP 200
[Asserts]
jsonpath "$.successCount" == 3
jsonpath "$.failureCount" not exists
jsonpath "$.messageIds" count == 3
jsonpath "$.messageIds[0]" matches "^[0-9]+-[0-9]+$"
jsonpath "$.messageIds[1]" matches "^[0-9]+-[0-9]+$"
jsonpath "$.messageIds[2]" matches "^[0-9]+-[0-9]+$"
