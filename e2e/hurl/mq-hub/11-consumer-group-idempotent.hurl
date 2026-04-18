# Re-create the same consumer group. The driver silently absorbs the Redis
# BUSYGROUP error (redis_driver.go:156), so the second call must also
# return success=true — consumer-group creation is effectively idempotent.
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
