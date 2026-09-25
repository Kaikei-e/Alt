//go:build contract

package contract

import (
	"path/filepath"
	"testing"
)

// TestVerifyRAGOrchestratorDataHubContract verifies the RAG tool surface
// (ADR-000617) and ListRecentArticles.
//
// It names alt-data-hub as the provider because that is the pacticipant
// rag-orchestrator published against — see the package comment. The stub is
// the same one every other verification here runs against, which is the honest
// arrangement: one binary serves both names.
//
// The DataHubContract suffix is load-bearing, not decoration: scripts/pact-check.sh
// partitions this package between its "alt-backend provider" and "alt-data-hub
// provider" steps on that substring, because the step registry splits on `|`
// and so cannot hold an alternation of test names. A verification whose
// provider pacticipant is alt-data-hub must carry the suffix or it runs under
// the wrong service's leg.
func TestVerifyRAGOrchestratorDataHubContract(t *testing.T) {
	verifyConsumer(t, "rag-orchestrator", dataHubProviderName,
		filepath.Join(ragPactDir, ragOrchestratorPactFile),
		noopStates(
			"alt-data-hub has articles tagged ai",
			"alt-data-hub has tagged articles",
			"alt-data-hub has articles published in the last 24 hours",
		), true)
}

// Verified without failIfNoPactsFound, unlike its siblings: services.yaml keeps
// alt-harvester as a runtime rather than a pacticipant until wave 3, so the
// production broker holds no pact under that consumer name by design and this
// selector legitimately resolves to nothing there. The throwaway broker in the
// Alt repo's own workflow does receive the pact, so the contract is still
// replayed on every push. Restore the flag when alt-harvester is promoted.
func TestVerifyAltHarvesterDataHubContract(t *testing.T) {
	verifyConsumer(t, "alt-harvester", dataHubProviderName,
		filepath.Join(altBackendPactDir, altHarvesterDataHubPactFile),
		withStates(noopStates(
			"alt-data-hub has pending outbox events",
			"alt-data-hub has a claimed outbox event",
			"alt-data-hub has processed outbox events past retention",
			"alt-data-hub has recent articles with no og image",
			"alt-data-hub has feeds with uncached og images",
			"alt-data-hub has article heads past retention",
			"alt-data-hub has cached images past retention",
			"alt-data-hub has expired cached images",
			"alt-data-hub accepts scraping domain writes",
			"alt-data-hub has scraping domains",
			"alt-data-hub has a scraping domain",

			// Wave 3 batch 3 (catalog §2.F / §2.G / §2.H).
			"alt-data-hub has feed links",
			"alt-data-hub has pollable feed links",
			"alt-data-hub has a feed link with failures below the threshold",
			"alt-data-hub has a feed link at the failure threshold",
			"alt-data-hub accepts feed registrations",

			// The daily-entrance digest the today-entrance job enqueues.
			"alt-data-hub accepts notification enqueues",
		), backlogStates()), false)
}

// TestVerifyRecapWorkerDataHubContract verifies the notification enqueue the
// recap worker's outbox relay makes once a recap is ready.
//
// recap-worker is the third caller of EnqueueNotification and the only one
// outside this Go module, which is what makes its pact worth a file of its
// own: the other two would still compile against a renamed field.
func TestVerifyRecapWorkerDataHubContract(t *testing.T) {
	verifyConsumer(t, "recap-worker", dataHubProviderName,
		filepath.Join(pactDir, recapWorkerDataHubPactFile),
		withStates(noopStates("alt-data-hub accepts notification enqueues"), readStateStates()), true)
}
