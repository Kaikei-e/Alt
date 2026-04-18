# End-to-end Redis Streams round trip — Hurl cannot XREAD directly, so
# we drive the round trip through mq-hub's synchronous wrapper RPC.
#
# Flow:
#   1. mq-hub /GenerateTagsForArticle publishes a TagGenerationRequested
#      event to alt:events:tags with reply_to=alt:replies:tags:<corrId>.
#   2. tag-generator's Redis Streams consumer (started in FastAPI
#      lifespan) picks it up, runs inline ML inference, and XADDs a
#      TagGenerationCompleted event to the reply stream.
#   3. mq-hub's usecase XREADs the reply and returns it in the HTTP
#      response body.
#
# The mq-hub suite already pins the timeout variant (no consumer) in
# e2e/hurl/mq-hub/14-generate-tags-timeout.hurl; this scenario is the
# matching happy path.
#
# Body is inlined (not loaded via `file,…;`) because Hurl 7.1 does not
# interpolate `{{run_id}}` inside file-loaded bodies. Response fields
# are proto3-JSON camelCase (this is Connect-RPC, not FastAPI).
POST {{mqhub_base_url}}/services.mqhub.v1.MQHubService/GenerateTagsForArticle
Content-Type: application/json
Connect-Protocol-Version: 1
{
  "articleId": "hurl-e2e-tg-{{run_id}}",
  "title": "Redis Streams round trip",
  "content": "This article exercises the full tag generation flow. Keywords: machine learning, Redis, Python, FastAPI, natural language processing.",
  "feedId": "hurl-e2e-feed",
  "timeoutMs": 15000
}
HTTP 200
[Asserts]
jsonpath "$.success" == true
jsonpath "$.articleId" == "hurl-e2e-tg-{{run_id}}"
jsonpath "$.tags" isCollection
jsonpath "$.inferenceMs" isNumber
jsonpath "$.inferenceMs" > 0
