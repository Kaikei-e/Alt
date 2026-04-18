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
#
# timeoutMs is 30 s rather than the CI smoke-test rule of thumb (5–10 s):
# mq-hub waits that long for tag-generator's consumer to pick up the
# TagGenerationRequested event from alt:events:tags, run inference, and
# XADD the reply. Scenarios 02/03 already warmed SBERT, so the actual
# inference is sub-second — the headroom is for the consumer's XREAD
# block cycle (~1 s) plus transient Redis Streams latency on shared CI.
#
# The outer retry covers the narrow window where scenario 02's last
# retry has just seen 200 from /extract-tags (so `_background_tag_service`
# is non-None) but the `alt:events:tags` consumer thread has not yet
# reached XGROUP CREATE. 3 attempts spaced 3 s apart costs at most 9 s
# of pre-first-success waiting on a warm path.
POST {{mqhub_base_url}}/services.mqhub.v1.MQHubService/GenerateTagsForArticle
Content-Type: application/json
Connect-Protocol-Version: 1
[Options]
retry: 3
retry-interval: 3000
{
  "articleId": "hurl-e2e-tg-{{run_id}}",
  "title": "Redis Streams round trip",
  "content": "This article exercises the full tag generation flow. Keywords: machine learning, Redis, Python, FastAPI, natural language processing.",
  "feedId": "hurl-e2e-feed",
  "timeoutMs": 30000
}
HTTP 200
[Asserts]
jsonpath "$.success" == true
jsonpath "$.articleId" == "hurl-e2e-tg-{{run_id}}"
jsonpath "$.tags" isCollection
jsonpath "$.inferenceMs" isNumber
jsonpath "$.inferenceMs" > 0
