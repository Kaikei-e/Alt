# Topology contract for the alt-backend / alt-harvester / alt-data-hub split,
# from the user-facing binary's side.
#
# Two listeners face users, and they must answer exactly what the monolith
# answered:
#
#   :9000   user-facing REST
#   :9101   user-facing Connect
#
# Nothing that authenticates a *service* rather than a user may be reachable
# on either. The data plane and the /v1/internal/* REST routes move out of
# this binary entirely, to alt-data-hub behind mTLS
# (di/datahub/container.go), where they are served as
# `alt.datahub.v1.DataHubService` (ADR-000954 Wave 2-C retired the
# `services.backend.v1.BackendInternalService` name outright). The admin
# Connect services do not move — they
# stay on this binary's loopback-bound operator listener — so for them the
# claim is narrower but just as load-bearing: not on a port the public NIC
# can reach.
#
# Every negative below asserts 404, never 401/403. A 401 would mean the
# routes are still registered on the user-facing mux and only a middleware
# stands between the public NIC and them; 404 is the only status that says
# "this surface is not here".
#
# Why the CSRF token is captured below: routes.go installs CSRFMiddleware
# globally (e.Use), and its default branch protects every unexempted
# POST/PUT/PATCH/DELETE. Without a token, *any* POST to :9000 answers 403
# before Echo's router ever runs — which would make this file pass for the
# wrong reason (403 "blocked by middleware" is not 404 "route absent"). We
# hand the middleware a valid token so the request reaches routing and the
# 404 is a statement about the route table.
#
# Coverage note: the mirror-image assertions — that these same surfaces DO
# answer on alt-data-hub over mTLS — live in e2e/hurl/alt-data-hub/.

# --- wire compatibility: the user-facing surface is unchanged -------------
#
# 01-health-csrf.hurl already smoke-tests /v1/health as a public endpoint.
# The duplication here is intentional and has a different lifetime: this
# assertion is the "the split did not move the user's API" clause, and it
# must survive even if the public smoke file is ever restructured.

GET {{base_url}}/v1/health
HTTP 200
[Asserts]
jsonpath "$.status" == "healthy"
jsonpath "$.database" == "connected"

GET {{base_url}}/v1/csrf-token
HTTP 200
[Captures]
csrf: jsonpath "$.csrf_token"

# The Connect listener still serves the browser-facing services. A
# registered procedure called without a user JWT answers 401 (AuthInterceptor
# ran, so the handler is mounted); this is the control that gives the 404
# assertions below their meaning. If an unregistered path and a registered
# one both answered 404, this file would prove nothing.
POST {{connect_url}}/alt.knowledge_trail.v1.KnowledgeTrailService/GetTrail
Content-Type: application/json
{}
HTTP 401

POST {{connect_url}}/alt.knowledge_trail.v1.KnowledgeTrailService/GetTrail
X-Alt-Backend-Token: {{jwt}}
Content-Type: application/json
{}
HTTP *
[Asserts]
status != 404

# --- DataHubService must not exist on the user-facing Connect port ---------
#
# One procedure per shape of caller: pre-processor's write path, the
# search-indexer read path, and the recap-worker path that the recap
# pipeline stub still mounts (e2e/stubs/recap-pipeline-stub/main.py).
# Their post-split home is asserted positively in
# ../alt-data-hub/04-datahub-service.hurl.

POST {{connect_url}}/alt.datahub.v1.DataHubService/CreateArticle
X-Alt-Backend-Token: {{jwt}}
Content-Type: application/json
{}
HTTP 404

POST {{connect_url}}/alt.datahub.v1.DataHubService/ListArticlesWithTags
X-Alt-Backend-Token: {{jwt}}
Content-Type: application/json
{}
HTTP 404

POST {{connect_url}}/alt.datahub.v1.DataHubService/ListRecapArticles
X-Alt-Backend-Token: {{jwt}}
Content-Type: application/json
{}
HTTP 404

# The retired name must stay dead too. Wave 2-C deleted
# `services.backend.v1` from the proto tree, so this is a regression fence
# against anyone re-adding a compatibility shim on the browser-facing mux.
# ../alt-data-hub/01-retired-connect-namespace-absent.hurl asserts the same
# for the data plane's own listener.

POST {{connect_url}}/services.backend.v1.BackendInternalService/CreateArticle
X-Alt-Backend-Token: {{jwt}}
Content-Type: application/json
{}
HTTP 404

# The admin Connect surface stays with this binary, on the loopback-bound
# operator listener that 72-admin-emit-article-url-backfill.hurl reaches via
# {{internal_url}}. Neither service authenticates its caller, so the bind
# address is the entire access control — which is precisely why it must not
# also answer on a port that faces the browser.
#
# AdminMonitorService is deliberately not probed: SetupInternalConnect
# Handlers registers it only when config.AdminMonitor.Enabled, which staging
# leaves false, so a 404 here would be the config talking rather than the
# topology.
POST {{connect_url}}/alt.knowledge_home.v1.KnowledgeHomeAdminService/EmitArticleUrlBackfill
X-Alt-Backend-Token: {{jwt}}
Content-Type: application/json
{}
HTTP 404

POST {{connect_url}}/alt.knowledge_home.v1.KnowledgeHomeAdminService/GetProjectionHealth
X-Alt-Backend-Token: {{jwt}}
Content-Type: application/json
{}
HTTP 404

# --- DataHubService must not exist on the REST port either -----------------
#
# :9000 is an Echo router, not a Connect mux, so a Connect path here can only
# ever be an unmatched route. Asserting it anyway pins the negative: a future
# "just proxy the internal RPCs through the REST server" shortcut breaks here.

POST {{base_url}}/alt.datahub.v1.DataHubService/CreateArticle
X-Alt-Backend-Token: {{jwt}}
X-CSRF-Token: {{csrf}}
Content-Type: application/json
{}
HTTP 404

# --- /v1/internal/* REST must not exist on the user-facing ports -----------
#
# Both handlers answer system-level queries with no tenant predicate and
# carry no auth middleware of their own (see RegisterInternalRoutes), so
# reachability is the entire control. GET is the registered verb, so these
# skip CSRF and land straight on the router.

GET {{base_url}}/v1/internal/system-user
HTTP 404

GET {{base_url}}/v1/internal/system-user
X-Alt-Backend-Token: {{jwt}}
HTTP 404

GET {{base_url}}/v1/internal/articles/recent
HTTP 404

GET {{base_url}}/v1/internal/articles/recent?within_hours=24&limit=10
X-Alt-Backend-Token: {{jwt}}
HTTP 404

# --- the operator surfaces are not on the user-facing ports ---------------
#
# /metrics moved to the shared operator listener on :9110 so all three split
# binaries expose the same shape, and so alt-data-hub can be scraped without
# handing Prometheus a client certificate
# (observability/prometheus/prometheus.yml). 01-health-csrf.hurl asserts the
# positive on {{ops_url}}; this is the other half — it is no longer served
# from the browser-facing listener.
GET {{base_url}}/metrics
HTTP 404

# The operator listener's own /health must not leak onto :9000 either. The
# public health route is /v1/health and it is asserted above.
GET {{base_url}}/health
HTTP 404
