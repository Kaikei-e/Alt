//go:build contract

package contract

import (
	"encoding/json"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"alt/dataplane/connect/datahubapi"
)

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

	// Shared handler for the recap-worker paginated feed window fetch.
	feedsInWindowHandler := func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		var req listFeedsInWindowRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		from, errFrom := time.Parse(time.RFC3339, req.From)
		to, errTo := time.Parse(time.RFC3339, req.To)
		if errFrom == nil && errTo == nil && to.Sub(from) > 8*24*time.Hour {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"code":    "invalid_argument",
				"message": "date range exceeds 8 days",
			})
			return
		}
		page := 1
		if req.Page != nil && *req.Page > 0 {
			page = *req.Page
		}
		pageSize := 500
		if req.PageSize != nil && *req.PageSize > 0 {
			pageSize = *req.PageSize
		}

		feedLinkID := "c3d4e5f6-0001-4000-8000-000000000001"
		resp := feedsInWindowResponse{
			Feeds: []feedInWindowResponse{
				{
					ID:          "f1e2d3c4-0001-4000-8000-000000000001",
					Title:       "Example headline",
					Description: "<p>Example lede.</p>",
					WebsiteURL:  "https://example.com/post",
					PubDate:     "2026-03-20T00:00:00Z",
					CreatedAt:   "2026-03-20T01:00:00Z",
					UpdatedAt:   "2026-03-20T01:00:00Z",
					IsRead:      false,
					FeedLinkID:  &feedLinkID,
				},
			},
			Total:    2064,
			Page:     page,
			PageSize: pageSize,
			HasMore:  page*pageSize < 2064,
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}

	// ---- POST .../ListFeedsInWindow ----
	dataHubProcedure(mux, "ListFeedsInWindow", feedsInWindowHandler)

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

	// ---- services.datahub.v1.DataHubService (JSON wire format) ----
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
			// alt-backend's operator listener requires "Authorization: Bearer
			// <token>" (config.LoadOperatorAuth + the operator auth
			// interceptor); the BFF is the only consumer of this pact and
			// always sends one.
			auth := r.Header.Get("Authorization")
			hasBearer := strings.HasPrefix(auth, "Bearer ") && len(strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))) > 0
			if !hasBearer {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			_, _ = io.Copy(io.Discard, r.Body)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]int{
				"totalEvents": 100,
			})
		})

	mountMediaProcedures(mux)
	mountPushProcedures(mux)
	mountDeepHealth(mux)

	// The one route on this stub server that is not a stub: see
	// articles_prefetch_provider_test.go.
	mountArticleServiceProcedures(mux)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = ln.Close() })

	go func() {
		// Wrapped in the same alias the shipped listener uses: consumers
		// deployed before the ADR-000955 rename publish pacts against
		// alt.datahub.v1, and the broker's deployed-version selector keeps
		// those pacts in this matrix until each renamed consumer is recorded
		// as deployed (alt-deploy run 30769818431). Serving the stub through
		// the alias verifies the shim that actually ships, procedure list
		// included, instead of enumerating routes the way the
		// services.backend.v1 block below does. Remove with the alias.
		_ = http.Serve(ln, datahubapi.LegacyNamespaceAlias(mux))
	}()

	return ln.Addr().(*net.TCPAddr).Port
}
