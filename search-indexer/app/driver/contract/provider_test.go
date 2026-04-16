//go:build contract

// Provider verification for search-indexer.
//
// Replays the Pact files published by search-indexer's consumers against a
// stub HTTP server that mirrors the real endpoints. Authentication is
// established at the TLS transport layer (mTLS peer-identity allowlist);
// the stub does not gate on X-Service-Token because the pact replay does
// not present a TLS peer.
package contract

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pact-foundation/pact-go/v2/models"
	"github.com/pact-foundation/pact-go/v2/provider"
	"github.com/stretchr/testify/require"
)

// emptyResultState is toggled by the "search-indexer has no matching articles"
// provider state so the REST stub returns empty hits for that interaction.
var emptyResultState atomic.Bool

const (
	providerPactDirAltBackend = "../../../../alt-backend/pacts"
	providerPactDirRAG        = "../../../../rag-orchestrator/pacts"
	providerPactDirRoot       = "../../../../pacts"
)

// searchHit mirrors the schema asserted by consumer pacts.
type searchHit struct {
	ID      string   `json:"id"`
	Title   string   `json:"title"`
	Content string   `json:"content"`
	Tags    []string `json:"tags"`
}

type searchArticlesResponse struct {
	Query string      `json:"query"`
	Hits  []searchHit `json:"hits"`
}

type connectSearchArticlesResponse struct {
	Hits               []searchHit `json:"hits"`
	EstimatedTotalHits int         `json:"estimatedTotalHits"`
}

type recapHit struct {
	JobID      string   `json:"jobId"`
	ExecutedAt string   `json:"executedAt"`
	WindowDays int      `json:"windowDays"`
	Genre      string   `json:"genre"`
	Summary    string   `json:"summary"`
	TopTerms   []string `json:"topTerms"`
	Tags       []string `json:"tags"`
	Bullets    []string `json:"bullets"`
}

type connectSearchRecapsResponse struct {
	Hits               []recapHit `json:"hits"`
	EstimatedTotalHits int        `json:"estimatedTotalHits"`
}

// startProviderStub starts a minimal HTTP server that mirrors the endpoints
// exercised by the pacts. Authentication is handled at the transport layer
// (mTLS client cert) in production; the stub deliberately accepts any caller
// because the pact replay does not present a TLS peer.
func startProviderStub(t *testing.T) int {
	t.Helper()

	mux := http.NewServeMux()

	// REST /v1/search for rag-orchestrator and acolyte-orchestrator pacts.
	// emptyResultState toggles empty-hits responses for the acolyte
	// "no matching articles" provider state.
	restSearch := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("q")
		w.Header().Set("Content-Type", "application/json")

		resp := searchArticlesResponse{Query: q}
		if emptyResultState.Load() {
			resp.Hits = []searchHit{}
		} else {
			resp.Hits = []searchHit{
				{
					ID:      "article-1",
					Title:   "An LLM primer",
					Content: "Some content",
					Tags:    []string{"ai"},
				},
			}
		}
		_ = json.NewEncoder(w).Encode(resp)
	})
	mux.Handle("/v1/search", restSearch)

	// Connect-RPC POST /services.search.v2.SearchService/SearchArticles
	connectSearchArticles := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := connectSearchArticlesResponse{
			Hits: []searchHit{
				{
					ID:      "article-1",
					Title:   "An LLM primer",
					Content: "body",
					Tags:    []string{"ai"},
				},
			},
			EstimatedTotalHits: 1,
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	})
	mux.Handle("/services.search.v2.SearchService/SearchArticles", connectSearchArticles)

	// Connect-RPC POST /services.search.v2.SearchService/SearchRecaps
	connectSearchRecaps := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := connectSearchRecapsResponse{
			Hits: []recapHit{
				{
					JobID:      "job-1",
					ExecutedAt: "2026-04-10T00:00:00Z",
					WindowDays: 7,
					Genre:      "technology",
					Summary:    "weekly recap",
					TopTerms:   []string{"ai"},
					Tags:       []string{"technology"},
					Bullets:    []string{"bullet"},
				},
			},
			EstimatedTotalHits: 1,
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	})
	mux.Handle("/services.search.v2.SearchService/SearchRecaps", connectSearchRecaps)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = ln.Close() })

	go func() { _ = http.Serve(ln, mux) }()

	return ln.Addr().(*net.TCPAddr).Port
}

func findProviderPacts(t *testing.T) []string {
	t.Helper()
	// All three search-indexer consumers are now verified: rag-orchestrator,
	// alt-backend, and acolyte-orchestrator. acolyte's consumer test was
	// updated in to pin X-Service-Token (PM-2026-025 remediation),
	// so the replay now satisfies the real ServiceAuthMiddleware.
	candidates := []string{
		filepath.Join(providerPactDirRAG, "rag-orchestrator-search-indexer.json"),
		filepath.Join(providerPactDirAltBackend, "alt-backend-search-indexer.json"),
		filepath.Join(providerPactDirRoot, "acolyte-orchestrator-search-indexer.json"),
	}
	found := make([]string, 0, len(candidates))
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			found = append(found, c)
		} else {
			t.Logf("skipping missing pact: %s", c)
		}
	}
	return found
}

func TestVerifySearchIndexerProviderContracts(t *testing.T) {
	pactFiles := findProviderPacts(t)
	if len(pactFiles) == 0 {
		t.Skip("no consumer pacts found — run consumer tests first")
	}

	port := startProviderStub(t)

	stateHandlers := models.StateHandlers{
		"a service token is configured and search has indexed articles": func(setup bool, s models.ProviderState) (models.ProviderStateResponse, error) {
			emptyResultState.Store(false)
			return nil, nil
		},
		"a service token is configured and articles are indexed": func(setup bool, s models.ProviderState) (models.ProviderStateResponse, error) {
			emptyResultState.Store(false)
			return nil, nil
		},
		"a service token is configured and recap jobs are indexed under a tag": func(setup bool, s models.ProviderState) (models.ProviderStateResponse, error) {
			emptyResultState.Store(false)
			return nil, nil
		},
		"search-indexer has indexed articles": func(setup bool, s models.ProviderState) (models.ProviderStateResponse, error) {
			emptyResultState.Store(false)
			return nil, nil
		},
		"search-indexer has no matching articles": func(setup bool, s models.ProviderState) (models.ProviderStateResponse, error) {
			emptyResultState.Store(setup)
			return nil, nil
		},
		"search-indexer has indexed articles and a service token is configured": func(setup bool, s models.ProviderState) (models.ProviderStateResponse, error) {
			emptyResultState.Store(false)
			return nil, nil
		},
		"search-indexer has no matching articles and a service token is configured": func(setup bool, s models.ProviderState) (models.ProviderStateResponse, error) {
			emptyResultState.Store(setup)
			return nil, nil
		},
	}

	verifyRequest := provider.VerifyRequest{
		Provider:        "search-indexer",
		ProviderBaseURL: fmt.Sprintf("http://127.0.0.1:%d", port),
		PactFiles:       pactFiles,
		StateHandlers:   stateHandlers,
	}

	if brokerURL := os.Getenv("PACT_BROKER_BASE_URL"); brokerURL != "" {
		verifyRequest.PactFiles = nil
		verifyRequest.BrokerURL = brokerURL
		verifyRequest.BrokerUsername = os.Getenv("PACT_BROKER_USERNAME")
		verifyRequest.BrokerPassword = os.Getenv("PACT_BROKER_PASSWORD")
		verifyRequest.ConsumerVersionSelectors = []provider.Selector{
			&provider.ConsumerVersionSelector{Consumer: "rag-orchestrator", MainBranch: true},
			&provider.ConsumerVersionSelector{Consumer: "rag-orchestrator", DeployedOrReleased: true},
			&provider.ConsumerVersionSelector{Consumer: "alt-backend", MainBranch: true},
			&provider.ConsumerVersionSelector{Consumer: "alt-backend", DeployedOrReleased: true},
			&provider.ConsumerVersionSelector{Consumer: "acolyte-orchestrator", MainBranch: true},
			&provider.ConsumerVersionSelector{Consumer: "acolyte-orchestrator", DeployedOrReleased: true},
		}
		if ver := os.Getenv("PACT_PROVIDER_VERSION"); ver != "" {
			verifyRequest.ProviderVersion = ver
			verifyRequest.PublishVerificationResults = true
		}
		if branch := os.Getenv("PACT_PROVIDER_BRANCH"); branch != "" {
			verifyRequest.ProviderBranch = branch
		}
		// Pending pacts: new contracts warn instead of breaking the provider
		// build until they have been verified at least once.
		if os.Getenv("PACT_DISABLE_PENDING") != "true" {
			verifyRequest.EnablePending = true
		}
		if since := os.Getenv("PACT_INCLUDE_WIP_SINCE"); since != "" {
			if t, err := time.Parse(time.RFC3339, since); err == nil {
				verifyRequest.IncludeWIPPactsSince = &t
			}
		}
	}

	verifier := provider.NewVerifier()
	err := verifier.VerifyProvider(t, verifyRequest)
	require.NoError(t, err)
}
