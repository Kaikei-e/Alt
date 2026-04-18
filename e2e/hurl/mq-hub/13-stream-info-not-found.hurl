# GetStreamInfo on a stream that has never been written to. Redis XINFO
# STREAM returns "no such key" → driver surfaces the error and the handler
# propagates it (handler.go:112-116). Contract: non-2xx Connect error.
POST {{base_url}}/services.mqhub.v1.MQHubService/GetStreamInfo
Content-Type: application/json
Connect-Protocol-Version: 1
{
  "stream": "alt:events:hurl-nonexistent-{{run_id}}"
}
HTTP *
[Asserts]
status >= 400
status < 600
jsonpath "$.code" exists
