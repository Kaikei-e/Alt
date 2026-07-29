//go:build contract

// Package contract contains provider verification tests for
// knowledge-sovereign. It is the durable-state owner for every
// consumer that writes knowledge mutations, so Pact verification
// here is the gate that prevents wire-format drift from shipping.
//
// Consumers verified here:
//   - alt-backend → ApplyProjectionMutation / ApplyRecallMutation /
//     ApplyCurationMutation (Connect-RPC, JSON wire format)
//
// Verification runs against the production Connect-RPC and admin REST
// handlers with a fixture repository behind them (provider_server.go); it
// does not spin up Postgres. The handlers, not a hand-written stub, decide
// the wire shape.
package contract

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/pact-foundation/pact-go/v2/models"
	"github.com/pact-foundation/pact-go/v2/provider"
	"github.com/stretchr/testify/require"
)

const (
	providerName          = "knowledge-sovereign"
	altBackendPactFile    = "../../../../alt-backend/pacts/alt-backend-knowledge-sovereign.json"
	altBackendPactAtRoot  = "../../../../pacts/alt-backend-knowledge-sovereign.json"
	altctlPactFile        = "../../../../pacts/altctl-knowledge-sovereign.json"
	altctlPactAtAlt       = "../../../../altctl/pacts/altctl-knowledge-sovereign.json"
	ragOrchPactFile       = "../../../../rag-orchestrator/pacts/rag-orchestrator-knowledge-sovereign.json"
	ragOrchPactAtRoot     = "../../../../pacts/rag-orchestrator-knowledge-sovereign.json"
	recapWorkerPactFile   = "../../../../recap-worker/pacts/recap-worker-knowledge-sovereign.json"
	recapWorkerPactAtRoot = "../../../../pacts/recap-worker-knowledge-sovereign.json"
)

// resolvePactFile returns the first existing path among the
// candidates, or "" if none exists. Consumer tests write to
// different locations depending on which module generated them.
func resolvePactFile(candidates ...string) string {
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

func TestVerifyAltBackendConsumerContract(t *testing.T) {
	brokerURL := os.Getenv("PACT_BROKER_BASE_URL")
	localPact := resolvePactFile(altBackendPactFile, altBackendPactAtRoot)

	if brokerURL == "" && localPact == "" {
		t.Skipf("No Broker URL set and no local pact file found at %s or %s. "+
			"Run alt-backend consumer tests first: "+
			"cd alt-backend/app && CGO_ENABLED=1 go test -tags=contract ./driver/sovereign_client/contract/ -v",
			altBackendPactFile, altBackendPactAtRoot)
	}

	repo := &fakeRepo{}
	port := startProviderServer(t, repo)

	verifyRequest := provider.VerifyRequest{
		Provider:        providerName,
		ProviderBaseURL: fmt.Sprintf("http://127.0.0.1:%d", port),
		StateHandlers: models.StateHandlers{
			"the projection mutation upsert_home_item is accepted": func(setup bool, s models.ProviderState) (models.ProviderStateResponse, error) {
				repo.rejectMutation = false
				return nil, nil
			},
			"a user with at least one footprint exists": func(setup bool, s models.ProviderState) (models.ProviderStateResponse, error) {
				return nil, nil
			},
			"a user with footprints across two articles, one matching the search filter": func(setup bool, s models.ProviderState) (models.ProviderStateResponse, error) {
				return nil, nil
			},
			"a user has an open branch anchored on the just-read item": func(setup bool, s models.ProviderState) (models.ProviderStateResponse, error) {
				return nil, nil
			},
			"sovereign accepts trail.branch_resolved.v1 events carrying an optional dismiss reason": func(setup bool, s models.ProviderState) (models.ProviderStateResponse, error) {
				return nil, nil
			},
			"the projection mutation is rejected with an error": func(setup bool, s models.ProviderState) (models.ProviderStateResponse, error) {
				repo.rejectMutation = true
				return nil, nil
			},
			"the recall mutation snooze_candidate is accepted": func(setup bool, s models.ProviderState) (models.ProviderStateResponse, error) {
				repo.rejectMutation = false
				return nil, nil
			},
			"the curation mutation dismiss_curation is accepted": func(setup bool, s models.ProviderState) (models.ProviderStateResponse, error) {
				repo.rejectMutation = false
				return nil, nil
			},
			// Knowledge Loop append states (ADR-000840). The handlers
			// don't mutate stub state — the wire contract for
			// AppendKnowledgeEvent is identical for all three event_types,
			// the differentiation happens in the projector. Pact-go still
			// requires the state name to be registered or it fails the
			// interaction with "no setup handler".
			"sovereign accepts append-only Loop transition events": func(setup bool, s models.ProviderState) (models.ProviderStateResponse, error) {
				return nil, nil
			},
			"sovereign accepts Deferred Loop events with same-stage transitions": func(setup bool, s models.ProviderState) (models.ProviderStateResponse, error) {
				return nil, nil
			},
			"sovereign accepts Reviewed Loop events with trigger-based actions": func(setup bool, s models.ProviderState) (models.ProviderStateResponse, error) {
				return nil, nil
			},
			"sovereign accepts Act-stage Loop events without inferring HomeItemOpened": func(setup bool, s models.ProviderState) (models.ProviderStateResponse, error) {
				return nil, nil
			},
		},
	}

	if brokerURL != "" {
		verifyRequest.BrokerURL = brokerURL
		verifyRequest.BrokerUsername = os.Getenv("PACT_BROKER_USERNAME")
		verifyRequest.BrokerPassword = os.Getenv("PACT_BROKER_PASSWORD")
		verifyRequest.ConsumerVersionSelectors = []provider.Selector{
			&provider.ConsumerVersionSelector{Consumer: "alt-backend", MainBranch: true},
			&provider.ConsumerVersionSelector{Consumer: "alt-backend", DeployedOrReleased: true},
		}
		if ver := os.Getenv("PACT_PROVIDER_VERSION"); ver != "" {
			verifyRequest.ProviderVersion = ver
		}
		if branch := os.Getenv("PACT_PROVIDER_BRANCH"); branch != "" {
			verifyRequest.ProviderBranch = branch
		}
		verifyRequest.PublishVerificationResults = os.Getenv("PACT_PROVIDER_VERSION") != ""
		if os.Getenv("PACT_DISABLE_PENDING") != "true" {
			verifyRequest.EnablePending = true
		}
		if since := os.Getenv("PACT_INCLUDE_WIP_SINCE"); since != "" {
			if ts, err := time.Parse(time.RFC3339, since); err == nil {
				verifyRequest.IncludeWIPPactsSince = &ts
			}
		}
	} else {
		verifyRequest.PactFiles = []string{localPact}
	}

	verifier := provider.NewVerifier()
	err := verifier.VerifyProvider(t, verifyRequest)
	require.NoError(t, err)
}

// TestVerifyRagOrchestratorConsumerContract verifies the rag-orchestrator
// consumer pact for AppendKnowledgeEvent on the augur.conversation_linked.v1
// path (Wave 4-A, ADR-000853 / ADR-000855). The provider state is identical
// to the alt-backend Loop-event states — sovereign does not interpret the
// event_type at the wire layer; the projector does. Stub server is shared
// with the alt-backend verifier.
func TestVerifyRagOrchestratorConsumerContract(t *testing.T) {
	brokerURL := os.Getenv("PACT_BROKER_BASE_URL")
	localPact := resolvePactFile(ragOrchPactFile, ragOrchPactAtRoot)

	if brokerURL == "" && localPact == "" {
		t.Skipf("No Broker URL set and no local pact file found at %s or %s. "+
			"Run rag-orchestrator consumer tests first: "+
			"cd rag-orchestrator && CGO_ENABLED=1 go test -tags=contract ./internal/adapter/contract/ -v",
			ragOrchPactFile, ragOrchPactAtRoot)
	}

	repo := &fakeRepo{}
	port := startProviderServer(t, repo)

	verifyRequest := provider.VerifyRequest{
		Provider:        providerName,
		ProviderBaseURL: fmt.Sprintf("http://127.0.0.1:%d", port),
		StateHandlers: models.StateHandlers{
			"sovereign accepts append-only Loop transition events": func(setup bool, s models.ProviderState) (models.ProviderStateResponse, error) {
				return nil, nil
			},
		},
	}

	if brokerURL != "" {
		verifyRequest.BrokerURL = brokerURL
		verifyRequest.BrokerUsername = os.Getenv("PACT_BROKER_USERNAME")
		verifyRequest.BrokerPassword = os.Getenv("PACT_BROKER_PASSWORD")
		verifyRequest.ConsumerVersionSelectors = []provider.Selector{
			&provider.ConsumerVersionSelector{Consumer: "rag-orchestrator", MainBranch: true},
			&provider.ConsumerVersionSelector{Consumer: "rag-orchestrator", DeployedOrReleased: true},
		}
		if ver := os.Getenv("PACT_PROVIDER_VERSION"); ver != "" {
			verifyRequest.ProviderVersion = ver
		}
		if branch := os.Getenv("PACT_PROVIDER_BRANCH"); branch != "" {
			verifyRequest.ProviderBranch = branch
		}
		verifyRequest.PublishVerificationResults = os.Getenv("PACT_PROVIDER_VERSION") != ""
		if os.Getenv("PACT_DISABLE_PENDING") != "true" {
			verifyRequest.EnablePending = true
		}
		if since := os.Getenv("PACT_INCLUDE_WIP_SINCE"); since != "" {
			if ts, err := time.Parse(time.RFC3339, since); err == nil {
				verifyRequest.IncludeWIPPactsSince = &ts
			}
		}
	} else {
		verifyRequest.PactFiles = []string{localPact}
	}

	verifier := provider.NewVerifier()
	err := verifier.VerifyProvider(t, verifyRequest)
	require.NoError(t, err)
}

// TestVerifyRecapWorkerConsumerContract verifies the recap-worker consumer
// pact for AppendKnowledgeEvent on the recap.topic_snapshotted.v1 path
// (Wave 4-B, ADR-000853). recap-worker is the Rust producer for the
// topic_overlap_count signal Surface Planner v2 consumes; this verifier
// holds the wire shape stable so an actor_type / event_type rename in
// the producer is caught at the pact gate rather than in production
// metric stalls.
func TestVerifyRecapWorkerConsumerContract(t *testing.T) {
	brokerURL := os.Getenv("PACT_BROKER_BASE_URL")
	localPact := resolvePactFile(recapWorkerPactFile, recapWorkerPactAtRoot)

	if brokerURL == "" && localPact == "" {
		t.Skipf("No Broker URL set and no local pact file found at %s or %s. "+
			"Run recap-worker consumer tests first: "+
			"cd recap-worker/recap-worker && cargo test contract -- --ignored",
			recapWorkerPactFile, recapWorkerPactAtRoot)
	}

	repo := &fakeRepo{}
	port := startProviderServer(t, repo)

	verifyRequest := provider.VerifyRequest{
		Provider:        providerName,
		ProviderBaseURL: fmt.Sprintf("http://127.0.0.1:%d", port),
		StateHandlers: models.StateHandlers{
			"sovereign accepts append-only Loop transition events": func(setup bool, s models.ProviderState) (models.ProviderStateResponse, error) {
				return nil, nil
			},
		},
	}

	if brokerURL != "" {
		verifyRequest.BrokerURL = brokerURL
		verifyRequest.BrokerUsername = os.Getenv("PACT_BROKER_USERNAME")
		verifyRequest.BrokerPassword = os.Getenv("PACT_BROKER_PASSWORD")
		verifyRequest.ConsumerVersionSelectors = []provider.Selector{
			&provider.ConsumerVersionSelector{Consumer: "recap-worker", MainBranch: true},
			&provider.ConsumerVersionSelector{Consumer: "recap-worker", DeployedOrReleased: true},
		}
		if ver := os.Getenv("PACT_PROVIDER_VERSION"); ver != "" {
			verifyRequest.ProviderVersion = ver
		}
		if branch := os.Getenv("PACT_PROVIDER_BRANCH"); branch != "" {
			verifyRequest.ProviderBranch = branch
		}
		verifyRequest.PublishVerificationResults = os.Getenv("PACT_PROVIDER_VERSION") != ""
		if os.Getenv("PACT_DISABLE_PENDING") != "true" {
			verifyRequest.EnablePending = true
		}
		if since := os.Getenv("PACT_INCLUDE_WIP_SINCE"); since != "" {
			if ts, err := time.Parse(time.RFC3339, since); err == nil {
				verifyRequest.IncludeWIPPactsSince = &ts
			}
		}
	} else {
		verifyRequest.PactFiles = []string{localPact}
	}

	verifier := provider.NewVerifier()
	err := verifier.VerifyProvider(t, verifyRequest)
	require.NoError(t, err)
}

func TestVerifyAltctlConsumerContract(t *testing.T) {
	brokerURL := os.Getenv("PACT_BROKER_BASE_URL")
	localPact := resolvePactFile(altctlPactFile, altctlPactAtAlt)

	if brokerURL == "" && localPact == "" {
		t.Skipf("No Broker URL set and no local pact file found at %s or %s. "+
			"Run altctl consumer tests first: "+
			"cd altctl && CGO_ENABLED=1 go test -tags=contract ./internal/sovereignclient/contract/ -v",
			altctlPactFile, altctlPactAtAlt)
	}

	repo := &fakeRepo{}
	port := startProviderServer(t, repo)

	verifyRequest := provider.VerifyRequest{
		Provider:        providerName,
		ProviderBaseURL: fmt.Sprintf("http://127.0.0.1:%d", port),
		StateHandlers: models.StateHandlers{
			"an admin operator has snapshot authority": func(setup bool, s models.ProviderState) (models.ProviderStateResponse, error) {
				return nil, nil
			},
			"at least one snapshot exists": func(setup bool, s models.ProviderState) (models.ProviderStateResponse, error) {
				return nil, nil
			},
			"retention policies are configured": func(setup bool, s models.ProviderState) (models.ProviderStateResponse, error) {
				return nil, nil
			},
			"at least one retention log entry exists": func(setup bool, s models.ProviderState) (models.ProviderStateResponse, error) {
				return nil, nil
			},
			"storage stats are available": func(setup bool, s models.ProviderState) (models.ProviderStateResponse, error) {
				return nil, nil
			},
		},
	}

	if brokerURL != "" {
		verifyRequest.BrokerURL = brokerURL
		verifyRequest.BrokerUsername = os.Getenv("PACT_BROKER_USERNAME")
		verifyRequest.BrokerPassword = os.Getenv("PACT_BROKER_PASSWORD")
		verifyRequest.ConsumerVersionSelectors = []provider.Selector{
			&provider.ConsumerVersionSelector{Consumer: "altctl", MainBranch: true},
			&provider.ConsumerVersionSelector{Consumer: "altctl", DeployedOrReleased: true},
		}
		if ver := os.Getenv("PACT_PROVIDER_VERSION"); ver != "" {
			verifyRequest.ProviderVersion = ver
		}
		if branch := os.Getenv("PACT_PROVIDER_BRANCH"); branch != "" {
			verifyRequest.ProviderBranch = branch
		}
		verifyRequest.PublishVerificationResults = os.Getenv("PACT_PROVIDER_VERSION") != ""
		if os.Getenv("PACT_DISABLE_PENDING") != "true" {
			verifyRequest.EnablePending = true
		}
		if since := os.Getenv("PACT_INCLUDE_WIP_SINCE"); since != "" {
			if ts, err := time.Parse(time.RFC3339, since); err == nil {
				verifyRequest.IncludeWIPPactsSince = &ts
			}
		}
	} else {
		verifyRequest.PactFiles = []string{localPact}
	}

	verifier := provider.NewVerifier()
	err := verifier.VerifyProvider(t, verifyRequest)
	require.NoError(t, err)
}
