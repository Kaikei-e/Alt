# alt.datahub.v1.DataHubService answers on alt-data-hub over mTLS.
#
# This is the new namespace from ADR-000954 D7. It sits alongside
# 01-backend-internal-service.hurl rather than replacing it: during Wave 2 the
# provider serves both names, and the file that would prove the migration is
# finished — the one asserting 404 on the legacy path — cannot be written
# until Wave 2-C, because until then a peer that has not been moved yet is
# still calling it. When that wave lands, 01 flips from "must answer" to "must
# 404" and this file is what remains.
#
# The procedures below are the same handler reached by a different path;
# what is under test here is the mount, the TLS boundary and the request
# envelope, exactly as in 01. Response bodies are not field-pinned for the
# same reason given there — Connect's stock protojson codec omits zero-valued
# scalars, so asserting on them would encode a codec detail. Field shapes
# belong to the Pact consumer contracts.
#
# Caller identity is run.sh's `pre-processor` leaf; the peer certificate is
# the credential on this surface.

POST {{data_hub_url}}/alt.datahub.v1.DataHubService/GetLatestArticleTimestamp
Content-Type: application/json
{}
HTTP 200
[Asserts]
header "Content-Type" contains "application/json"

POST {{data_hub_url}}/alt.datahub.v1.DataHubService/ListArticlesWithTags
Content-Type: application/json
{
  "limit": 10
}
HTTP 200

POST {{data_hub_url}}/alt.datahub.v1.DataHubService/ListArticlesWithTagsForward
Content-Type: application/json
{
  "incrementalMark": "2020-01-01T00:00:00Z",
  "limit": 10
}
HTTP 200

POST {{data_hub_url}}/alt.datahub.v1.DataHubService/ListDeletedArticles
Content-Type: application/json
{
  "limit": 10
}
HTTP 200

# The write-path probe, matching 01's. The staging slice boots with an empty
# `articles` table, so this must answer 200 with exists absent/false.
POST {{data_hub_url}}/alt.datahub.v1.DataHubService/CheckArticleExists
Content-Type: application/json
{
  "url": "https://stub.invalid/alt-backend/e2e/datahub-{{run_id}}",
  "feedId": "00000000-0000-0000-0000-000000000001"
}
HTTP 200

POST {{data_hub_url}}/alt.datahub.v1.DataHubService/ListRecapArticles
Content-Type: application/json
{}
HTTP *
[Asserts]
status != 404

# ── ADR-000954 D6: the two /v1/internal REST routes as Connect procedures ──
#
# These have no counterpart in 01 — they are the REST group folded into the
# capability surface so that Wave 2-C can delete it rather than move it.
# 02-internal-rest-routes.hurl still asserts the REST originals answer, and
# the pair is the migration's overlap window: both protocols work, so a peer
# can move on its own schedule.

# rag-orchestrator's reader. Empty `articles` table in the staging slice, so
# the assertion is on the envelope. The RPC has no `limit=0 means unlimited`
# ambiguity that the REST query string had — the field is optional, so
# omitting it here asks for the server default.
POST {{data_hub_url}}/alt.datahub.v1.DataHubService/ListRecentArticles
Content-Type: application/json
{}
HTTP 200
[Asserts]
header "Content-Type" contains "application/json"

POST {{data_hub_url}}/alt.datahub.v1.DataHubService/ListRecentArticles
Content-Type: application/json
{
  "withinHours": 24,
  "limit": 10
}
HTTP 200

# An explicit zero limit is a legitimate request meaning "time window only",
# and it must not be confused with the field being absent. If proto3 implicit
# presence had been used for this field the server could not tell the two
# apart; a 200 here is what says it can.
POST {{data_hub_url}}/alt.datahub.v1.DataHubService/ListRecentArticles
Content-Type: application/json
{
  "limit": 0
}
HTTP 200

# Bad input must reach the handler's validation rather than the router, so
# that a rejection proves the mount as firmly as an acceptance does. Connect
# maps InvalidArgument to HTTP 400.
POST {{data_hub_url}}/alt.datahub.v1.DataHubService/ListRecentArticles
Content-Type: application/json
{
  "withinHours": 0
}
HTTP 400
[Asserts]
jsonpath "$.code" == "invalid_argument"

# The Kratos-backed system-user lookup. Status is not pinned to 200 for the
# same reason as in 02: the staging slice answers AUTH_HUB_URL from
# alt-backend-deps-stub, so a 500 here is a stub-fidelity question rather
# than a topology one. What this suite owns is that the procedure exists.
POST {{data_hub_url}}/alt.datahub.v1.DataHubService/GetSystemUser
Content-Type: application/json
{}
HTTP *
[Asserts]
status != 404
