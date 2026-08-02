# Neither services.backend.v1.BackendInternalService nor
# alt.datahub.v1.DataHubService answers anywhere.
#
# This file used to assert the opposite. Through ADR-000954 Wave 2 alt-data-hub
# served the data plane under two names — the one it inherited from when this
# code lived inside alt-backend, and alt.datahub.v1.DataHubService — so that the
# five consumers could migrate one PR at a time. Wave 2-C deleted the old proto
# once they were all across, and the README said this file would flip in the
# same commit rather than be deleted. That is what it is: the same procedures,
# same listener, same certificate, inverted assertion.
#
# ADR-000955 then retired the second name as well, moving every proto package
# under the `services.` prefix. The data plane answers as
# services.datahub.v1.DataHubService — 04-datahub-service.hurl is the positive
# half — and `alt.datahub.v1` joins the fence at the bottom of this file. It is
# newly retired rather than long gone, which is precisely when a compatibility
# shim is tempting enough to need a test standing in front of it.
#
# Deleting it instead would have left nothing that fails if the mount comes
# back. A second name for the data plane is not a harmless leftover — it is a
# second path to alt-db's owner that no consumer contract, no Pact provider
# verification and no allowlist is looking at, which is exactly how the
# pre-split listener came to answer for three audiences at once.
#
# 404 is asserted rather than "not 200". `cmd/datahub`'s router answers
# anything outside `/services.datahub.v1.` and `/health` with http.NotFound —
# the prefix is the full namespace and not the `/services.` root precisely so
# that services.backend.v1 fails it too — so a 404 here is the router
# refusing; a 501 or a Connect `unimplemented` envelope would mean the mux
# still knows the service and something else declined, which is a different
# and much less finished state.
#
# The caller identity is run.sh's `pre-processor` leaf, the same credential
# 04-datahub-service.hurl uses. Probing with a peer that would be refused at
# the handshake would make this file pass for the wrong reason.

POST {{data_hub_url}}/services.backend.v1.BackendInternalService/GetLatestArticleTimestamp
Content-Type: application/json
{}
HTTP 404

POST {{data_hub_url}}/services.backend.v1.BackendInternalService/ListArticlesWithTags
Content-Type: application/json
{
  "limit": 10
}
HTTP 404

POST {{data_hub_url}}/services.backend.v1.BackendInternalService/ListArticlesWithTagsForward
Content-Type: application/json
{
  "incrementalMark": "2020-01-01T00:00:00Z",
  "limit": 10
}
HTTP 404

POST {{data_hub_url}}/services.backend.v1.BackendInternalService/ListDeletedArticles
Content-Type: application/json
{
  "limit": 10
}
HTTP 404

# pre-processor's write path. The one worth naming separately: a stale caller
# on the old namespace must fail loudly rather than write through a door
# nobody is auditing.
POST {{data_hub_url}}/services.backend.v1.BackendInternalService/CheckArticleExists
Content-Type: application/json
{
  "url": "https://stub.invalid/alt-backend/e2e/topology-{{run_id}}",
  "feedId": "00000000-0000-0000-0000-000000000001"
}
HTTP 404

POST {{data_hub_url}}/services.backend.v1.BackendInternalService/ListRecapArticles
Content-Type: application/json
{}
HTTP 404

# --- the ADR-000955 casualty: alt.datahub.v1 --------------------------------
#
# One write procedure and one read procedure, the same two shapes of caller the
# block above fences. The whole point of renaming the package rather than
# aliasing it is that there is one wire name for alt-db's owner; a provider
# that quietly kept answering the old one would give every consumer a way to
# skip the rename and every reviewer a reason to believe it had landed.

POST {{data_hub_url}}/alt.datahub.v1.DataHubService/CreateArticle
Content-Type: application/json
{}
HTTP 404

POST {{data_hub_url}}/alt.datahub.v1.DataHubService/ListRecapArticles
Content-Type: application/json
{}
HTTP 404
