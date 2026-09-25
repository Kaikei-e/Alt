//go:build contract

package contract

import (
	"encoding/json"
	"net/http"
)

// mountMediaProcedures adds the capabilities ADR-000954 moved off the
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
func mountMediaProcedures(mux *http.ServeMux) {
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

	// On-demand OG image resolution. The stub answers by feed id because the
	// two branches the consumer takes are exactly the two encodings it has to
	// tell apart: a feed still worth fetching carries neither og_image_url nor
	// suppressed (both absent under protojson), while a refused one carries
	// suppressed. Answering the same body for both would let a consumer that
	// ignored `suppressed` verify green and then re-request refused origins on
	// every scroll.
	dataHubProcedure(mux, "GetFeedOgImageTargets", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			FeedIDs []string `json:"feedIds"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		const (
			fetchable  = "c3d4e5f6-3333-4333-8333-333333333333"
			suppressed = "d4e5f6a7-4444-4444-8444-444444444444"
			pageURL    = "https://example.com/posts/on-demand-og"
		)

		targets := make([]map[string]interface{}, 0, len(req.FeedIDs))
		for _, id := range req.FeedIDs {
			switch id {
			case fetchable:
				targets = append(targets, map[string]interface{}{
					"feedId":  fetchable,
					"pageUrl": pageURL,
					// Attempts already spent on a feed whose bar has expired.
					// The consumer multiplies the next bar by this, so a stub
					// that answered zero here would verify green against a
					// consumer that had quietly stopped escalating.
					"attempts": 2,
				})
			case suppressed:
				targets = append(targets, map[string]interface{}{
					"feedId":     suppressed,
					"pageUrl":    pageURL,
					"suppressed": true,
					"attempts":   3,
					// int64 crosses protojson as a string. This is the half of
					// the answer `suppressed` cannot carry: how much of the bar
					// is left, and therefore whether the reader's client should
					// come back in twenty seconds or give the card up.
					"retryAfterSeconds": "20",
				})
			}
			// A feed id the stub does not know is omitted, which is the wire
			// form of "never asked".
		}

		w.Header().Set("Content-Type", "application/json")
		if len(targets) == 0 {
			_, _ = w.Write([]byte("{}"))
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"targets": targets})
	})
	dataHubProcedure(mux, "SaveFeedOgImage", jsonPost(map[string]interface{}{}))
	dataHubProcedure(mux, "PurgeExpiredFeedOgImages", jsonPost(map[string]interface{}{
		"purgedCount": "12",
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

	mountArticleProcedures(mux)
}

// mountArticleProcedures adds the article capabilities ADR-000954
// moved off the direct alt_db path (catalog §2.B / §2.C / §2.N).
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
func mountArticleProcedures(mux *http.ServeMux) {
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
		"url":   stubURL,
		"title": "Example headline",
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

	mountFeedProcedures(mux)
	mountReadStateAndTagProcedures(mux)
	mountVersionAndStatsProcedures(mux)
	mountTagTrailProcedures(mux)
}

// mountTagTrailProcedures adds the capabilities for the
// Tag Trail's paged reads (§2.J) and the recall rail's article fallback
// (§2.C).
//
// GetArticleTitleAndLink branches on the article id for the same reason
// GetArticleByURL and GetArticleHead do above: both of its answers mean
// something. `found` is the entire point of the message — proto3 cannot tell
// an unset title from an empty one — so a stub that always answered found
// would verify a provider that had dropped the distinction and would let the
// recall rail render every deleted article as a blank row.
func mountTagTrailProcedures(mux *http.ServeMux) {
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
