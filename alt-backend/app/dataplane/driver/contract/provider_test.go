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
	// altBackendPactDir holds the pacts this module writes as a *consumer* —
	// including the two in-family ones alt-backend and alt-harvester publish
	// against alt-data-hub since ADR-000954 Wave 3. Same convention as the
	// sovereign, pre-processor and search-indexer consumer pacts alongside
	// them; scripts/pact-check.sh publishes this directory too.
	altBackendPactDir = "../../../../pacts"

	providerName = "alt-backend"
	// dataHubProviderName is the pacticipant rag-orchestrator publishes
	// against — see the package comment.
	dataHubProviderName = "alt-data-hub"

	recapWorkerPactFile         = "recap-worker-alt-backend.json"
	searchIndexerPactFile       = "search-indexer-alt-backend.json"
	altButterflyFacadePactFile  = "alt-butterfly-facade-alt-backend.json"
	preProcessorPactFile        = "pre-processor-alt-backend.json"
	tagGeneratorPactFile        = "tag-generator-alt-backend.json"
	ragOrchestratorPactFile     = "rag-orchestrator-alt-data-hub.json"
	altBackendDataHubPactFile   = "alt-backend-alt-data-hub.json"
	altHarvesterDataHubPactFile = "alt-harvester-alt-data-hub.json"
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
// migration did not also need an edit here. That blanket dual-mount is gone:
// a stub that answered the old name for every procedure would let any future
// consumer pact naming it verify green against a provider that would 404 it
// in production. The BackendInternalService routes that remain are mounted
// explicitly in the transitional-shims block below, scoped to the pacts the
// production-deployed recap-worker and search-indexer still publish, with a
// remove-once-deployed-past marker.
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
	// search-indexer-alt-backend.json contract. The three handlers are named
	// so the BackendInternalService transitional shims below can mount the
	// same stubs under the retired path.
	latestArticleTimestampHandler := http.HandlerFunc(
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
	dataHubProcedure(mux, "GetLatestArticleTimestamp", latestArticleTimestampHandler)

	listArticlesWithTagsHandler := http.HandlerFunc(
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
	dataHubProcedure(mux, "ListArticlesWithTags", listArticlesWithTagsHandler)

	// search-indexer-alt-backend.json: "a GetArticleByID request".
	// published_at is deliberately a different instant from created_at: the
	// consumer indexes documents from this response alone, so substituting
	// created_at here would silently regress its date filter.
	getArticleByIDHandler := http.HandlerFunc(
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
	dataHubProcedure(mux, "GetArticleByID", getArticleByIDHandler)

	// recap-worker-alt-backend.json: "a batch tags request by article ids"
	// recap-worker fetches tags for a batch of article ids to enrich the
	// recap payload. Connect-RPC, JSON wire format, camelCase keys.
	batchTagsHandler := func(w http.ResponseWriter, r *http.Request) {
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
	}
	dataHubProcedure(mux, "BatchGetTagsByArticleIDs", batchTagsHandler)

	// Transitional shims, same rationale as the recap-articles pair above:
	// the recap-worker (58648b8) and search-indexer (64d94ed / caed48f)
	// currently deployed to production publish pacts against the retired
	// services.backend.v1.BackendInternalService paths, and the broker's
	// DeployedOrReleased selector keeps selecting those pacts until a
	// post-split version of each consumer is recorded as deployed. Without
	// these routes every release that includes alt-backend fails provider
	// verification (alt-deploy run 30666223031) and the pipeline deadlocks —
	// record-deployment only happens after a successful deploy. The wire
	// shapes are identical to the DataHubService procedures, so the same
	// stubs serve both names. Remove once the deployed recap-worker and
	// search-indexer versions advance past 4b4f07230.
	for procedure, handler := range map[string]http.Handler{
		"ListRecapArticles":         http.HandlerFunc(recapArticlesHandler),
		"BatchGetTagsByArticleIDs":  http.HandlerFunc(batchTagsHandler),
		"GetLatestArticleTimestamp": latestArticleTimestampHandler,
		"ListArticlesWithTags":      listArticlesWithTagsHandler,
		"GetArticleByID":            getArticleByIDHandler,
	} {
		mux.Handle("/services.backend.v1.BackendInternalService/"+procedure, handler)
	}

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

	mountWave3Procedures(mux)

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

// TestVerifyAltBackendDataHubContract and TestVerifyAltHarvesterDataHubContract
// verify the Wave 3 in-family capabilities (ADR-000954 D3, catalog §2.A /
// §2.D / §2.E / §2.L / §2.O).
//
// These two consumers are unusual only in that they are built from this same
// Go module. That is the reason to pin them, not a reason to skip them: a
// shared module lets a message and a handler change together and still
// compile, while the three binaries ship as three containers and roll
// independently. "It builds" and "the deployed provider answers what the
// deployed consumer sends" are different claims, and only the second one is
// what breaks in production.
//
// Two pacticipants rather than one because the two binaries call disjoint
// halves of the surface — the harvester drives the outbox and the retention
// jobs, the backend drives the article-serving reads. Publishing both under
// one name would let a harvester-only break verify green.
func TestVerifyAltBackendDataHubContract(t *testing.T) {
	verifyConsumer(t, "alt-backend", dataHubProviderName,
		filepath.Join(altBackendPactDir, altBackendDataHubPactFile),
		noopStates(
			"alt-data-hub has a scraped article head",
			"alt-data-hub has no article head for the article",
			"alt-data-hub has og images for the articles",
			"alt-data-hub has a live image proxy cache entry",
			"alt-data-hub has no live image proxy cache entry",
			"alt-data-hub accepts image proxy cache writes",
			"alt-data-hub has a scraping domain",
			"alt-data-hub has no scraping domain for the host",
			"alt-data-hub accepts declined domain writes",
			"alt-data-hub has a declined domain for the user",
			"alt-data-hub has subscribers for the feed link",
			"alt-data-hub has the article for the user",

			// Wave 3 batch 2 (catalog §2.B / §2.C / §2.N).
			"alt-data-hub accepts article archives",
			"alt-data-hub accepts article head writes",
			"alt-data-hub has no article for the url",
			"alt-data-hub has articles for the user",
			"alt-data-hub has articles for the feed",
			"alt-data-hub has historic articles to replay",
			"alt-data-hub has summary versions to replay",

			// Wave 3 batch 3 (catalog §2.F / §2.G / §2.H).
			"alt-data-hub accepts feed link registrations",
			"alt-data-hub accepts bulk feed link registrations",
			"alt-data-hub has feed links",
			"alt-data-hub has a feed link that has never been polled",
			"alt-data-hub has no feed link for the url",
			"alt-data-hub has unread feeds for the user",
			"alt-data-hub has favorite feeds past the og image retention window",
			"alt-data-hub has feeds",
			"alt-data-hub has no feeds",
			"alt-data-hub has feeds for the feed link",
			"alt-data-hub has no summary for the article",
			"alt-data-hub has a summary for the article url",
			"alt-data-hub has feeds matching the title query for the user",
			"alt-data-hub has no tagged feeds",
			"alt-data-hub has feeds for the articles",
			"alt-data-hub has imported inoreader articles for the urls",

			// Wave 3 batch 4 (catalog §2.I / §2.J).
			"alt-data-hub has a feed at the url",
			"alt-data-hub has no feed at the url",
			"alt-data-hub has a feed for the article url",
			"alt-data-hub has read marks for the user",
			"alt-data-hub has subscriptions for the user",
			"alt-data-hub accepts subscription writes",
			"alt-data-hub has no tags for the article",
			"alt-data-hub has tags for the feed",
			"alt-data-hub accepts article tag writes",
			"alt-data-hub has tag cooccurrences",
			"alt-data-hub has tags matching the prefix",
			"alt-data-hub has tagged articles for the user in the window",

			// Wave 3 batch 5 (catalog §2.K / §2.M).
			"alt-data-hub accepts summary version appends",
			"alt-data-hub has an earlier summary version for the article",
			"alt-data-hub has no earlier summary version for the article",
			"alt-data-hub has a superseded summary version",
			"alt-data-hub has a current summary version for the article",
			"alt-data-hub accepts tag set version appends",
			"alt-data-hub has an earlier tag set version for the article",
			"alt-data-hub has a tag set version",
			"alt-data-hub accepts article summary writes",
			// "has feeds" and "has articles for the user" are already declared
			// by batches 2 and 3 above; the §2.M counts reuse them rather than
			// inventing a second name for the same precondition.
			"alt-data-hub has summarized articles for the user",
			"alt-data-hub has unsummarized articles for the user",
			"alt-data-hub has unread feeds for the user since the bound",
			"alt-data-hub has trend data for the user",
			"alt-data-hub has read state for the user",

			// Wave 3 batch 6 (catalog §2.J / §2.C) — the last two.
			"alt-data-hub has articles carrying the feed tag",
			"alt-data-hub has articles carrying the tag name across feeds",
			"alt-data-hub has the article",
			"alt-data-hub has no article with that id",
		))
}

func TestVerifyAltHarvesterDataHubContract(t *testing.T) {
	verifyConsumer(t, "alt-harvester", dataHubProviderName,
		filepath.Join(altBackendPactDir, altHarvesterDataHubPactFile),
		noopStates(
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
		))
}

// mountWave3Procedures adds the capabilities ADR-000954 Wave 3 moved off the
// direct alt_db path (catalog §2.A / §2.D / §2.E / §2.L / §2.O).
//
// Two shapes appear repeatedly and both are protoJSON rules rather than
// choices made here:
//
//   - 64-bit integers are JSON strings. prunedCount, purgedCount,
//     evictedCount and sizeBytes are int64 in the proto, so "12" is correct
//     and 12 is not.
//   - An unset optional message is an absent key, not null. The three "miss"
//     interactions — no article head, no cache entry, no scraping domain —
//     answer `{}`, because the consumers read that absence as "never
//     recorded" and behave differently from "recorded as empty".
func mountWave3Procedures(mux *http.ServeMux) {
	// ---- §2.A Outbox -------------------------------------------------------
	dataHubProcedure(mux, "ClaimOutboxBatch", jsonPost(map[string]interface{}{
		"events": []map[string]interface{}{
			{
				"id":        "8f14e45f-ceea-467a-9d0c-1a2b3c4d5e6f",
				"eventType": "ARTICLE_UPSERT",
				"payload":   "eyJhcnRpY2xlX2lkIjoiYTEifQ==",
				// Already claimed. A stub that answered PENDING here would
				// verify a provider that had lost the point of the capability.
				"status":    "OUTBOX_EVENT_STATUS_PROCESSING",
				"createdAt": "2026-07-31T00:00:00Z",
			},
		},
	}))
	dataHubProcedure(mux, "MarkOutboxProcessed", jsonPost(map[string]interface{}{}))
	dataHubProcedure(mux, "ReleaseOutboxEvent", jsonPost(map[string]interface{}{}))
	dataHubProcedure(mux, "PruneOutboxEvents", jsonPost(map[string]interface{}{
		"prunedCount": "12",
	}))

	// ---- §2.D OG image / article_heads -------------------------------------
	//
	// GetArticleHead is the one procedure here whose two answers are both
	// meaningful, so the stub branches on the request rather than always
	// returning a head: fetch_article_usecase re-scrapes on the absent one.
	dataHubProcedure(mux, "GetArticleHead", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			ArticleID string `json:"articleId"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)

		w.Header().Set("Content-Type", "application/json")
		if req.ArticleID == "00000000-0000-4000-8000-000000000000" {
			_, _ = w.Write([]byte(`{}`))
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"head": map[string]interface{}{
				"id":         "11111111-2222-3333-4444-555555555555",
				"articleId":  "6f1a2f7e-1f1e-4c2a-9a3e-5b6c7d8e9f01",
				"headHtml":   "<head><title>x</title></head>",
				"ogImageUrl": "https://cdn.example.com/og.png",
			},
		})
	})
	dataHubProcedure(mux, "BatchGetOgImageURLs", jsonPost(map[string]interface{}{
		"ogImageUrls": map[string]string{
			"6f1a2f7e-1f1e-4c2a-9a3e-5b6c7d8e9f01": "https://cdn.example.com/og.png",
		},
	}))
	dataHubProcedure(mux, "ListFeedsMissingOgImage", jsonPost(map[string]interface{}{
		"candidates": []map[string]interface{}{
			{"articleId": "6f1a2f7e-1f1e-4c2a-9a3e-5b6c7d8e9f01", "url": "https://example.com/post"},
		},
	}))
	dataHubProcedure(mux, "ListUnwarmedOgImageURLs", jsonPost(map[string]interface{}{
		"urls": []string{"https://cdn.example.com/og.png"},
	}))
	dataHubProcedure(mux, "PurgeExpiredArticleHeads", jsonPost(map[string]interface{}{
		"purgedCount": "7",
	}))

	// ---- §2.E Image proxy cache -------------------------------------------
	dataHubProcedure(mux, "GetImageProxyCache", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			URLHash string `json:"urlHash"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)

		w.Header().Set("Content-Type", "application/json")
		if req.URLHash == "missing" {
			_, _ = w.Write([]byte(`{}`))
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"entry": map[string]interface{}{
				"urlHash":     "abc123",
				"originalUrl": "https://cdn.example.com/og.png",
				"data":        "UklGRg==",
				"contentType": "image/webp",
				"width":       600,
				"height":      315,
				"sizeBytes":   "4",
				"expiresAt":   "2026-08-07T00:00:00Z",
			},
		})
	})
	dataHubProcedure(mux, "PutImageProxyCache", jsonPost(map[string]interface{}{}))
	dataHubProcedure(mux, "EvictExpiredImageProxyCache", jsonPost(map[string]interface{}{
		"evictedCount": "5",
	}))
	dataHubProcedure(mux, "PurgeImageProxyCacheOlderThan", jsonPost(map[string]interface{}{
		"purgedCount": "3",
	}))

	// ---- §2.L Scraping policy ---------------------------------------------
	scrapingDomain := map[string]interface{}{
		"id":                  "2b1c3d4e-5f60-4711-8899-aabbccddeeff",
		"domain":              "example.com",
		"scheme":              "https",
		"allowFetchBody":      true,
		"forceRespectRobots":  true,
		"robotsCrawlDelaySec": 5,
		"robotsDisallowPaths": []string{"/private"},
		"createdAt":           "2026-07-31T00:00:00Z",
		"updatedAt":           "2026-07-31T00:00:00Z",
	}
	dataHubProcedure(mux, "GetScrapingDomainByDomain", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			Domain string `json:"domain"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)

		w.Header().Set("Content-Type", "application/json")
		if req.Domain == "unknown.example" {
			_, _ = w.Write([]byte(`{}`))
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"scrapingDomain": scrapingDomain})
	})
	dataHubProcedure(mux, "GetScrapingDomainByID", jsonPost(map[string]interface{}{
		"scrapingDomain": scrapingDomain,
	}))
	dataHubProcedure(mux, "SaveScrapingDomain", jsonPost(map[string]interface{}{
		"scrapingDomain": scrapingDomain,
	}))
	dataHubProcedure(mux, "ListScrapingDomains", jsonPost(map[string]interface{}{
		"scrapingDomains": []map[string]interface{}{scrapingDomain},
	}))
	dataHubProcedure(mux, "UpdateScrapingDomainPolicy", jsonPost(map[string]interface{}{}))
	dataHubProcedure(mux, "SaveDeclinedDomain", jsonPost(map[string]interface{}{}))
	dataHubProcedure(mux, "IsDomainDeclined", jsonPost(map[string]interface{}{
		"declined": true,
	}))

	// ---- §2.O Automatic full-text fetch groundwork -------------------------
	dataHubProcedure(mux, "ListSubscribedUserIDsByFeedLinkID", jsonPost(map[string]interface{}{
		"userIds": []string{"11111111-2222-3333-4444-555555555555"},
	}))
	dataHubProcedure(mux, "CheckArticleExistsByURLForUser", jsonPost(map[string]interface{}{
		"exists":    true,
		"articleId": "6f1a2f7e-1f1e-4c2a-9a3e-5b6c7d8e9f01",
	}))

	mountWave3Batch2Procedures(mux)
}

// mountWave3Batch2Procedures adds the article capabilities ADR-000954 Wave 3
// batch 2 moved off the direct alt_db path (catalog §2.B / §2.C / §2.N).
//
// Two shapes recur here and both are contract, not convenience:
//
//   - "no such article" is an absent key, not an empty object. GetArticleByURL
//     answers `{}` for an unarchived URL, and the fetch usecase reads that
//     absence as "go get the page". An empty ArticleContent would read as an
//     article with no body and stop it fetching.
//   - The backfill counts are JSON numbers because they are int32, while the
//     retention counts above are JSON strings because those are int64. That is
//     protojson's rule, not a per-procedure choice.
func mountWave3Batch2Procedures(mux *http.ServeMux) {
	const (
		stubArticleID = "6f1a2f7e-1f1e-4c2a-9a3e-5b6c7d8e9f01"
		stubUserID    = "11111111-2222-3333-4444-555555555555"
		stubFeedID    = "33333333-4444-5555-6666-777777777777"
		stubURL       = "https://example.com/post"
		stubTimestamp = "2026-07-31T00:00:00Z"
	)

	articleContent := map[string]interface{}{
		"id":      stubArticleID,
		"title":   "Example",
		"content": "body text",
		"url":     stubURL,
		"feedId":  stubFeedID,
	}
	userArticle := map[string]interface{}{
		"id":          stubArticleID,
		"feedId":      stubFeedID,
		"title":       "Example",
		"content":     "body text",
		"url":         stubURL,
		"tags":        []string{"go"},
		"publishedAt": stubTimestamp,
		"createdAt":   stubTimestamp,
	}

	// ---- §2.B Article writes ----------------------------------------------
	dataHubProcedure(mux, "ArchiveArticle", jsonPost(map[string]interface{}{
		"articleId": stubArticleID,
		"created":   true,
	}))
	dataHubProcedure(mux, "SaveArticleHead", jsonPost(map[string]interface{}{}))

	// ---- §2.C Article reads ------------------------------------------------
	//
	// GetArticleByURL branches for the same reason GetArticleHead does above:
	// both of its answers mean something, and a stub that always found an
	// article would verify a provider that had lost the distinction.
	dataHubProcedure(mux, "GetArticleByURL", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			URL string `json:"url"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)

		w.Header().Set("Content-Type", "application/json")
		if req.URL != stubURL {
			_, _ = w.Write([]byte(`{}`))
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"article": articleContent})
	})
	dataHubProcedure(mux, "BatchGetArticlesByURLs", jsonPost(map[string]interface{}{
		"articles": map[string]interface{}{stubURL: articleContent},
	}))
	dataHubProcedure(mux, "GetArticleContentByID", jsonPost(map[string]interface{}{
		"article": articleContent,
	}))
	dataHubProcedure(mux, "ListArticlesCursor", jsonPost(map[string]interface{}{
		"articles": []map[string]interface{}{userArticle},
	}))
	dataHubProcedure(mux, "ListArticleIDsCursor", jsonPost(map[string]interface{}{
		"articleIds": []string{stubArticleID},
	}))
	dataHubProcedure(mux, "BatchGetArticlesByIDs", jsonPost(map[string]interface{}{
		"articles": []map[string]interface{}{userArticle},
	}))
	dataHubProcedure(mux, "GetLatestArticleByFeedID", jsonPost(map[string]interface{}{
		"article": articleContent,
	}))
	dataHubProcedure(mux, "LookupArticleURL", jsonPost(map[string]interface{}{
		"url": stubURL,
	}))

	// ---- §2.N Knowledge backfill -------------------------------------------
	dataHubProcedure(mux, "CountBackfillArticles", jsonPost(map[string]interface{}{
		"count": 4242,
	}))
	dataHubProcedure(mux, "ListBackfillArticles", jsonPost(map[string]interface{}{
		"articles": []map[string]interface{}{{
			"articleId":   stubArticleID,
			"userId":      stubUserID,
			"createdAt":   "2026-01-02T03:04:05Z",
			"publishedAt": "2026-01-02T03:04:05Z",
			"title":       "Example",
			"url":         stubURL,
		}},
	}))
	dataHubProcedure(mux, "CountBackfillSummaryTitles", jsonPost(map[string]interface{}{
		"count": 7,
	}))
	dataHubProcedure(mux, "ListBackfillSummaryTitles", jsonPost(map[string]interface{}{
		"entries": []map[string]interface{}{{
			"summaryVersionId": "22222222-3333-4444-5555-666666666666",
			"articleId":        stubArticleID,
			"userId":           stubUserID,
			"tenantId":         stubUserID,
			"title":            "Example",
			"generatedAt":      "2026-03-04T05:06:07Z",
		}},
	}))

	mountWave3Batch3Procedures(mux)
	mountWave3Batch4Procedures(mux)
	mountWave3Batch5Procedures(mux)
	mountWave3Batch6Procedures(mux)
}

// mountWave3Batch6Procedures adds the two capabilities that close Wave 3: the
// Tag Trail's paged reads (§2.J) and the recall rail's article fallback
// (§2.C).
//
// GetArticleTitleAndLink branches on the article id for the same reason
// GetArticleByURL and GetArticleHead do above: both of its answers mean
// something. `found` is the entire point of the message — proto3 cannot tell
// an unset title from an empty one — so a stub that always answered found
// would verify a provider that had dropped the distinction and would let the
// recall rail render every deleted article as a blank row.
func mountWave3Batch6Procedures(mux *http.ServeMux) {
	const (
		stubArticleID    = "6f1a2f7e-1f1e-4c2a-9a3e-5b6c7d8e9f01"
		stubFeedID       = "33333333-4444-5555-6666-777777777777"
		missingArticleID = "00000000-0000-4000-8000-000000000000"
	)

	trailArticle := map[string]interface{}{
		"id":          stubArticleID,
		"title":       "Tagged article",
		"url":         "https://example.com/tagged",
		"publishedAt": "2026-07-30T08:00:00Z",
		"feedId":      stubFeedID,
		"feedTitle":   "Example Feed",
	}

	// ---- §2.J Tag Trail paging ---------------------------------------------
	//
	// The by-id stub branches on the cursor, because the interaction that
	// records the first page expects an empty body: a stub answering the same
	// page either way would verify a provider that ignored the cursor, which
	// is the failure that pins the Trail to its first screen.
	dataHubProcedure(mux, "ListArticlesByTagID", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			Cursor string `json:"cursor"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)

		w.Header().Set("Content-Type", "application/json")
		if req.Cursor == "" {
			_, _ = w.Write([]byte(`{}`))
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"articles": []map[string]interface{}{trailArticle},
		})
	})
	// The cross-feed half answers a row with no feed, which is legal and is
	// what the consumer asserts: the by-name query does not always resolve one.
	dataHubProcedure(mux, "ListArticlesByTagName", jsonPost(map[string]interface{}{
		"articles": []map[string]interface{}{{
			"id":          stubArticleID,
			"title":       "Cross-feed article",
			"url":         "https://example.com/cross",
			"publishedAt": "2026-07-30T08:00:00Z",
		}},
	}))

	// ---- §2.C Article reference for the recall rail -------------------------
	dataHubProcedure(mux, "GetArticleTitleAndLink", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			ArticleID string `json:"articleId"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)

		w.Header().Set("Content-Type", "application/json")
		if req.ArticleID == missingArticleID {
			// No found key at all: protojson omits a false bool, and the
			// consumer must read that absence as "nothing to render".
			_, _ = w.Write([]byte(`{}`))
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"found":       true,
			"title":       "Recalled article",
			"url":         "https://example.com/recalled",
			"publishedAt": "2026-06-01T07:30:00Z",
		})
	})
}

// mountWave3Batch3Procedures adds the feed and feed-link capabilities
// ADR-000954 Wave 3 batch 3 moved off the direct alt_db path (catalog §2.F /
// §2.G / §2.H).
//
// Three procedures branch on the request rather than answering one canned
// body, and each branch is a distinction a consumer acts on:
//
//   - RecordFeedLinkFailure answers a still-active row for one URL and a
//     disabled one for another, so the merged capability (catalog §4-4) is
//     verified in both of its outcomes. A stub that always reported
//     disabledNow would let a provider that disabled on the first failure
//     verify green.
//   - GetSingleFeed and GetRandomFeed answer {} for the empty table, because
//     the consumers read that absence as "nothing yet" and render it.
//
// The summary reads answer {} for the miss for the same reason: an
// unsummarised article is not a fault.
func mountWave3Batch3Procedures(mux *http.ServeMux) {
	const (
		stubFeedLinkID  = "a1b2c3d4-1111-4111-8111-111111111111"
		stubFeedRowID   = "b2c3d4e5-2222-4222-8222-222222222222"
		stubFeedLinkURL = "https://example.com/feed.xml"
		stubDeadFeedURL = "https://dead.example.com/feed.xml"
		// Repeated from the batch 2 mount rather than hoisted to package
		// scope: these stubs are fixtures for one set of interactions, and a
		// shared constant would make a later edit for one batch silently
		// change the other batch's expectations.
		stubArticleID = "6f1a2f7e-1f1e-4c2a-9a3e-5b6c7d8e9f01"
		stubURL       = "https://example.com/post"
		stubTimestamp = "2026-07-31T00:00:00Z"
	)

	feedLink := map[string]interface{}{"id": stubFeedLinkID, "url": stubFeedLinkURL}
	feedRow := map[string]interface{}{
		"id":          stubFeedRowID,
		"title":       "Example Post",
		"description": "body",
		"websiteUrl":  stubURL,
		"pubDate":     stubTimestamp,
		"createdAt":   "2026-07-31T09:00:00Z",
		"updatedAt":   "2026-07-31T09:00:00Z",
		"articleId":   stubArticleID,
		"ogImageUrl":  "https://cdn.example.com/og.png",
	}
	// The retired-og-image row omits ogImageUrl and articleId entirely rather
	// than sending empty strings — the absence is what the placeholder
	// renderer keys on.
	agedFeedRow := map[string]interface{}{
		"id":         stubFeedRowID,
		"title":      "Older Post",
		"websiteUrl": "https://example.com/older",
		"createdAt":  "2026-06-01T09:00:00Z",
	}

	// ---- §2.F Feed links ---------------------------------------------------
	dataHubProcedure(mux, "RegisterFeedLink", jsonPost(map[string]interface{}{}))
	dataHubProcedure(mux, "BulkRegisterFeedLinks", jsonPost(map[string]interface{}{
		"registered": 1,
		"skipped":    1,
	}))
	dataHubProcedure(mux, "ListFeedLinks", jsonPost(map[string]interface{}{
		"feedLinks": []map[string]interface{}{feedLink},
	}))
	dataHubProcedure(mux, "ListFeedLinksWithHealth", jsonPost(map[string]interface{}{
		// No availability key: this is the never-polled link, which the admin
		// screen must classify as unknown rather than healthy.
		"feedLinks": []map[string]interface{}{{"feedLink": feedLink}},
	}))
	dataHubProcedure(mux, "DeleteFeedLink", jsonPost(map[string]interface{}{}))
	dataHubProcedure(mux, "ResolveFeedLinkIDByURL", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			FeedURL string `json:"feedUrl"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)

		w.Header().Set("Content-Type", "application/json")
		if req.FeedURL != stubFeedLinkURL {
			_, _ = w.Write([]byte(`{}`))
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"feedLinkId": stubFeedLinkID})
	})
	dataHubProcedure(mux, "ListFeedLinkDomains", jsonPost(map[string]interface{}{
		"domains": []map[string]interface{}{{"domain": "example.com", "scheme": "https"}},
	}))
	dataHubProcedure(mux, "ListRSSFeedURLs", jsonPost(map[string]interface{}{
		"feedLinks": []map[string]interface{}{feedLink},
	}))
	dataHubProcedure(mux, "ListFeedLinksForExport", jsonPost(map[string]interface{}{
		"entries": []map[string]interface{}{{"url": stubFeedLinkURL, "title": "Example Blog"}},
	}))

	// ---- §2.G Feed link availability ---------------------------------------
	dataHubProcedure(mux, "RecordFeedLinkFailure", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			FeedURL string `json:"feedUrl"`
			Reason  string `json:"reason"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)

		w.Header().Set("Content-Type", "application/json")
		if req.FeedURL == stubDeadFeedURL {
			// isActive is omitted: false is an absent key under protojson, and
			// the consumer must read the absence as "disabled".
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"availability": map[string]interface{}{
					"feedLinkId":          stubFeedLinkID,
					"consecutiveFailures": 5,
					"lastFailureReason":   req.Reason,
				},
				"disabledNow": true,
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"availability": map[string]interface{}{
				"feedLinkId":          stubFeedLinkID,
				"isActive":            true,
				"consecutiveFailures": 3,
				"lastFailureAt":       "2026-07-31T10:00:00Z",
				"lastFailureReason":   req.Reason,
			},
		})
	})
	dataHubProcedure(mux, "ResetFeedLinkFailures", jsonPost(map[string]interface{}{}))

	// ---- §2.H Feeds --------------------------------------------------------
	dataHubProcedure(mux, "RegisterFeeds", jsonPost(map[string]interface{}{
		"results": []map[string]interface{}{{"feedId": stubFeedRowID, "created": true}},
	}))
	dataHubProcedure(mux, "ListFeedsCursor", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			Scope string `json:"scope"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)

		w.Header().Set("Content-Type", "application/json")
		row := feedRow
		if req.Scope == "FEED_SCOPE_FAVORITE" {
			row = agedFeedRow
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"feeds": []map[string]interface{}{row},
		})
	})
	dataHubProcedure(mux, "ListFeedsPage", jsonPost(map[string]interface{}{
		"feeds": []map[string]interface{}{feedRow},
	}))
	dataHubProcedure(mux, "ListFeedsLimit", jsonPost(map[string]interface{}{
		"feeds": []map[string]interface{}{feedRow},
	}))
	dataHubProcedure(mux, "GetSingleFeed", jsonPost(map[string]interface{}{}))
	dataHubProcedure(mux, "ListFeedsByFeedLinkID", jsonPost(map[string]interface{}{
		"feeds": []map[string]interface{}{feedRow},
	}))
	dataHubProcedure(mux, "GetFeedSummary", jsonPost(map[string]interface{}{
		"summary": map[string]interface{}{"summary": "要約テキスト"},
	}))
	dataHubProcedure(mux, "GetArticleSummaryByArticleID", jsonPost(map[string]interface{}{}))
	dataHubProcedure(mux, "SearchFeedsByTitle", jsonPost(map[string]interface{}{
		"feeds": []map[string]interface{}{{
			"title":      "Example Post",
			"websiteUrl": stubURL,
			"pubDate":    stubTimestamp,
		}},
	}))
	dataHubProcedure(mux, "GetRandomFeed", jsonPost(map[string]interface{}{}))
	dataHubProcedure(mux, "GetFeedURLsByArticleIDs", jsonPost(map[string]interface{}{
		"pairs": []map[string]interface{}{{
			"feedId":       stubFeedRowID,
			"articleId":    stubArticleID,
			"url":          stubURL,
			"feedTitle":    "Example Blog",
			"articleTitle": "Example Post",
		}},
	}))
	dataHubProcedure(mux, "BatchGetFeedTitlesByIDs", jsonPost(map[string]interface{}{
		"titles": map[string]interface{}{stubFeedRowID: "Example Blog"},
	}))
	dataHubProcedure(mux, "GetInoreaderSummariesByURLs", jsonPost(map[string]interface{}{
		"summaries": []map[string]interface{}{{
			"articleUrl":  stubURL,
			"title":       "Example Post",
			"content":     "<p>body</p>",
			"contentType": "html",
			"publishedAt": stubTimestamp,
			"fetchedAt":   "2026-07-31T01:00:00Z",
			"inoreaderId": "tag:google.com,2005:reader/item/0001",
		}},
	}))
}

// mountWave3Batch4Procedures adds the per-user feed state and the tag reads
// ADR-000954 Wave 3 batch 4 moved off the direct alt_db path (capability
// catalog §2.I / §2.J).
//
// Three of the writes branch on the URL, and that branch is the batch's whole
// argument. Capability catalog §4-5 found MarkFeedRead and MarkArticleRead
// answering "there is no such feed" two different ways — one from a preceding
// SELECT's pgx.ErrNoRows, one from an upsert's RowsAffected() — and the
// favourite writes raising a third sentinel for the same situation. A stub
// that always answered 200 would verify a provider that had quietly kept them
// apart, because the consumer's whole branch is on the Connect code.
func mountWave3Batch4Procedures(mux *http.ServeMux) {
	const (
		stubUser        = "3f2504e0-4f89-41d3-9a0c-0305e82c3301"
		stubFeedRow     = "b2c3d4e5-2222-4222-8222-222222222222"
		stubLink        = "a1b2c3d4-1111-4111-8111-111111111111"
		stubMissingFeed = "https://gone.example.com/feed.xml"
	)

	// ---- §2.I Read state ---------------------------------------------------

	// notFoundForMissingURL answers 404 when the request names the URL the
	// consumer uses for "nothing here", and 200 otherwise. urlField is the
	// protoJSON name of whichever URL field the procedure takes — the two read
	// marks disagree about that and agree about everything else, which is the
	// state §4-5 left them in.
	notFoundForMissingURL := func(urlField string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			var req map[string]interface{}
			_ = json.NewDecoder(r.Body).Decode(&req)

			w.Header().Set("Content-Type", "application/json")
			if url, _ := req[urlField].(string); url == stubMissingFeed {
				w.WriteHeader(http.StatusNotFound)
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"code":    "not_found",
					"message": "feed not found",
				})
				return
			}
			_, _ = w.Write([]byte(`{}`))
		}
	}

	dataHubProcedure(mux, "MarkFeedRead", notFoundForMissingURL("feedUrl"))
	dataHubProcedure(mux, "MarkArticleRead", notFoundForMissingURL("articleUrl"))
	dataHubProcedure(mux, "AddFavoriteFeed", notFoundForMissingURL("feedUrl"))
	dataHubProcedure(mux, "RemoveFavoriteFeed", notFoundForMissingURL("feedUrl"))

	dataHubProcedure(mux, "GetReadFeedIDs", jsonPost(map[string]interface{}{
		"readFeedIds": []string{stubFeedRow},
	}))
	dataHubProcedure(mux, "GetAllReadFeedIDs", jsonPost(map[string]interface{}{
		"readFeedIds": []string{stubFeedRow},
	}))
	dataHubProcedure(mux, "GetUserSubscribedFeedLinkIDs", jsonPost(map[string]interface{}{
		"feedLinkIds": []string{stubLink},
	}))
	dataHubProcedure(mux, "ListSubscriptions", jsonPost(map[string]interface{}{
		"subscriptions": []map[string]interface{}{{
			"feedLinkId":   stubLink,
			"url":          "https://example.com/feed.xml",
			"isSubscribed": true,
			"subscribedAt": "2026-05-01T12:00:00Z",
		}},
	}))
	dataHubProcedure(mux, "Subscribe", jsonPost(map[string]interface{}{}))
	dataHubProcedure(mux, "Unsubscribe", jsonPost(map[string]interface{}{}))

	// ---- §2.J Tag reads ----------------------------------------------------

	// GetArticleTags answers {} — the empty repeated field. An untagged
	// article is not a 404 here, because the consumer reads emptiness as
	// "generate some" and a NotFound would suppress that path entirely.
	dataHubProcedure(mux, "GetArticleTags", jsonPost(map[string]interface{}{}))
	dataHubProcedure(mux, "GetFeedTags", jsonPost(map[string]interface{}{
		"tags": []map[string]interface{}{{
			"id":        "11111111-2222-3333-4444-555555555555",
			"tagName":   "AI",
			"createdAt": "2026-07-31T09:00:00Z",
		}},
	}))
	dataHubProcedure(mux, "GetTagCooccurrences", jsonPost(map[string]interface{}{
		"cooccurrences": []map[string]interface{}{{
			"tagNameA":    "AI",
			"tagNameB":    "Go",
			"sharedCount": 4,
		}},
	}))
	dataHubProcedure(mux, "SearchTagsByPrefix", jsonPost(map[string]interface{}{
		"hits": []map[string]interface{}{{"tagName": "AI", "articleCount": 42}},
	}))
	dataHubProcedure(mux, "GetTagArticleCounts", jsonPost(map[string]interface{}{
		"counts": []map[string]interface{}{{"tagName": "AI", "articleCount": 10}},
	}))
}

// mountWave3Batch5Procedures adds the versioned artifacts and the dashboard
// statistics ADR-000954 Wave 3 batch 5 moved off the direct alt_db path
// (capability catalog §2.K / §2.M).
//
// Two shapes here are the batch's argument rather than stub filler.
//
// MarkSummaryVersionSuperseded branches on the article id: one answers with a
// previousVersion, the other with `{}`. That pair is the only thing a consumer
// can observe of a transaction that takes a per-article
// pg_advisory_xact_lock, reads the current version and updates it — and the
// caller emits SummarySuperseded on exactly that distinction. A stub that
// always returned a previous version would verify a provider that announced
// the replacement of summaries that never existed.
//
// GetTrendStats answers a granularity the consumer did not ask for. The window
// selects the date_trunc unit inside the query, so "7d implies daily" is the
// provider's to state; a stub that echoed the request would hide a provider
// that had stopped stating it.
func mountWave3Batch5Procedures(mux *http.ServeMux) {
	const (
		stubVersionUser   = "9f8e7d6c-5b4a-4392-8281-706f5e4d3c2b"
		stubVersionArtID  = "7a2b3c4d-5e6f-4a1b-8c2d-3e4f5a6b7c8d"
		stubSummaryVerID  = "11111111-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
		stubPrevSummaryID = "22222222-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
		stubTagSetVerID   = "33333333-cccc-4ccc-8ccc-cccccccccccc"
		stubPrevTagSetID  = "44444444-dddd-4ddd-8ddd-dddddddddddd"
		stubStatsFeedID   = "5c6d7e8f-9a0b-4c1d-8e2f-3a4b5c6d7e8f"
		// The article the consumer uses for "this is a first version". Its all
		// zeroes make it a deliberate sentinel rather than a value that could
		// collide with a real id.
		stubFirstVersionArticle = "00000000-0000-4000-8000-000000000000"
		// base64 of [{"name":"AI"}] — protoJSON renders the jsonb column's
		// bytes field this way, and the encoding is part of the contract.
		stubTagsJSON = "W3sibmFtZSI6IkFJIn1d"
	)

	// ---- §2.K Versioned artifacts ------------------------------------------

	// Append-only writes report nothing: the id was the caller's, so there is
	// no server-assigned value to hand back.
	dataHubProcedure(mux, "CreateSummaryVersion", jsonPost(map[string]interface{}{}))
	dataHubProcedure(mux, "CreateTagSetVersion", jsonPost(map[string]interface{}{}))

	// previousOrAbsent answers `{}` for the sentinel article and a previous
	// version for anything else. The absent case is the load-bearing one — see
	// the function comment.
	previousOrAbsent := func(previous map[string]interface{}) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			var req struct {
				ArticleID string `json:"articleId"`
			}
			_ = json.NewDecoder(r.Body).Decode(&req)

			w.Header().Set("Content-Type", "application/json")
			if req.ArticleID == stubFirstVersionArticle {
				_, _ = w.Write([]byte(`{}`))
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"previousVersion": previous})
		}
	}

	dataHubProcedure(mux, "MarkSummaryVersionSuperseded", previousOrAbsent(map[string]interface{}{
		"summaryVersionId": stubPrevSummaryID,
		"articleId":        stubVersionArtID,
		"userId":           stubVersionUser,
		"generatedAt":      "2026-07-30T09:00:00Z",
		"model":            "pre-processor",
		"summaryText":      "the older summary",
	}))
	dataHubProcedure(mux, "MarkTagSetVersionSuperseded", previousOrAbsent(map[string]interface{}{
		"tagSetVersionId": stubPrevTagSetID,
		"articleId":       stubVersionArtID,
		"userId":          stubVersionUser,
		"generatedAt":     "2026-07-30T09:00:00Z",
		"generator":       "tag-generator",
		"tagsJson":        stubTagsJSON,
	}))

	// GetSummaryVersionByID answers a version that carries supersededBy. That
	// is the reproject-safe read stated as data: an old event resolves to the
	// version it named even though something has replaced it since.
	dataHubProcedure(mux, "GetSummaryVersionByID", jsonPost(map[string]interface{}{
		"version": map[string]interface{}{
			"summaryVersionId": stubPrevSummaryID,
			"articleId":        stubVersionArtID,
			"userId":           stubVersionUser,
			"generatedAt":      "2026-07-30T09:00:00Z",
			"model":            "pre-processor",
			"summaryText":      "the older summary",
			"supersededBy":     stubSummaryVerID,
		},
	}))
	// GetLatestSummaryVersion answers one without it, because "latest" means
	// exactly "nothing has replaced it".
	dataHubProcedure(mux, "GetLatestSummaryVersion", jsonPost(map[string]interface{}{
		"version": map[string]interface{}{
			"summaryVersionId": stubSummaryVerID,
			"articleId":        stubVersionArtID,
			"userId":           stubVersionUser,
			"generatedAt":      "2026-07-31T09:00:00Z",
			"model":            "stream-summarize",
			"summaryText":      "a summary",
		},
	}))
	dataHubProcedure(mux, "GetTagSetVersionByID", jsonPost(map[string]interface{}{
		"version": map[string]interface{}{
			"tagSetVersionId": stubTagSetVerID,
			"articleId":       stubVersionArtID,
			"userId":          stubVersionUser,
			"generatedAt":     "2026-07-31T09:00:00Z",
			"generator":       "tag-generator",
			"tagsJson":        stubTagsJSON,
		},
	}))

	// ---- §2.M Statistics / dashboard ---------------------------------------

	dataHubProcedure(mux, "GetFeedAmount", jsonPost(map[string]interface{}{"count": 42}))
	dataHubProcedure(mux, "GetTotalArticlesCount", jsonPost(map[string]interface{}{"count": 120}))
	dataHubProcedure(mux, "GetSummarizedArticlesCount", jsonPost(map[string]interface{}{"count": 80}))
	dataHubProcedure(mux, "GetUnsummarizedArticlesCount", jsonPost(map[string]interface{}{"count": 40}))
	dataHubProcedure(mux, "GetTodayUnreadArticlesCount", jsonPost(map[string]interface{}{"count": 7}))
	dataHubProcedure(mux, "GetTrendStats", jsonPost(map[string]interface{}{
		"points": []map[string]interface{}{{
			"bucket":       "2026-07-30T00:00:00Z",
			"articles":     12,
			"summarized":   9,
			"feedActivity": 3,
		}},
		"granularity": "TREND_GRANULARITY_DAILY",
	}))
	dataHubProcedure(mux, "ListUserFeedIDs", jsonPost(map[string]interface{}{
		"feedIds": []string{stubStatsFeedID},
	}))
}
