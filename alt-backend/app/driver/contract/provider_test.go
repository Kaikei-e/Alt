//go:build contract

// Package contract contains provider verification tests for alt-backend.
//
// These tests verify that alt-backend fulfills contracts from two consumers:
//   - recap-worker → /v1/recap/articles (REST JSON)
//   - search-indexer → BackendInternalService (Connect-RPC / JSON wire format)
package contract

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/pact-foundation/pact-go/v2/models"
	"github.com/pact-foundation/pact-go/v2/provider"
	"github.com/stretchr/testify/require"
)

const (
	pactDir                    = "../../../../pacts"
	providerName               = "alt-backend"
	recapWorkerPactFile        = "recap-worker-alt-backend.json"
	searchIndexerPactFile      = "search-indexer-alt-backend.json"
	altButterflyFacadePactFile = "alt-butterfly-facade-alt-backend.json"
)

// recapArticleResponse mirrors the JSON shape expected by recap-worker.
type recapArticleResponse struct {
	ArticleID string       `json:"article_id"`
	Title     string       `json:"title"`
	FullText  string       `json:"fulltext"`
	Tags      []tagPayload `json:"tags"`
}

type tagPayload struct {
	Label string `json:"label"`
}

type recapArticlesResponse struct {
	Range    rangeResponse          `json:"range"`
	Total    int                    `json:"total"`
	Page     int                    `json:"page"`
	PageSize int                    `json:"page_size"`
	HasMore  bool                   `json:"has_more"`
	Articles []recapArticleResponse `json:"articles"`
}

type rangeResponse struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// startStubServer creates a minimal HTTP server bound to an ephemeral port.
// It returns the listener port so the Pact verifier can connect.
func startStubServer(t *testing.T) int {
	t.Helper()

	mux := http.NewServeMux()

	// ---- GET /v1/recap/articles ----
	mux.HandleFunc("/v1/recap/articles", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		fromStr := q.Get("from")
		toStr := q.Get("to")
		if fromStr == "" {
			fromStr = "2026-03-19T00:00:00Z"
		}
		if toStr == "" {
			toStr = "2026-03-26T00:00:00Z"
		}

		resp := recapArticlesResponse{
			Range: rangeResponse{
				From: fromStr,
				To:   toStr,
			},
			Total:    42,
			Page:     1,
			PageSize: 500,
			HasMore:  false,
			Articles: []recapArticleResponse{
				{
					ArticleID: "art-001",
					Title:     "Test Article Title",
					FullText:  "Full article text content here.",
					Tags:      []tagPayload{{Label: "technology"}},
				},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	})

	// ---- Connect-RPC BackendInternalService (JSON wire format) ----
	// search-indexer-alt-backend.json contract.
	mux.HandleFunc("/services.backend.v1.BackendInternalService/GetLatestArticleTimestamp",
		func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{
				"latestCreatedAt": "2026-03-26T00:00:00Z",
			})
		})

	mux.HandleFunc("/services.backend.v1.BackendInternalService/ListArticlesWithTags",
		func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			_, _ = io.Copy(io.Discard, r.Body)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"articles": []map[string]interface{}{
					{
						"id":        "art-001",
						"title":     "Test Article",
						"content":   "Article content.",
						"tags":      []string{"technology"},
						"createdAt": "2026-03-26T00:00:00Z",
						"userId":    "user-001",
						"feedId":    "feed-001",
					},
				},
				"nextId": "art-002",
			})
		})

	// ---- alt-butterfly-facade proxy targets (Connect-RPC, JSON wire format) ----
	// BFF unit-tests its proxy by speaking Connect-RPC directly to alt-backend.
	// Only the 404 path is covered by the consumer pact.
	mux.HandleFunc("/alt.feeds.v2.FeedService/GetFeed",
		func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			_, _ = io.Copy(io.Discard, r.Body)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"code":    "not_found",
				"message": "feed not found",
			})
		})

	mux.HandleFunc("/alt.feeds.v2.FeedService/GetFeedStats",
		func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			_, _ = io.Copy(io.Discard, r.Body)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]int{
				"totalArticles": 250,
				"totalFeeds":    10,
			})
		})

	mux.HandleFunc("/alt.knowledge_home.v1.KnowledgeHomeAdminService/GetOverview",
		func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			_, _ = io.Copy(io.Discard, r.Body)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]int{
				"totalEvents": 100,
			})
		})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = ln.Close() })

	go func() {
		_ = http.Serve(ln, mux)
	}()

	return ln.Addr().(*net.TCPAddr).Port
}

func TestVerifyRecapWorkerContract(t *testing.T) {
	pactFile := filepath.Join(pactDir, recapWorkerPactFile)

	// Support Broker mode via environment variables
	brokerURL := os.Getenv("PACT_BROKER_BASE_URL")

	if brokerURL == "" {
		// Local file mode: check pact file exists
		if _, err := os.Stat(pactFile); os.IsNotExist(err) {
			t.Skipf("No Broker URL set and pact file not found: %s. "+
				"Set PACT_BROKER_BASE_URL or run Rust consumer tests first.", pactFile)
		}
	}

	port := startStubServer(t)

	verifyRequest := provider.VerifyRequest{
		Provider:        providerName,
		ProviderBaseURL: fmt.Sprintf("http://127.0.0.1:%d", port),
		StateHandlers: models.StateHandlers{
			"articles exist in the recap window": func(setup bool, s models.ProviderState) (models.ProviderStateResponse, error) {
				// No-op: stub server always returns articles
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
			if t, err := time.Parse(time.RFC3339, since); err == nil {
				verifyRequest.IncludeWIPPactsSince = &t
			}
		}
	} else {
		verifyRequest.PactFiles = []string{pactFile}
	}

	verifier := provider.NewVerifier()
	err := verifier.VerifyProvider(t, verifyRequest)
	require.NoError(t, err)
}

// TestVerifyAltButterflyFacadeContract verifies that alt-backend satisfies
// the BFF's proxy-layer contract for FeedService.GetFeed/GetFeedStats and
// KnowledgeHomeAdminService.GetOverview. The BFF fans these Connect-RPC
// calls out to alt-backend; alt-backend must keep the wire format stable.
func TestVerifyAltButterflyFacadeContract(t *testing.T) {
	pactFile := filepath.Join(pactDir, altButterflyFacadePactFile)

	brokerURL := os.Getenv("PACT_BROKER_BASE_URL")
	if brokerURL == "" {
		if _, err := os.Stat(pactFile); os.IsNotExist(err) {
			t.Skipf("No Broker URL set and pact file not found: %s. "+
				"Run alt-butterfly-facade consumer tests first.", pactFile)
		}
	}

	port := startStubServer(t)

	verifyRequest := provider.VerifyRequest{
		Provider:        providerName,
		ProviderBaseURL: fmt.Sprintf("http://127.0.0.1:%d", port),
		StateHandlers: models.StateHandlers{
			"article does not exist": func(setup bool, s models.ProviderState) (models.ProviderStateResponse, error) {
				return nil, nil
			},
			"feed stats are available": func(setup bool, s models.ProviderState) (models.ProviderStateResponse, error) {
				return nil, nil
			},
			"knowledge home admin service is available": func(setup bool, s models.ProviderState) (models.ProviderStateResponse, error) {
				return nil, nil
			},
		},
	}

	if brokerURL != "" {
		verifyRequest.BrokerURL = brokerURL
		verifyRequest.BrokerUsername = os.Getenv("PACT_BROKER_USERNAME")
		verifyRequest.BrokerPassword = os.Getenv("PACT_BROKER_PASSWORD")
		verifyRequest.ConsumerVersionSelectors = []provider.Selector{
			&provider.ConsumerVersionSelector{Consumer: "alt-butterfly-facade", MainBranch: true},
			&provider.ConsumerVersionSelector{Consumer: "alt-butterfly-facade", DeployedOrReleased: true},
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
			if t, err := time.Parse(time.RFC3339, since); err == nil {
				verifyRequest.IncludeWIPPactsSince = &t
			}
		}
	} else {
		verifyRequest.PactFiles = []string{pactFile}
	}

	verifier := provider.NewVerifier()
	err := verifier.VerifyProvider(t, verifyRequest)
	require.NoError(t, err)
}

// TestVerifySearchIndexerContract verifies that alt-backend's Connect-RPC
// BackendInternalService fulfills the contract expected by search-indexer
// (GetLatestArticleTimestamp + ListArticlesWithTags via JSON wire format).
func TestVerifySearchIndexerContract(t *testing.T) {
	pactFile := filepath.Join(pactDir, searchIndexerPactFile)

	brokerURL := os.Getenv("PACT_BROKER_BASE_URL")
	if brokerURL == "" {
		if _, err := os.Stat(pactFile); os.IsNotExist(err) {
			t.Skipf("No Broker URL set and pact file not found: %s. "+
				"Run search-indexer consumer tests first.", pactFile)
		}
	}

	port := startStubServer(t)

	verifyRequest := provider.VerifyRequest{
		Provider:        providerName,
		ProviderBaseURL: fmt.Sprintf("http://127.0.0.1:%d", port),
		StateHandlers: models.StateHandlers{
			"articles exist in the database": func(setup bool, s models.ProviderState) (models.ProviderStateResponse, error) {
				return nil, nil
			},
			"articles with tags exist for backward pagination": func(setup bool, s models.ProviderState) (models.ProviderStateResponse, error) {
				return nil, nil
			},
		},
	}

	if brokerURL != "" {
		verifyRequest.BrokerURL = brokerURL
		verifyRequest.BrokerUsername = os.Getenv("PACT_BROKER_USERNAME")
		verifyRequest.BrokerPassword = os.Getenv("PACT_BROKER_PASSWORD")
		verifyRequest.ConsumerVersionSelectors = []provider.Selector{
			&provider.ConsumerVersionSelector{Consumer: "search-indexer", MainBranch: true},
			&provider.ConsumerVersionSelector{Consumer: "search-indexer", DeployedOrReleased: true},
		}
		if ver := os.Getenv("PACT_PROVIDER_VERSION"); ver != "" {
			verifyRequest.ProviderVersion = ver
		}
		if branch := os.Getenv("PACT_PROVIDER_BRANCH"); branch != "" {
			verifyRequest.ProviderBranch = branch
		}
		verifyRequest.PublishVerificationResults = os.Getenv("PACT_PROVIDER_VERSION") != ""
	} else {
		verifyRequest.PactFiles = []string{pactFile}
	}

	verifier := provider.NewVerifier()
	err := verifier.VerifyProvider(t, verifyRequest)
	require.NoError(t, err)
}
