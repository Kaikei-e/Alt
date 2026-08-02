package datahubapi

import (
	"net/http"
	"strings"
)

// LegacyNamespacePrefix is the wire path prefix DataHubService answered on
// before ADR-000955 moved the contract to the services.* east-west root.
const LegacyNamespacePrefix = "/alt.datahub.v1.DataHubService/"

const currentNamespacePrefix = "/services.datahub.v1.DataHubService/"

// LegacyNamespaceAlias serves the retired alt.datahub.v1.DataHubService wire
// path by rewriting it onto services.datahub.v1.DataHubService. The two names
// are byte-identical contracts — ADR-000955 changed only the proto package
// root — so the same handlers answer both.
//
// This is a transitional shim, not a second door: consumers deployed before
// the rename still speak the old name during the rollout, and the pact
// broker's deployed-version selector keeps their old pacts in the provider
// verification matrix until each renamed consumer is recorded as deployed.
// Without the alias the release pipeline deadlocks — verification fails
// against the deployed pacts, and record-deployment only happens after a
// successful deploy (the services.backend.v1 shim in the provider contract
// tests exists for the same reason).
//
// Remove once every deployed DataHubService consumer (alt-backend,
// alt-harvester, pre-processor, search-indexer, tag-generator, recap-worker,
// rag-orchestrator) publishes pacts on services.datahub.v1, and restore the
// retired-namespace fence in e2e/hurl/alt-data-hub at the same time.
func LegacyNamespaceAlias(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if rest, ok := strings.CutPrefix(r.URL.Path, LegacyNamespacePrefix); ok {
			aliased := r.Clone(r.Context())
			aliased.URL.Path = currentNamespacePrefix + rest
			aliased.URL.RawPath = ""
			next.ServeHTTP(w, aliased)
			return
		}
		next.ServeHTTP(w, r)
	})
}
