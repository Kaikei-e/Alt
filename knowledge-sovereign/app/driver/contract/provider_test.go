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

	// One name per consumer, read by both FilterConsumers and the
	// ConsumerVersionSelectors. Stating it twice let the two disagree, and a
	// FilterConsumers that matches nothing is not caught by
	// FailIfNoPactsFound — the Broker's pacts are counted before the consumer
	// filter runs, so the verification passes having checked nothing.
	altBackendConsumer  = "alt-backend"
	ragOrchConsumer     = "rag-orchestrator"
	recapWorkerConsumer = "recap-worker"
	altctlConsumer      = "altctl"
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

// Each verifier below sets FilterConsumers to the one consumer it declares
// state handlers for. ConsumerVersionSelectors is not enough: pact-go appends
// $PACT_URL to every VerifyRequest's pact sources (provider/verify_request.go,
// addPactUrlsFromEnvironment), and the Broker's
// contract_requiring_verification_published webhook sets PACT_URL for the whole
// job. So one webhook about the alt-backend pact makes all four verifiers in
// this package load that pact, and pact-go answers a provider state it has no
// handler for with a WARN and HTTP 200 rather than an error. The three
// verifiers that never registered "the projection mutation is rejected with an
// error" then drove the mutation through an accepting fixture and reported the
// 200/500 mismatch as a contract failure against alt-backend.
//
// FilterConsumers is enforced by the verifier itself, so it holds no matter
// which source handed it the pact.
//
// FailIfNoPactsFound accompanies it so a selector that matches nothing fails
// instead of reporting a green verification of zero interactions. The
// file-mode branch always supplies a pact — the test skips earlier when none
// exists — so the flag can only fire when the Broker genuinely holds no pact
// for this consumer, which is a result worth failing on rather than passing.

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
		Provider:           providerName,
		ProviderBaseURL:    fmt.Sprintf("http://127.0.0.1:%d", port),
		FilterConsumers:    []string{altBackendConsumer},
		FailIfNoPactsFound: true,
		StateHandlers: models.StateHandlers{
			"the projection mutation upsert_home_item is accepted": func(setup bool, s models.ProviderState) (models.ProviderStateResponse, error) {
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
			// Scoped to the one interaction that asked for it: pact-go calls
			// this handler again with setup=false on teardown, so the fixture
			// is accepting again before the next interaction is set up. The
			// neighbouring "is accepted" states used to carry a compensating
			// reset, which only worked while they happened to sort after this
			// one in the pact file.
			"the projection mutation is rejected with an error": func(setup bool, s models.ProviderState) (models.ProviderStateResponse, error) {
				repo.rejectMutation = setup
				return nil, nil
			},
			"the recall mutation snooze_candidate is accepted": func(setup bool, s models.ProviderState) (models.ProviderStateResponse, error) {
				return nil, nil
			},
			"the curation mutation dismiss_curation is accepted": func(setup bool, s models.ProviderState) (models.ProviderStateResponse, error) {
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
			&provider.ConsumerVersionSelector{Consumer: altBackendConsumer, MainBranch: true},
			&provider.ConsumerVersionSelector{Consumer: altBackendConsumer, DeployedOrReleased: true},
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
		Provider:           providerName,
		ProviderBaseURL:    fmt.Sprintf("http://127.0.0.1:%d", port),
		FilterConsumers:    []string{ragOrchConsumer},
		FailIfNoPactsFound: true,
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
			&provider.ConsumerVersionSelector{Consumer: ragOrchConsumer, MainBranch: true},
			&provider.ConsumerVersionSelector{Consumer: ragOrchConsumer, DeployedOrReleased: true},
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
		Provider:           providerName,
		ProviderBaseURL:    fmt.Sprintf("http://127.0.0.1:%d", port),
		FilterConsumers:    []string{recapWorkerConsumer},
		FailIfNoPactsFound: true,
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
			&provider.ConsumerVersionSelector{Consumer: recapWorkerConsumer, MainBranch: true},
			&provider.ConsumerVersionSelector{Consumer: recapWorkerConsumer, DeployedOrReleased: true},
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
		Provider:           providerName,
		ProviderBaseURL:    fmt.Sprintf("http://127.0.0.1:%d", port),
		FilterConsumers:    []string{altctlConsumer},
		FailIfNoPactsFound: true,
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
			&provider.ConsumerVersionSelector{Consumer: altctlConsumer, MainBranch: true},
			&provider.ConsumerVersionSelector{Consumer: altctlConsumer, DeployedOrReleased: true},
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
