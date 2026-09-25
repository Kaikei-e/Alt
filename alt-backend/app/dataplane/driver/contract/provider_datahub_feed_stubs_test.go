//go:build contract

package contract

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	"alt/dataplane/connect/datahubapi"
)

// mountFeedProcedures adds the feed and feed-link capabilities
// ADR-000954 moved off the direct alt_db path (catalog §2.F /
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
func mountFeedProcedures(mux *http.ServeMux) {
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

// mountReadStateAndTagProcedures adds the per-user feed state and the tag reads
// ADR-000954 moved off the direct alt_db path (capability
// catalog §2.I / §2.J).
//
// Three of the writes branch on the URL, and that branch is the batch's whole
// argument. Capability catalog §4-5 found MarkFeedRead and MarkArticleRead
// answering "there is no such feed" two different ways — one from a preceding
// SELECT's pgx.ErrNoRows, one from an upsert's RowsAffected() — and the
// favourite writes raising a third sentinel for the same situation. A stub
// that always answered 200 would verify a provider that had quietly kept them
// apart, because the consumer's whole branch is on the Connect code.
func mountReadStateAndTagProcedures(mux *http.ServeMux) {
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
	pactReadState := &pactReadStatePort{
		feedIDs: []uuid.UUID{uuid.MustParse(stubFeedRow)},
	}
	readStateDHHandler := datahubapi.NewHandler(
		nil, nil, nil, nil, nil,
		pactSystemUser{}, pactRecentArticles{},
		slog.Default(),
		datahubapi.WithReadStateAndTagCapabilities(pactReadState, pactTagReadPort{}),
	)
	dataHubProcedure(mux, "GetAllReadFeedIDs", connect.NewUnaryHandler(
		"/services.datahub.v1.DataHubService/GetAllReadFeedIDs",
		readStateDHHandler.GetAllReadFeedIDs,
	).ServeHTTP)
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
