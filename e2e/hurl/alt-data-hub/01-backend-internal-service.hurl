# BackendInternalService answers on alt-data-hub over mTLS.
#
# This is the positive half of the split's core contract. Its negative half
# lives in e2e/hurl/alt-backend/03-topology-internal-surface-absent.hurl,
# which asserts the same procedures 404 on the user-facing binary. Read the
# two files together: alone, either one is satisfiable by a service that
# simply serves nothing.
#
# The caller identity is the `pre-processor` leaf minted by run.sh — a name
# on alt-data-hub's MTLS_ALLOWED_PEERS list. There is no JWT here and no
# X-Alt-Backend-Token: the peer certificate *is* the credential on this
# surface, which is the whole reason the surface had to leave a listener
# that the public NIC can reach.
#
# Response bodies are not field-pinned. The internal handler is registered
# with Connect's stock protojson codec (only the admin services opt into
# EmitUnpopulatedJSONCodec), so zero-valued scalars are absent from the wire
# and asserting on them would encode a codec detail. Field-shape contracts
# for these RPCs belong to the Pact consumer tests under alt-backend/pacts/;
# what E2E owns here is "the handler is mounted on this listener, behind
# this TLS boundary, and decodes a real request envelope".

POST {{data_hub_url}}/services.backend.v1.BackendInternalService/GetLatestArticleTimestamp
Content-Type: application/json
{}
HTTP 200
[Asserts]
header "Content-Type" contains "application/json"

POST {{data_hub_url}}/services.backend.v1.BackendInternalService/ListArticlesWithTags
Content-Type: application/json
{
  "limit": 10
}
HTTP 200

POST {{data_hub_url}}/services.backend.v1.BackendInternalService/ListArticlesWithTagsForward
Content-Type: application/json
{
  "incrementalMark": "2020-01-01T00:00:00Z",
  "limit": 10
}
HTTP 200

POST {{data_hub_url}}/services.backend.v1.BackendInternalService/ListDeletedArticles
Content-Type: application/json
{
  "limit": 10
}
HTTP 200

# pre-processor's write-path probe. The staging slice boots with an empty
# `articles` table, so this must answer 200 with exists absent/false rather
# than error.
POST {{data_hub_url}}/services.backend.v1.BackendInternalService/CheckArticleExists
Content-Type: application/json
{
  "url": "https://stub.invalid/alt-backend/e2e/topology-{{run_id}}",
  "feedId": "00000000-0000-0000-0000-000000000001"
}
HTTP 200

# recap-worker's contract moved onto BackendInternalService (see the note in
# alt-backend/app/orchestrator/rest/routes.go and the recap-pipeline stub).
# It rides the same TLS boundary after the split.
POST {{data_hub_url}}/services.backend.v1.BackendInternalService/ListRecapArticles
Content-Type: application/json
{}
HTTP *
[Asserts]
status != 404
