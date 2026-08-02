# The operator listener on :9110 — health and metrics, plaintext, never
# published to the host.
#
# "mTLS only" is a claim about the *data plane*, not about every socket the
# process opens. Prometheus scrapes alt-data-hub over alt-network
# (observability/prometheus/prometheus.yml, job `alt-data-hub`), and giving
# it a client certificate would mean adding a monitoring identity to
# MTLS_ALLOWED_PEERS — diluting the data plane's allowlist to buy a
# gauge. So the operator surface is plaintext and carries nothing worth
# authenticating.
#
# The bargain only holds if that surface stays empty of everything else,
# which is what the 404s below are for. run.sh separately proves the ports
# that must not exist at all (:9000, :9101, and :9102 from outside the
# container — the internal listener binds loopback inside the netns).
#
# These entries are plaintext HTTP; the --cert / --key flags run.sh passes
# for the mTLS suite are simply unused here.

GET {{ops_url}}/health
HTTP 200
[Asserts]
jsonpath "$.status" == "healthy"
jsonpath "$.service" == "alt-data-hub"

GET {{ops_url}}/metrics
HTTP 200
[Asserts]
header "Content-Type" contains "text/plain"
body contains "# HELP"

# --- and nothing else -----------------------------------------------------
#
# The data plane must not be reachable without a certificate by walking in
# through the monitoring door.

POST {{ops_url}}/services.datahub.v1.DataHubService/GetLatestArticleTimestamp
Content-Type: application/json
{}
HTTP 404

POST {{ops_url}}/services.datahub.v1.DataHubService/CreateArticle
Content-Type: application/json
{}
HTTP 404

# ListRecentArticles and GetSystemUser were REST routes under /v1/internal
# until ADR-000954 D6 folded them into DataHubService. Probed by their current
# names: a 404 for a path this listener never served proves less than a 404 for
# the path the capability actually lives at today.
POST {{ops_url}}/services.datahub.v1.DataHubService/ListRecentArticles
Content-Type: application/json
{}
HTTP 404

POST {{ops_url}}/services.datahub.v1.DataHubService/GetSystemUser
Content-Type: application/json
{}
HTTP 404

GET {{ops_url}}/v1/health
HTTP 404
