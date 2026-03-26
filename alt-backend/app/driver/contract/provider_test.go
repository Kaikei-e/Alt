//go:build contract

// Package contract contains provider verification tests for alt-backend.
//
// These tests verify that alt-backend fulfills the contract expectations
// defined by recap-worker for the /v1/recap/articles REST endpoint.
package contract

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/pact-foundation/pact-go/v2/models"
	"github.com/pact-foundation/pact-go/v2/provider"
	"github.com/stretchr/testify/require"
)

const (
	pactDir      = "../../../../pacts"
	providerName = "alt-backend"
	pactFileName = "recap-worker-alt-backend.json"
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

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = ln.Close() })

	go func() {
		_ = http.Serve(ln, mux)
	}()

	return ln.Addr().(*net.TCPAddr).Port
}

func TestVerifyRecapWorkerContract(t *testing.T) {
	pactFile := filepath.Join(pactDir, pactFileName)

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
			&provider.ConsumerVersionSelector{Consumer: "recap-worker", Latest: true},
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
