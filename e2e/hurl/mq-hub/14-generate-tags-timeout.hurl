# GenerateTagsForArticle with no tag-generator consumer running. The request
# publishes a TagGenerationRequested event to alt:events:tags and waits on
# the reply stream (alt:replies:tags:<correlation_id>) for timeout_ms=200.
# generate_tags_usecase.go:120 surfaces "timeout waiting for reply", which
# the handler returns as a Connect error. Reply stream is cleaned up by the
# defer in the usecase.
POST {{base_url}}/services.mqhub.v1.MQHubService/GenerateTagsForArticle
Content-Type: application/json
Connect-Protocol-Version: 1
file,e2e/fixtures/mq-hub/generate-tags-timeout.json;
HTTP *
[Asserts]
status >= 400
status < 600
jsonpath "$.code" exists
jsonpath "$.message" contains "timeout"
duration < 2000
