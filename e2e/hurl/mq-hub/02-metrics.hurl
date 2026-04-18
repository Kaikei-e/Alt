# Prometheus /metrics — assert the endpoint speaks text/plain and exposes
# the mqhub-specific metric families. CounterVec/HistogramVec samples are
# registered lazily (only after the first WithLabelValues call), so we
# warm the publish counter with a no-op-ish publish before reading /metrics.
POST {{base_url}}/services.mqhub.v1.MQHubService/Publish
Content-Type: application/json
Connect-Protocol-Version: 1
file,e2e/fixtures/mq-hub/event-article-created.json;
HTTP 200

GET {{base_url}}/metrics
HTTP 200
[Asserts]
header "Content-Type" contains "text/plain"
body contains "mqhub_redis_connection_status"
body contains "mqhub_publish_total"
body contains "mqhub_publish_duration_seconds"
