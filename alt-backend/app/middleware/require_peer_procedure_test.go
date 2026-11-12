package middleware

import (
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
)

func TestRequirePeerProcedure_DefaultAllowlist_FullInventory(t *testing.T) {
	var invoked bool
	handler := RequirePeerProcedure(nil, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		invoked = true
		w.WriteHeader(http.StatusOK)
	}))

	// Positive test: All actual peer-method pairs defined in the Sol 6.1 verified inventory
	// must succeed under both /services.datahub.v1.DataHubService/ and /alt.datahub.v1.DataHubService/
	namespaces := []string{
		"/services.datahub.v1.DataHubService/",
		"/alt.datahub.v1.DataHubService/",
	}

	for peer, methods := range defaultPeerProcedures {
		for method := range methods {
			for _, ns := range namespaces {
				path := ns + method
				testName := fmt.Sprintf("allowed_%s_%s", peer, method)
				if ns == "/alt.datahub.v1.DataHubService/" {
					testName += "_legacy_ns"
				}

				t.Run(testName, func(t *testing.T) {
					invoked = false
					req := httptest.NewRequest(http.MethodPost, path, nil)
					req.TLS = &tls.ConnectionState{
						VerifiedChains: [][]*x509.Certificate{
							{
								{Subject: pkix.Name{CommonName: peer}},
							},
						},
					}

					rr := httptest.NewRecorder()
					handler.ServeHTTP(rr, req)

					if rr.Code != http.StatusOK {
						t.Fatalf("peer %s calling %s: got status %d, want %d", peer, path, rr.Code, http.StatusOK)
					}
					if !invoked {
						t.Fatalf("peer %s calling %s: handler was not invoked", peer, path)
					}
				})
			}
		}
	}
}

func TestRequirePeerProcedure_Denials(t *testing.T) {
	customAllowlist := map[string]map[string]struct{}{
		"nil-peer": nil,
	}
	customHandler := RequirePeerProcedure(customAllowlist, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not have been reached")
	}))

	defaultHandler := RequirePeerProcedure(nil, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not have been reached")
	}))

	tests := []struct {
		name       string
		handler    http.Handler
		peer       string
		path       string
		hasTLS     bool
		wantStatus int
	}{
		// Health bypass
		{
			name:       "health_bypass",
			handler:    defaultHandler,
			peer:       "",
			path:       "/health",
			hasTLS:     false,
			wantStatus: http.StatusOK,
		},
		{
			name:       "health_deep_bypass",
			handler:    defaultHandler,
			peer:       "",
			path:       "/health/deep",
			hasTLS:     false,
			wantStatus: http.StatusOK,
		},

		// Missing TLS client certificate
		{
			name:       "missing_tls_cert",
			handler:    defaultHandler,
			peer:       "",
			path:       "/services.datahub.v1.DataHubService/GetArticleByID",
			hasTLS:     false,
			wantStatus: http.StatusForbidden,
		},

		// Unknown path (outside datahub namespaces)
		{
			name:       "unknown_path_denied",
			handler:    defaultHandler,
			peer:       "alt-backend",
			path:       "/services.other.v1.OtherService/Call",
			hasTLS:     true,
			wantStatus: http.StatusForbidden,
		},

		// Unknown peer denied
		{
			name:       "unknown_peer_denied",
			handler:    defaultHandler,
			peer:       "unknown-untrusted-peer",
			path:       "/services.datahub.v1.DataHubService/GetArticleByID",
			hasTLS:     true,
			wantStatus: http.StatusForbidden,
		},

		// Nil policy block explicitly denies all
		{
			name:       "nil_policy_peer_denied",
			handler:    customHandler,
			peer:       "nil-peer",
			path:       "/services.datahub.v1.DataHubService/GetArticleByID",
			hasTLS:     true,
			wantStatus: http.StatusForbidden,
		},

		// Future unknown methods denied for valid peers
		{
			name:       "future_method_denied_for_backend",
			handler:    defaultHandler,
			peer:       "alt-backend",
			path:       "/services.datahub.v1.DataHubService/FutureUnknownMethod",
			hasTLS:     true,
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "future_method_denied_for_harvester",
			handler:    defaultHandler,
			peer:       "alt-harvester",
			path:       "/services.datahub.v1.DataHubService/FutureIngestHook",
			hasTLS:     true,
			wantStatus: http.StatusForbidden,
		},

		// Specific Sol 6.1 cross-peer and pruned privilege denials
		{
			name:       "harvester_denied_CreateArticle",
			handler:    defaultHandler,
			peer:       "alt-harvester",
			path:       "/services.datahub.v1.DataHubService/CreateArticle",
			hasTLS:     true,
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "harvester_denied_FetchTagCloud",
			handler:    defaultHandler,
			peer:       "alt-harvester",
			path:       "/services.datahub.v1.DataHubService/FetchTagCloud",
			hasTLS:     true,
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "harvester_denied_GetTagCooccurrences",
			handler:    defaultHandler,
			peer:       "alt-harvester",
			path:       "/services.datahub.v1.DataHubService/GetTagCooccurrences",
			hasTLS:     true,
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "tag_generator_denied_BatchGetTagsByArticleIDs",
			handler:    defaultHandler,
			peer:       "tag-generator",
			path:       "/services.datahub.v1.DataHubService/BatchGetTagsByArticleIDs",
			hasTLS:     true,
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "acolyte_denied_FetchTagCloud",
			handler:    defaultHandler,
			peer:       "acolyte-orchestrator",
			path:       "/services.datahub.v1.DataHubService/FetchTagCloud",
			hasTLS:     true,
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "rag_orchestrator_denied_SearchRecapsByTag",
			handler:    defaultHandler,
			peer:       "rag-orchestrator",
			path:       "/services.datahub.v1.DataHubService/SearchRecapsByTag",
			hasTLS:     true,
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "search_indexer_denied_CreateArticle",
			handler:    defaultHandler,
			peer:       "search-indexer",
			path:       "/services.datahub.v1.DataHubService/CreateArticle",
			hasTLS:     true,
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "alt_notifier_denied_ListRecapArticles",
			handler:    defaultHandler,
			peer:       "alt-notifier",
			path:       "/services.datahub.v1.DataHubService/ListRecapArticles",
			hasTLS:     true,
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "pre_processor_denied_BatchUpsertArticleTags",
			handler:    defaultHandler,
			peer:       "pre-processor",
			path:       "/services.datahub.v1.DataHubService/BatchUpsertArticleTags",
			hasTLS:     true,
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "recap_worker_denied_DeletePushSubscription",
			handler:    defaultHandler,
			peer:       "recap-worker",
			path:       "/services.datahub.v1.DataHubService/DeletePushSubscription",
			hasTLS:     true,
			wantStatus: http.StatusForbidden,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// For health bypass tests, override handler with one that returns 200
			h := tt.handler
			if tt.path == "/health" || tt.path == "/health/deep" {
				h = RequirePeerProcedure(nil, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusOK)
				}))
			}

			req := httptest.NewRequest(http.MethodPost, tt.path, nil)
			if tt.hasTLS {
				req.TLS = &tls.ConnectionState{
					VerifiedChains: [][]*x509.Certificate{
						{
							{Subject: pkix.Name{CommonName: tt.peer}},
						},
					},
				}
			}

			rr := httptest.NewRecorder()
			h.ServeHTTP(rr, req)

			if rr.Code != tt.wantStatus {
				t.Errorf("got status %d, want %d", rr.Code, tt.wantStatus)
			}
		})
	}
}

func TestRequirePeerProcedure_ExactPeerAllowlist(t *testing.T) {
	handler := RequirePeerProcedure(nil, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	expected := map[string]struct {
		allowed []string
		denied  []string
	}{
		"alt-backend": {
			allowed: []string{
				"AddFavoriteFeed",
				"ArchiveArticle",
				"BatchGetArticlesByIDs",
				"BatchGetArticlesByURLs",
				"BatchGetFeedTitlesByIDs",
				"BatchGetOgImageURLs",
				"BulkRegisterFeedLinks",
				"CountBackfillArticles",
				"CreateSummaryVersion",
				"DeleteFeedLink",
				"DeletePushSubscription",
				"FetchTagCloud",
				"GetAllReadFeedIDs",
				"GetArticleByURL",
				"GetArticleContentByID",
				"GetArticleHead",
				"GetArticleSummaryByArticleID",
				"GetArticleTags",
				"GetArticleTitleAndLink",
				"GetFeedAmount",
				"GetFeedID",
				"GetFeedOgImageTargets",
				"GetFeedSummary",
				"GetFeedTags",
				"GetFeedURLsByArticleIDs",
				"GetImageProxyCache",
				"GetInoreaderSummariesByURLs",
				"GetLatestArticleByFeedID",
				"GetPushSubscription",
				"GetRandomFeed",
				"GetReadFeedIDs",
				"GetScrapingDomainByDomain",
				"GetScrapingDomainByID",
				"GetSingleFeed",
				"GetSummarizedArticlesCount",
				"GetTagArticleCounts",
				"GetTagCooccurrences",
				"GetTodayUnreadArticlesCount",
				"GetTotalArticlesCount",
				"GetTrendStats",
				"GetUnsummarizedArticlesCount",
				"GetUserSubscribedFeedLinkIDs",
				"IsDomainDeclined",
				"ListArticleIDsCursor",
				"ListArticlesByTagID",
				"ListArticlesByTagName",
				"ListArticlesCursor",
				"ListBackfillArticles",
				"ListFeedLinkDomains",
				"ListFeedLinks",
				"ListFeedLinksForExport",
				"ListFeedLinksWithHealth",
				"ListFeedsByFeedLinkID",
				"ListFeedsCursor",
				"ListFeedsLimit",
				"ListFeedsPage",
				"ListRSSFeedURLs",
				"ListScrapingDomains",
				"ListSubscriptions",
				"ListUserFeedIDs",
				"LookupArticleURL",
				"MarkArticleRead",
				"MarkFeedRead",
				"MarkSummaryVersionSuperseded",
				"PutImageProxyCache",
				"RegisterFeedLink",
				"RegisterFeeds",
				"RemoveFavoriteFeed",
				"ResetFeedLinkFailures",
				"ResolveFeedLinkIDByURL",
				"SaveArticleHead",
				"SaveArticleSummary",
				"SaveDeclinedDomain",
				"SaveFeedOgImage",
				"SaveScrapingDomain",
				"SearchFeedsByTitle",
				"SearchTagsByPrefix",
				"Subscribe",
				"Unsubscribe",
				"UpdatePushSubscriptionPreferences",
				"UpdateScrapingDomainPolicy",
				"UpsertArticleTags",
				"UpsertPushSubscription",
			},
			denied: []string{
				"ClaimNotificationBatch",
				"ClaimOutboxBatch",
				"PruneOutboxEvents",
				"PurgeExpiredArticleHeads",
				"PurgeExpiredFeedOgImages",
				"PurgeImageProxyCacheOlderThan",
				"EnqueueNotification",
				"MarkOutboxProcessed",
				"ReleaseOutboxEvent",
				"CreateArticle",
				"ListUntaggedArticles",
				"ListArticlesWithTags",
				"ListRecapArticles",
			},
		},
		"alt-harvester": {
			allowed: []string{
				"ClaimOutboxBatch",
				"EnqueueNotification",
				"EvictExpiredImageProxyCache",
				"GetImageProxyCache",
				"GetScrapingDomainByDomain",
				"GetScrapingDomainByID",
				"ListFeedLinkDomains",
				"ListRSSFeedURLs",
				"ListScrapingDomains",
				"ListUnwarmedOgImageURLs",
				"MarkOutboxProcessed",
				"PruneOutboxEvents",
				"PurgeExpiredArticleHeads",
				"PurgeExpiredFeedOgImages",
				"PurgeImageProxyCacheOlderThan",
				"PutImageProxyCache",
				"RecordFeedLinkFailure",
				"RegisterFeeds",
				"ReleaseOutboxEvent",
				"ResetFeedLinkFailures",
				"SaveScrapingDomain",
			},
			denied: []string{"ClaimNotificationBatch", "ArchiveArticle"},
		},
		"alt-notifier": {
			allowed: []string{
				"ClaimNotificationBatch",
				"DeletePushSubscription",
				"GetNotificationBacklogAge",
				"MarkNotificationDead",
				"MarkNotificationSent",
				"ReleaseNotification",
			},
			denied: []string{"ClaimOutboxBatch", "UpsertPushSubscription"},
		},
		"pre-processor": {
			allowed: []string{
				"CheckArticleExists",
				"CheckArticleSummaryExists",
				"CreateArticle",
				"DeleteArticleSummary",
				"EnqueueNotification",
				"FindArticlesWithSummaries",
				"GetArticleContent",
				"GetEmptyFeedID",
				"GetFeedID",
				"GetSystemUser",
				"HasUnsummarizedArticles",
				"ListFeedURLs",
				"ListUnsummarizedArticles",
				"SaveArticleSummary",
			},
			denied: []string{"ClaimOutboxBatch", "PurgeExpiredArticleHeads"},
		},
		"search-indexer": {
			allowed: []string{
				"GetArticleByID",
				"GetLatestArticleTimestamp",
				"ListArticlesWithTags",
				"ListArticlesWithTagsForward",
				"ListDeletedArticles",
			},
			denied: []string{"CreateArticle", "ClaimNotificationBatch"},
		},
		"tag-generator": {
			allowed: []string{
				"BatchUpsertArticleTags",
				"GetArticleContent",
				"ListUntaggedArticles",
				"UpsertArticleTags",
			},
			denied: []string{"DeleteArticleSummary", "ClaimOutboxBatch"},
		},
		"recap-worker": {
			allowed: []string{
				"BatchGetTagsByArticleIDs",
				"EnqueueNotification",
				"GetAllReadFeedIDs",
				"ListFeedsInWindow",
				"ListRecapArticles",
			},
			denied: []string{"CreateArticle", "DeletePushSubscription"},
		},
		"rag-orchestrator": {
			allowed: []string{
				"FetchArticlesByTag",
				"FetchTagCloud",
				"ListRecentArticles",
			},
			denied: []string{"SaveArticleSummary", "ClaimNotificationBatch"},
		},
		"acolyte-orchestrator": {
			allowed: []string{
				"EnqueueNotification",
			},
			denied: []string{"ClaimNotificationBatch", "GetArticleContentByID"},
		},
	}

	if len(defaultPeerProcedures) != len(expected) {
		t.Fatalf("peer count = %d, want %d", len(defaultPeerProcedures), len(expected))
	}

	for peer, tc := range expected {
		t.Run(peer, func(t *testing.T) {
			actualSet, ok := defaultPeerProcedures[peer]
			if !ok {
				t.Fatalf("missing peer %q in defaultPeerProcedures", peer)
			}
			actual := make([]string, 0, len(actualSet))
			for proc := range actualSet {
				actual = append(actual, proc)
			}
			slices.Sort(actual)
			want := slices.Clone(tc.allowed)
			slices.Sort(want)

			if !slices.Equal(actual, want) {
				t.Fatalf("peer %s allowlist mismatch:\ngot (%d):  %v\nwant (%d): %v", peer, len(actual), actual, len(want), want)
			}

			for _, proc := range want {
				req := httptest.NewRequest(http.MethodPost, "/services.datahub.v1.DataHubService/"+proc, nil)
				req.TLS = &tls.ConnectionState{
					VerifiedChains: [][]*x509.Certificate{{{Subject: pkix.Name{CommonName: peer}}}},
				}
				rr := httptest.NewRecorder()
				handler.ServeHTTP(rr, req)
				if rr.Code != http.StatusOK {
					t.Errorf("peer %s allowed proc %s: got %d, want %d", peer, proc, rr.Code, http.StatusOK)
				}
			}

			for _, proc := range tc.denied {
				req := httptest.NewRequest(http.MethodPost, "/services.datahub.v1.DataHubService/"+proc, nil)
				req.TLS = &tls.ConnectionState{
					VerifiedChains: [][]*x509.Certificate{{{Subject: pkix.Name{CommonName: peer}}}},
				}
				rr := httptest.NewRecorder()
				handler.ServeHTTP(rr, req)
				if rr.Code != http.StatusForbidden {
					t.Errorf("peer %s denied proc %s: got %d, want %d", peer, proc, rr.Code, http.StatusForbidden)
				}
			}
		})
	}
}

func TestRequirePeerProcedure_BackendLeastPrivilegeAndBackgroundClaims(t *testing.T) {
	handler := RequirePeerProcedure(nil, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	tests := []struct {
		name       string
		peer       string
		procedure  string
		wantStatus int
	}{
		{name: "backend_denied_ClaimNotificationBatch", peer: "alt-backend", procedure: "ClaimNotificationBatch", wantStatus: http.StatusForbidden},
		{name: "backend_denied_ClaimOutboxBatch", peer: "alt-backend", procedure: "ClaimOutboxBatch", wantStatus: http.StatusForbidden},
		{name: "backend_denied_PruneOutboxEvents", peer: "alt-backend", procedure: "PruneOutboxEvents", wantStatus: http.StatusForbidden},
		{name: "backend_denied_PurgeExpiredArticleHeads", peer: "alt-backend", procedure: "PurgeExpiredArticleHeads", wantStatus: http.StatusForbidden},
		{name: "backend_denied_PurgeExpiredFeedOgImages", peer: "alt-backend", procedure: "PurgeExpiredFeedOgImages", wantStatus: http.StatusForbidden},
		{name: "backend_denied_PurgeImageProxyCacheOlderThan", peer: "alt-backend", procedure: "PurgeImageProxyCacheOlderThan", wantStatus: http.StatusForbidden},
		{name: "backend_allowed_ArchiveArticle", peer: "alt-backend", procedure: "ArchiveArticle", wantStatus: http.StatusOK},
		{name: "backend_allowed_GetArticleContentByID", peer: "alt-backend", procedure: "GetArticleContentByID", wantStatus: http.StatusOK},
		{name: "backend_allowed_ListFeedsCursor", peer: "alt-backend", procedure: "ListFeedsCursor", wantStatus: http.StatusOK},
		{name: "backend_allowed_MarkFeedRead", peer: "alt-backend", procedure: "MarkFeedRead", wantStatus: http.StatusOK},
		{name: "backend_allowed_UpsertPushSubscription", peer: "alt-backend", procedure: "UpsertPushSubscription", wantStatus: http.StatusOK},
		{name: "harvester_allowed_ClaimOutboxBatch", peer: "alt-harvester", procedure: "ClaimOutboxBatch", wantStatus: http.StatusOK},
		{name: "notifier_allowed_ClaimNotificationBatch", peer: "alt-notifier", procedure: "ClaimNotificationBatch", wantStatus: http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/services.datahub.v1.DataHubService/"+tt.procedure, nil)
			req.TLS = &tls.ConnectionState{
				VerifiedChains: [][]*x509.Certificate{{{Subject: pkix.Name{CommonName: tt.peer}}}},
			}
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)
			if rr.Code != tt.wantStatus {
				t.Fatalf("peer %s calling %s: got %d, want %d", tt.peer, tt.procedure, rr.Code, tt.wantStatus)
			}
		})
	}
}
