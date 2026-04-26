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
// The verification uses a minimal stub HTTP server that encodes the
// provider's Connect-RPC contract; it does not spin up Postgres.
// This matches the precedent set by alt-backend/app/driver/contract/
// provider_test.go.
package contract

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
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

// applyMutationRequest mirrors the shared wire shape of
// Apply{Projection,Recall,Curation}MutationRequest. protojson uses
// camelCase; `payload` is a base64 string on the wire because the
// proto field is `bytes`.
type applyMutationRequest struct {
	MutationType   string `json:"mutationType"`
	EntityId       string `json:"entityId"`
	Payload        string `json:"payload"`
	IdempotencyKey string `json:"idempotencyKey"`
}

// applyMutationResponse mirrors the shared response shape.
type applyMutationResponse struct {
	Success      bool   `json:"success"`
	ErrorMessage string `json:"errorMessage,omitempty"`
}

// startStubServer encodes the sovereign Connect-RPC mutation contract
// as a tiny HTTP stub. Every supported mutation returns success=true
// unless the consumer explicitly declares a rejection state via the
// "mutation is rejected" provider-state handler.
func startStubServer(t *testing.T, reject *bool) int {
	t.Helper()

	mux := http.NewServeMux()

	mutationHandler := func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var req applyMutationRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		_, _ = io.Copy(io.Discard, r.Body)

		w.Header().Set("Content-Type", "application/json")
		if reject != nil && *reject {
			_ = json.NewEncoder(w).Encode(applyMutationResponse{
				Success:      false,
				ErrorMessage: "projection version mismatch",
			})
			return
		}
		_ = json.NewEncoder(w).Encode(applyMutationResponse{Success: true})
	}

	mux.HandleFunc("/services.sovereign.v1.KnowledgeSovereignService/ApplyProjectionMutation", mutationHandler)
	mux.HandleFunc("/services.sovereign.v1.KnowledgeSovereignService/ApplyRecallMutation", mutationHandler)
	mux.HandleFunc("/services.sovereign.v1.KnowledgeSovereignService/ApplyCurationMutation", mutationHandler)

	// AppendKnowledgeEvent (ADR-000840 versioned event_type convention).
	// Consumer is alt-backend's TransitionKnowledgeLoopUsecase, which
	// appends knowledge_loop.{observed,deferred,acted}.v1 events. The
	// provider's wire-shape responsibility is just `{"success": true}`
	// on a well-formed request — projector-side dedupe / same-stage
	// validation lives in the sovereign DB driver, out of this contract.
	mux.HandleFunc("/services.sovereign.v1.KnowledgeSovereignService/AppendKnowledgeEvent", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true})
	})

	// Admin REST surface on the same listener: pact-go routes every
	// consumer interaction through the single ProviderBaseURL, so we
	// serve Connect-RPC and admin REST on the same port.
	mux.HandleFunc("/admin/snapshots/create", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"SnapshotID":       "11111111-2222-3333-4444-555555555555",
			"EventSeqBoundary": 1,
			"ItemsChecksum":    "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			"VersionsChecksum": "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
			"DedupesChecksum":  "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
			"CreatedAt":        "2026-04-23T00:00:00Z",
		})
	})
	mux.HandleFunc("/admin/snapshots/latest", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"SnapshotID":       "11111111-2222-3333-4444-555555555555",
			"EventSeqBoundary": 1,
			"ItemsChecksum":    "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			"VersionsChecksum": "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
			"DedupesChecksum":  "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
			"CreatedAt":        "2026-04-23T00:00:00Z",
		})
	})
	mux.HandleFunc("/admin/snapshots/list", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]map[string]any{{
			"SnapshotID":       "11111111-2222-3333-4444-555555555555",
			"EventSeqBoundary": 1,
			"ItemsChecksum":    "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			"VersionsChecksum": "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
			"DedupesChecksum":  "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
			"CreatedAt":        "2026-04-23T00:00:00Z",
		}})
	})
	mux.HandleFunc("/admin/retention/eligible", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"Table": "knowledge_events", "PartitionName": "knowledge_events_y2025m01", "EventSeqMax": 1},
			{"Table": "knowledge_user_events", "PartitionName": "knowledge_user_events_y2025m01", "EventSeqMax": 1},
		})
	})
	mux.HandleFunc("/admin/retention/run", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"dry_run": true,
			"actions": []map[string]any{
				{
					"table":          "knowledge_events",
					"partition_name": "knowledge_events_y2025m01",
					"action":         "would_archive",
				},
			},
		})
	})
	mux.HandleFunc("/admin/storage/stats", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"Tables": []map[string]any{
				{"Name": "knowledge_events", "Rows": 0, "SizeBytes": 0},
			},
		})
	})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = ln.Close() })

	go func() { _ = http.Serve(ln, mux) }()
	return ln.Addr().(*net.TCPAddr).Port
}

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

	reject := false
	port := startStubServer(t, &reject)

	verifyRequest := provider.VerifyRequest{
		Provider:        providerName,
		ProviderBaseURL: fmt.Sprintf("http://127.0.0.1:%d", port),
		StateHandlers: models.StateHandlers{
			"the projection mutation upsert_home_item is accepted": func(setup bool, s models.ProviderState) (models.ProviderStateResponse, error) {
				reject = false
				return nil, nil
			},
			"the projection mutation is rejected with an error": func(setup bool, s models.ProviderState) (models.ProviderStateResponse, error) {
				reject = true
				return nil, nil
			},
			"the recall mutation snooze_candidate is accepted": func(setup bool, s models.ProviderState) (models.ProviderStateResponse, error) {
				reject = false
				return nil, nil
			},
			"the curation mutation create_lens is accepted": func(setup bool, s models.ProviderState) (models.ProviderStateResponse, error) {
				reject = false
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

	reject := false
	port := startStubServer(t, &reject)

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

	reject := false
	port := startStubServer(t, &reject)

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

	reject := false
	port := startStubServer(t, &reject)

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
