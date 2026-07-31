# /v1/internal/* REST answers on alt-data-hub over mTLS.
#
# RegisterInternalRoutes mounts two handlers that carry no auth middleware
# and answer system-level queries with no tenant predicate. Before the split
# their only control was "the listener is bound to loopback"; after it, the
# control is the peer certificate. This file pins that the routes survived
# the move — the mirror assertion (404 on alt-backend :9000) is in
# e2e/hurl/alt-backend/03-topology-internal-surface-absent.hurl.

# rag-orchestrator's reader. Empty `articles` table in the staging slice, so
# the assertion is on the envelope, not on rows.
GET {{data_hub_url}}/v1/internal/articles/recent
HTTP 200
[Asserts]
header "Content-Type" contains "application/json"

GET {{data_hub_url}}/v1/internal/articles/recent?within_hours=24&limit=10
HTTP 200

# Bad input still has to reach the handler's own validation rather than the
# router: a 400 here proves the route is mounted just as firmly as a 200,
# and distinguishes "handler rejected it" from "nothing listens".
GET {{data_hub_url}}/v1/internal/articles/recent?within_hours=0
HTTP 400

# The Kratos-backed system-user lookup. Status is not pinned to 200: the
# staging slice answers AUTH_HUB_URL from alt-backend-deps-stub and this
# handler's 500 branch is a stub-fidelity question, not a topology one.
# What this suite owns is that the route exists on this listener.
GET {{data_hub_url}}/v1/internal/system-user
HTTP *
[Asserts]
status != 404
