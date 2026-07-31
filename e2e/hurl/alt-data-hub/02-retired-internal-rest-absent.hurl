# /v1/internal/* no longer answers anywhere.
#
# The two routes it carried were not withdrawn, they were converted: ADR-000954
# D6 folded GET /v1/internal/system-user and GET /v1/internal/articles/recent
# into DataHubService.GetSystemUser and DataHubService.ListRecentArticles, and
# 04-datahub-service.hurl asserts both answer. This file is the other half of
# that claim — the capability moved rather than being duplicated.
#
# Keeping the REST pair alongside the RPCs would have been the tempting
# compromise, and it is the one worth a failing test. These two handlers took
# no tenant argument and carried no auth middleware; the listener was their
# entire access control. Two doors onto that, with only one of them covered by
# a consumer pact, is how a route survives past the last caller that needs it.
#
# The version this replaced asserted a 400 for `?within_hours=0`, to prove the
# request reached the handler's validation rather than stopping at the router.
# That distinction is gone with the handler: the router does not read query
# strings, so a 404 with the parameter says nothing a 404 without it does not.
# The query is kept on one entry only so the request is shaped like the one
# rag-orchestrator used to send.

GET {{data_hub_url}}/v1/internal/articles/recent
HTTP 404

GET {{data_hub_url}}/v1/internal/articles/recent?within_hours=24&limit=10
HTTP 404

GET {{data_hub_url}}/v1/internal/system-user
HTTP 404

# The whole prefix, not only the two routes that lived under it. A router that
# still recognised /v1/internal and merely had no handlers registered would be
# one DI line away from serving it again.
GET {{data_hub_url}}/v1/internal
HTTP 404
