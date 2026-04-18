# Connect-RPC HealthCheck over HTTP/1.1 + JSON codec.
# Request shape: proto3 JSON for services.mqhub.v1.HealthCheckRequest (empty).
# Response shape: services.mqhub.v1.HealthCheckResponse.
POST {{base_url}}/services.mqhub.v1.MQHubService/HealthCheck
Content-Type: application/json
Connect-Protocol-Version: 1
{}
HTTP 200
[Asserts]
header "Content-Type" contains "application/json"
jsonpath "$.healthy" == true
jsonpath "$.redisStatus" == "connected"
jsonpath "$.uptimeSeconds" exists
