# alt-harvester serves health and metrics, and nothing else.
#
# The split gives each container one job and one reason to accept a request.
# alt-backend answers users, alt-data-hub answers services holding a
# certificate, and alt-harvester answers no one — it runs schedules. A job
# runner that also exposes an API is a job runner someone will eventually
# call synchronously, and the schedule and the request path then share a
# process, a pool and a failure mode. This file is the fence around that.
#
# The two surfaces that do exist are both operator surfaces on :9110:
# /health for the compose probe, /metrics for the `alt-harvester` scrape job
# in observability/prometheus/prometheus.yml. Neither is published to the
# host.
#
# Every other assertion is 404, never 401/403. A 401 would mean the handler
# is compiled in and registered here with only a middleware between it and
# the caller; 404 says the surface does not exist in this binary — which for
# alt-harvester is a compile-time fact (di/container_harvester.go builds no
# handler at all). Ports that must not be bound at all (:9000, :9101, :9102,
# :9443) cannot be expressed as Hurl entries — a refused connection is a run
# failure, not a passing assertion — so run.sh probes them via
# _lib/probe-transport-refused.hurl.

# --- the two surfaces that do exist ---------------------------------------

GET {{ops_url}}/health
HTTP 200
[Asserts]
jsonpath "$.status" == "healthy"
jsonpath "$.service" == "alt-harvester"
header "Content-Type" contains "application/json"

# Prometheus scrapes this. The `service: alt-harvester` label is applied by
# the scrape config, not by the exporter, so what is pinned here is only
# that the endpoint speaks the text exposition format.
GET {{ops_url}}/metrics
HTTP 200
[Asserts]
header "Content-Type" contains "text/plain"
body contains "# HELP"

# --- the user-facing API is not here --------------------------------------
#
# /v1/health in particular: it is the monolith's public health route. If it
# answers here, the harvester is running the user-facing router.

GET {{ops_url}}/v1/health
HTTP 404

GET {{ops_url}}/v1/csrf-token
HTTP 404

GET {{ops_url}}/v1/feeds/fetch/list
HTTP 404

GET {{ops_url}}/v1/rss-feed-link/list
HTTP 404

POST {{ops_url}}/alt.knowledge_trail.v1.KnowledgeTrailService/GetTrail
Content-Type: application/json
{}
HTTP 404

# --- the data plane is not here either ------------------------------------
#
# alt-harvester is a *client* of alt-data-hub's database, so the tempting
# shortcut is to let it re-register DataHubService "since the gateway is
# already linked in". That would restore exactly the
# many-surfaces-one-binary shape the split exists to remove, and it would do
# it on a plaintext port with no peer allowlist in front.

POST {{ops_url}}/alt.datahub.v1.DataHubService/CreateArticle
Content-Type: application/json
{}
HTTP 404

POST {{ops_url}}/alt.datahub.v1.DataHubService/GetLatestArticleTimestamp
Content-Type: application/json
{}
HTTP 404

# The retired name (ADR-000954 Wave 2-C deleted `services.backend.v1` from
# the proto tree) must not come back here as a compatibility shim either.

POST {{ops_url}}/services.backend.v1.BackendInternalService/CreateArticle
Content-Type: application/json
{}
HTTP 404

GET {{ops_url}}/v1/internal/system-user
HTTP 404

GET {{ops_url}}/v1/internal/articles/recent
HTTP 404

# --- and the admin surface is not here ------------------------------------
#
# The admin Connect services stay on alt-backend's loopback operator
# listener. Nothing about "the harvester already has an operator port"
# should turn into a second home for them.

POST {{ops_url}}/alt.knowledge_home.v1.KnowledgeHomeAdminService/EmitArticleUrlBackfill
Content-Type: application/json
{}
HTTP 404

POST {{ops_url}}/alt.knowledge_home.v1.KnowledgeHomeAdminService/GetProjectionHealth
Content-Type: application/json
{}
HTTP 404
