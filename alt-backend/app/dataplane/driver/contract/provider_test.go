//go:build contract

// Package contract contains provider verification tests for the data plane.
//
// Every consumer of alt.datahub.v1.DataHubService is verified here, plus the
// browser-facing services alt-butterfly-facade proxies:
//
//	consumer            pacticipant it names as provider
//	recap-worker        alt-backend
//	search-indexer      alt-backend
//	pre-processor       alt-backend
//	tag-generator       alt-backend
//	rag-orchestrator    alt-data-hub
//	alt-butterfly-facade alt-backend
//
// Two provider names for one binary is a naming debt, not a second surface:
// alt-data-hub is the deployment that serves DataHubService after the
// ADR-000954 split, and services.yaml registers it as `kind: runtime` rather
// than as a pacticipant of its own. Renaming the pacticipant is a separate
// change — a rename on the Broker orphans the verification history of every
// pact under the old name — so until then the rag-orchestrator pact is
// verified under the name it was published with, against the same stub.
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
	pactDir = "../../../../../pacts"
	// ragPactDir: rag-orchestrator writes its pacts into its own module rather
	// than the repo-root directory, and scripts/pact-check.sh publishes from
	// both.
	ragPactDir = "../../../../../rag-orchestrator/pacts"

	providerName = "alt-backend"
	// dataHubProviderName is the pacticipant rag-orchestrator publishes
	// against — see the package comment.
	dataHubProviderName = "alt-data-hub"

	recapWorkerPactFile        = "recap-worker-alt-backend.json"
	searchIndexerPactFile      = "search-indexer-alt-backend.json"
	altButterflyFacadePactFile = "alt-butterfly-facade-alt-backend.json"
	preProcessorPactFile       = "pre-processor-alt-backend.json"
	tagGeneratorPactFile       = "tag-generator-alt-backend.json"
	ragOrchestratorPactFile    = "rag-orchestrator-alt-data-hub.json"
)

// recapArticleResponse mirrors the Connect-RPC JSON shape produced by
// BackendInternalService/ListRecapArticles. protojson uses camelCase, so
// the JSON tags do the same.
type recapArticleResponse struct {
	ArticleID string `json:"articleId"`
	Title     string `json:"title"`
	FullText  string `json:"fulltext"`
}

type recapArticlesResponse struct {
	Range    rangeResponse          `json:"range"`
	Total    int                    `json:"total"`
	Page     int                    `json:"page"`
	PageSize int                    `json:"pageSize"`
	HasMore  bool                   `json:"hasMore"`
	Articles []recapArticleResponse `json:"articles"`
}

// legacyRestRecapArticleResponse / legacyRestRecapArticlesResponse mirror the
// pre-Connect-RPC REST shape. Broker "DeployedOrReleased" pacts from the
// transitional commits still expect snake_case fields with a tags array.
// Kept only as long as those pacts are the deployed version on the broker.
type legacyRestRecapArticleResponse struct {
	ArticleID string          `json:"article_id"`
	Title     string          `json:"title"`
	FullText  string          `json:"fulltext"`
	Tags      []legacyRestTag `json:"tags"`
}

type legacyRestTag struct {
	Label string `json:"label"`
}

type legacyRestRecapArticlesResponse struct {
	Range    rangeResponse                    `json:"range"`
	Total    int                              `json:"total"`
	Page     int                              `json:"page"`
	PageSize int                              `json:"page_size"`
	HasMore  bool                             `json:"has_more"`
	Articles []legacyRestRecapArticleResponse `json:"articles"`
}

type rangeResponse struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// listRecapArticlesRequest mirrors the Connect-RPC request body.
type listRecapArticlesRequest struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// dataHubProcedure mounts one procedure of alt.datahub.v1.DataHubService, the
// only name the data plane answers to since ADR-000954 Wave 2-C.
//
// It mounted the retired services.backend.v1.BackendInternalService path as
// well while Wave 2-B moved the consumers one PR at a time, so that a peer's
// migration did not also need an edit here. Both names are no longer served by
// the real handler, and a stub that kept answering the old one would let a
// consumer pact still naming it verify green against a provider that would
// 404 it in production.
func dataHubProcedure(mux *http.ServeMux, procedure string, h http.HandlerFunc) {
	mux.HandleFunc("/alt.datahub.v1.DataHubService/"+procedure, h)
}

// jsonPost answers POST with body and rejects every other method, which is the
// shape every Connect-RPC unary procedure has over the JSON wire format.
func jsonPost(body map[string]interface{}) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(body)
	}
}

// startStubServer creates a minimal HTTP server bound to an ephemeral port.
// It returns the listener port so the Pact verifier can connect.
func startStubServer(t *testing.T) int {
	t.Helper()

	mux := http.NewServeMux()

	// Shared handler for the recap-worker paginated article window fetch.
	recapArticlesHandler := func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		var req listRecapArticlesRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if req.From == "" {
			req.From = "2026-03-19T00:00:00Z"
		}
		if req.To == "" {
			req.To = "2026-03-26T00:00:00Z"
		}

		resp := recapArticlesResponse{
			Range: rangeResponse{
				From: req.From,
				To:   req.To,
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
				},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}

	// ---- POST .../ListRecapArticles ----
	dataHubProcedure(mux, "ListRecapArticles", recapArticlesHandler)

	// Transitional shims: the broker's DeployedOrReleased selector still
	// advertises older recap-worker versions whose pact targets either the
	// first Connect-RPC path or the original REST path. Serve the same stub
	// under each so provider verification stays green until the next
	// successful deployment supersedes them. Remove once the deployed
	// version advances past 7575478fc.
	mux.HandleFunc("/alt.recap.v2.RecapService/ListRecapArticles", recapArticlesHandler)
	mux.HandleFunc("/v1/recap/articles", func(w http.ResponseWriter, r *http.Request) {
		// Legacy REST path: articles came back through query params, not a body.
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		fromStr := r.URL.Query().Get("from")
		toStr := r.URL.Query().Get("to")
		if fromStr == "" {
			fromStr = "2026-03-19T00:00:00Z"
		}
		if toStr == "" {
			toStr = "2026-03-26T00:00:00Z"
		}
		resp := legacyRestRecapArticlesResponse{
			Range:    rangeResponse{From: fromStr, To: toStr},
			Total:    42,
			Page:     1,
			PageSize: 500,
			HasMore:  false,
			Articles: []legacyRestRecapArticleResponse{
				{
					ArticleID: "art-001",
					Title:     "Test Article Title",
					FullText:  "Full article text content here.",
					Tags:      []legacyRestTag{{Label: "technology"}},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	})

	// ---- alt.datahub.v1.DataHubService (JSON wire format) ----
	// search-indexer-alt-backend.json contract.
	dataHubProcedure(mux, "GetLatestArticleTimestamp",
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

	dataHubProcedure(mux, "ListArticlesWithTags",
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

	// search-indexer-alt-backend.json: "a GetArticleByID request".
	// published_at is deliberately a different instant from created_at: the
	// consumer indexes documents from this response alone, so substituting
	// created_at here would silently regress its date filter.
	dataHubProcedure(mux, "GetArticleByID",
		func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			_, _ = io.Copy(io.Discard, r.Body)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"article": map[string]interface{}{
					"id":          "art-001",
					"title":       "Test Article",
					"content":     "Article content.",
					"tags":        []string{"technology"},
					"createdAt":   "2026-03-26T00:00:00Z",
					"userId":      "user-001",
					"feedId":      "feed-001",
					"language":    "en",
					"publishedAt": "2026-03-20T09:30:00Z",
				},
			})
		})

	// recap-worker-alt-backend.json: "a batch tags request by article ids"
	// recap-worker fetches tags for a batch of article ids to enrich the
	// recap payload. Connect-RPC, JSON wire format, camelCase keys.
	dataHubProcedure(mux, "BatchGetTagsByArticleIDs",
		func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			_, _ = io.Copy(io.Discard, r.Body)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"items": []map[string]interface{}{
					{
						"articleId": "art-001",
						"tags": []map[string]interface{}{
							{
								"tagName":    "technology",
								"confidence": 0.95,
								"source":     "ml_model",
								"updatedAt":  "2026-03-26T00:00:00Z",
							},
						},
					},
				},
			})
		})

	// ---- pre-processor-alt-backend.json ----
	// The crawl/summarise loop: resolve a feed, write an article, poll for
	// unsummarised ones, write the summary back. GetSystemUser is the identity
	// every one of those writes is attributed to — it was GET
	// /v1/internal/system-user until ADR-000954 D6 folded it into this service,
	// which is why a pact naming it as an RPC is what proves the fold landed.
	dataHubProcedure(mux, "GetSystemUser", jsonPost(map[string]interface{}{
		"userId": "11111111-2222-3333-4444-555555555555",
	}))

	dataHubProcedure(mux, "GetFeedID", jsonPost(map[string]interface{}{
		"feedId": "feed-001",
	}))

	dataHubProcedure(mux, "CreateArticle", jsonPost(map[string]interface{}{
		"articleId": "art-001",
	}))

	dataHubProcedure(mux, "ListFeedURLs", jsonPost(map[string]interface{}{
		"feeds": []map[string]interface{}{
			{"feedId": "feed-001", "url": "https://example.com/feed.xml"},
		},
		"nextCursor": "feed-002",
		"hasMore":    true,
	}))

	dataHubProcedure(mux, "ListUnsummarizedArticles", jsonPost(map[string]interface{}{
		"articles": []map[string]interface{}{
			{
				"id":        "art-001",
				"title":     "Go 1.26 Released",
				"content":   "Article body text.",
				"url":       "https://example.com/articles/go-126",
				"createdAt": "2026-03-26T00:00:00Z",
				"userId":    "user-001",
			},
		},
		"nextCreatedAt": "2026-03-26T00:00:00Z",
		"nextId":        "art-002",
	}))

	// success is sent explicitly rather than left to protojson's zero-value
	// omission: the consumer reads it to decide whether the summary was
	// persisted, and an absent field would read as false.
	dataHubProcedure(mux, "SaveArticleSummary", jsonPost(map[string]interface{}{
		"success": true,
	}))

	// ---- tag-generator-alt-backend.json ----
	// One ListUntaggedArticles stub covers both interactions in that pact: the
	// backlog probe (limit 1) reads totalCount, the first page (limit 75) reads
	// the cursor too, and Pact tolerates response keys an interaction does not
	// mention. Branching on the request limit would encode the consumer's
	// current page size into the provider, which is exactly the coupling the
	// cursor exists to avoid.
	dataHubProcedure(mux, "ListUntaggedArticles", jsonPost(map[string]interface{}{
		"articles": []map[string]interface{}{
			{
				"id":        "art-001",
				"title":     "Rust Memory Safety",
				"content":   "An article about memory safety in Rust.",
				"userId":    "user-001",
				"feedId":    "feed-001",
				"createdAt": "2026-03-26T00:00:00Z",
			},
		},
		"totalCount": 42,
		"nextId":     "art-002",
	}))

	dataHubProcedure(mux, "GetArticleContent", jsonPost(map[string]interface{}{
		"articleId": "art-001",
		"title":     "Rust Memory Safety",
		"content":   "An article about memory safety in Rust.",
		"url":       "https://example.com/rust-memory-safety",
		"userId":    "user-001",
	}))

	dataHubProcedure(mux, "UpsertArticleTags", jsonPost(map[string]interface{}{
		"success":       true,
		"upsertedCount": 2,
	}))

	dataHubProcedure(mux, "BatchUpsertArticleTags", jsonPost(map[string]interface{}{
		"success":       true,
		"totalUpserted": 2,
	}))

	// ---- rag-orchestrator-alt-data-hub.json ----
	// The RAG tool surface (ADR-000617) plus ListRecentArticles, the other
	// route ADR-000954 D6 absorbed. The ids here are real UUIDs because the
	// consumer pins them with a UUID regex matcher — it parses them, so a
	// placeholder like "art-001" would verify green and fail in production.
	dataHubProcedure(mux, "FetchTagCloud", jsonPost(map[string]interface{}{
		"tags": []map[string]interface{}{
			{"tagName": "ai", "articleCount": 42},
		},
	}))

	dataHubProcedure(mux, "FetchArticlesByTag", jsonPost(map[string]interface{}{
		"articles": []map[string]interface{}{
			{
				"id":          "article-1",
				"title":       "Multi-Agent Systems",
				"url":         "https://example.com/mas",
				"publishedAt": "2026-04-14T00:30:00Z",
			},
		},
	}))

	dataHubProcedure(mux, "ListRecentArticles", jsonPost(map[string]interface{}{
		"articles": []map[string]interface{}{
			{
				"id":          "6f1a2f7e-1f1e-4c2a-9a3e-5b6c7d8e9f01",
				"title":       "An LLM primer",
				"url":         "https://example.com/llm-primer",
				"publishedAt": "2026-04-14T00:30:00Z",
				"feedId":      "11111111-2222-3333-4444-555555555555",
				"tags":        []string{"ai"},
			},
		},
		"since": "2026-04-14T00:00:00Z",
		"until": "2026-04-15T00:00:00Z",
		"count": 1,
	}))

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
			"tags exist for the requested articles": func(setup bool, s models.ProviderState) (models.ProviderStateResponse, error) {
				// No-op: stub server always returns tags for art-001
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
			"an article with a source publication timestamp exists": func(setup bool, s models.ProviderState) (models.ProviderStateResponse, error) {
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

// noopStates turns a list of provider-state names into handlers that do
// nothing.
//
// Every state here is a no-op for the same reason: this verification runs
// against a stub, not against a database, so "articles exist" is already true
// by construction. The names still have to be declared — pact-go fails a
// verification whose pact names a state the provider does not know, which is
// what catches a consumer inventing a precondition nobody agreed to.
func noopStates(names ...string) models.StateHandlers {
	handlers := models.StateHandlers{}
	for _, name := range names {
		handlers[name] = func(bool, models.ProviderState) (models.ProviderStateResponse, error) {
			return nil, nil
		}
	}
	return handlers
}

// verifyConsumer runs one consumer's pact against the stub, in file mode
// locally and against the Broker in CI.
//
// The three verifications below differ only in consumer name, pact file and
// state list, so they share this rather than each carrying its own copy of the
// broker-vs-file branch. The three that came before it are left as they are:
// each has an accreted exception (recap-worker's transitional shims,
// search-indexer's missing WIP handling) and folding those in would mean
// parameters that exist for one caller.
func verifyConsumer(t *testing.T, consumer, providerPacticipant, pactPath string, states models.StateHandlers) {
	t.Helper()

	brokerURL := os.Getenv("PACT_BROKER_BASE_URL")
	if brokerURL == "" {
		if _, err := os.Stat(pactPath); os.IsNotExist(err) {
			t.Skipf("No Broker URL set and pact file not found: %s. "+
				"Run the %s consumer tests first.", pactPath, consumer)
		}
	}

	verifyRequest := provider.VerifyRequest{
		Provider:        providerPacticipant,
		ProviderBaseURL: fmt.Sprintf("http://127.0.0.1:%d", startStubServer(t)),
		StateHandlers:   states,
	}

	if brokerURL != "" {
		verifyRequest.BrokerURL = brokerURL
		verifyRequest.BrokerUsername = os.Getenv("PACT_BROKER_USERNAME")
		verifyRequest.BrokerPassword = os.Getenv("PACT_BROKER_PASSWORD")
		verifyRequest.ConsumerVersionSelectors = []provider.Selector{
			&provider.ConsumerVersionSelector{Consumer: consumer, MainBranch: true},
			&provider.ConsumerVersionSelector{Consumer: consumer, DeployedOrReleased: true},
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
	} else {
		verifyRequest.PactFiles = []string{pactPath}
	}

	require.NoError(t, provider.NewVerifier().VerifyProvider(t, verifyRequest))
}

// TestVerifyPreProcessorContract verifies the crawl/summarise loop's half of
// alt.datahub.v1.DataHubService.
//
// pre-processor is the only consumer that both reads and writes through this
// service, so it is the one whose pact would catch a write path answering with
// a shape its caller cannot read — and GetSystemUser is here rather than in a
// REST suite because ADR-000954 D6 moved it onto this service. Until this
// existed, alt-backend had a published pact from pre-processor that no
// provider job verified: the pact was generated, published, and never checked
// against anything.
func TestVerifyPreProcessorContract(t *testing.T) {
	verifyConsumer(t, "pre-processor", providerName,
		filepath.Join(pactDir, preProcessorPactFile),
		noopStates(
			"a feed exists with id feed-001",
			"a feed is registered for the requested url",
			"a Kratos identity exists",
			"registered feeds exist",
			"unsummarized articles exist",
			"an article exists with id art-001",
		))
}

// TestVerifyTagGeneratorContract verifies the tagging loop: find untagged
// articles, read one's body, write tags back one article at a time or in a
// batch.
func TestVerifyTagGeneratorContract(t *testing.T) {
	verifyConsumer(t, "tag-generator", providerName,
		filepath.Join(pactDir, tagGeneratorPactFile),
		noopStates(
			"untagged articles exist",
			"an article with body text exists",
			"the article exists and has no tags",
			"both articles exist and have no tags",
		))
}

// TestVerifyRAGOrchestratorContract verifies the RAG tool surface (ADR-000617)
// and ListRecentArticles.
//
// It names alt-data-hub as the provider because that is the pacticipant
// rag-orchestrator published against — see the package comment. The stub is
// the same one every other verification here runs against, which is the honest
// arrangement: one binary serves both names.
func TestVerifyRAGOrchestratorContract(t *testing.T) {
	verifyConsumer(t, "rag-orchestrator", dataHubProviderName,
		filepath.Join(ragPactDir, ragOrchestratorPactFile),
		noopStates(
			"alt-data-hub has articles tagged ai",
			"alt-data-hub has tagged articles",
			"alt-data-hub has articles published in the last 24 hours",
		))
}
