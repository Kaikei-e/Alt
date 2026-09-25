//go:build contract

package contract

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/pact-foundation/pact-go/v2/models"

	"alt/domain"
	"alt/orchestrator/usecase/fetch_recent_articles_usecase"
)

// mountVersionAndStatsProcedures adds the versioned artifacts and the dashboard
// statistics ADR-000954 moved off the direct alt_db path
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
func mountVersionAndStatsProcedures(mux *http.ServeMux) {
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

// mountPushProcedures adds the Web Push surface: the subscription registry the
// browser writes through alt-backend, and the delivery queue the dispatcher
// claims from. It is the one part of DataHubService with three consumers —
// alt-backend registers and dispatches, alt-harvester and recap-worker only
// enqueue — so a shape change here breaks in three places at once.
//
// Three things below are contract rather than stub filler:
//
//   - GetPushSubscription answers `{}` for an endpoint this user never
//     registered. The proto makes that deliberately indistinguishable from
//     "no such endpoint at all", and the caller re-prompts on the absence, so
//     a stub that always found a row would verify a provider that had dropped
//     the distinction.
//   - deliveryCount and supersededCount are JSON numbers. They are int32,
//     unlike the int64 retention counts above that protojson renders as
//     strings.
//   - A claimed delivery comes back SENDING, not PENDING. The claim and the
//     state change are one provider-side transaction; PENDING here would
//     describe a provider whose lease never took, and every dispatcher would
//     re-claim the same row.
func mountPushProcedures(mux *http.ServeMux) {
	const (
		stubPushUser         = "3f2504e0-4f89-11d3-9a0c-0305e82c3301"
		stubEndpoint         = "https://push.example.com/subscription/AAAA-BBBB-CCCC"
		stubP256dh           = "BEl62iUYgUivxIkv69yViEuiBIa-Ib9-SkTtF71JbFw"
		stubAuth             = "tBHItJI5svbpez7KI4CCXg"
		stubVAPIDFingerprint = "b0a1c2d3e4f5"
		stubDeliveryID       = "9c858901-8a57-4791-81fe-4c455b099bc9"
		stubSubscriptionID   = "1b4e28ba-2fa1-4d3b-a3f5-ccee1bf27e11"
		stubDedupeKey        = "recap:7d2a1c34-0a5f-4e51-9b1f-1a2b3c4d5e6f"
	)

	subscription := map[string]interface{}{
		"userId":              stubPushUser,
		"endpoint":            stubEndpoint,
		"p256dh":              stubP256dh,
		"auth":                stubAuth,
		"vapidKeyFingerprint": stubVAPIDFingerprint,
		"preferences": map[string]interface{}{
			"summaryReady":       true,
			"acolyteReportReady": true,
			"recapReady":         true,
			"todayEntranceReady": true,
		},
		"createdAt": "2026-08-01T00:00:00Z",
		"updatedAt": "2026-08-01T00:00:00Z",
	}

	// ---- Subscription registry ---------------------------------------------
	dataHubProcedure(mux, "UpsertPushSubscription", jsonPost(map[string]interface{}{
		"created": true,
	}))
	dataHubProcedure(mux, "GetPushSubscription", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			Endpoint string `json:"endpoint"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)

		w.Header().Set("Content-Type", "application/json")
		if req.Endpoint != stubEndpoint {
			_, _ = w.Write([]byte(`{}`))
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"subscription": subscription})
	})
	dataHubProcedure(mux, "UpdatePushSubscriptionPreferences", jsonPost(map[string]interface{}{
		"updated": true,
	}))
	dataHubProcedure(mux, "DeletePushSubscription", jsonPost(map[string]interface{}{
		"deleted": true,
	}))
	dataHubProcedure(mux, "ListPushSubscriptionsForUser", jsonPost(map[string]interface{}{
		"subscriptions": []map[string]interface{}{subscription},
	}))

	// ---- Delivery queue ----------------------------------------------------
	//
	// supersededCount is non-zero for every caller, including the two whose
	// pact records a zero: all three pin it with a type matcher, and the
	// harvester's daily digest is the one that actually collapses older rows.
	dataHubProcedure(mux, "EnqueueNotification", jsonPost(map[string]interface{}{
		"deliveryCount":   2,
		"supersededCount": 1,
	}))
	dataHubProcedure(mux, "ClaimNotificationBatch", jsonPost(map[string]interface{}{
		"deliveries": []map[string]interface{}{{
			"id":             stubDeliveryID,
			"dedupeKey":      stubDedupeKey,
			"subscriptionId": stubSubscriptionID,
			"userId":         stubPushUser,
			"kind":           "recap_ready",
			"payload":        "eyJyZWNhcF9pZCI6InIxIn0=",
			"occurredAt":     "2026-08-01T09:30:00Z",
			"state":          "NOTIFICATION_STATE_SENDING",
			"attempts":       1,
			"nextAttemptAt":  "2026-08-01T09:31:00Z",
			"expiresAt":      "2026-08-02T09:30:00Z",
			"endpoint":       stubEndpoint,
			"p256dh":         stubP256dh,
			"auth":           stubAuth,
		}},
	}))

	// The backlog read is the one procedure whose two agreed answers differ in
	// value rather than in shape, so it is the one that cannot be a constant.
	// A deployment with devices and one without return the same fields; the
	// difference is the whole content of the contract, and protoJSON omits a
	// zero, so the unsubscribed reading is the empty object. Answering a fixed
	// populated body would verify the interaction that matters least.
	dataHubProcedure(mux, "GetNotificationBacklogAge", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "application/json")

		body := map[string]interface{}{
			"oldestPendingAgeSeconds": 42.5,
			"pendingCount":            "3",
			"activeSubscriptionCount": "2",
		}
		if unsubscribedDeployment.Load() {
			body = map[string]interface{}{}
		}
		_ = json.NewEncoder(w).Encode(body)
	})

	// The three terminal transitions report nothing: the row id was the
	// caller's, so there is no server-assigned value to hand back.
	dataHubProcedure(mux, "MarkNotificationSent", jsonPost(map[string]interface{}{}))
	dataHubProcedure(mux, "ReleaseNotification", jsonPost(map[string]interface{}{}))
	dataHubProcedure(mux, "MarkNotificationDead", jsonPost(map[string]interface{}{}))
}

// mountDeepHealth serves GET /health/deep on the data-plane mux — the same
// listener as DataHubService, not ops :9110. The envelope is the one
// datahub_client.Ping reads; latency_ms and cached are omitted so the
// contract does not pin timings.
func mountDeepHealth(mux *http.ServeMux) {
	mux.HandleFunc("/health/deep", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")

		status := "pass"
		httpStatus := http.StatusOK
		checkStatus := "pass"
		switch deepHealthMode(deepHealth.Load()) {
		case deepHealthWarn:
			status = "warn"
		case deepHealthFail:
			status = "fail"
			checkStatus = "fail"
			httpStatus = http.StatusServiceUnavailable
		}

		body := map[string]interface{}{
			"status":  status,
			"service": "alt-data-hub",
		}
		if status != "warn" {
			body["checks"] = []map[string]interface{}{
				{"name": "database", "status": checkStatus, "critical": true},
			}
		}
		w.WriteHeader(httpStatus)
		_ = json.NewEncoder(w).Encode(body)
	})
}

// unsubscribedDeployment selects which of the two agreed backlog readings the
// stub returns.
//
// The verifier drives states and requests on its own goroutines, hence the
// atomics. These two flags are the only stub state: every other procedure
// answers the same body for every interaction because every other procedure's
// contract is about shape.
var unsubscribedDeployment atomic.Bool

type deepHealthMode uint32

const (
	deepHealthPass deepHealthMode = iota
	deepHealthWarn
	deepHealthFail
)

var deepHealth atomic.Uint32

// backlogStates toggles the flag above from the two provider states that name
// the difference, and resets it on teardown so a state left set cannot leak
// into the next interaction.
func backlogStates() models.StateHandlers {
	set := func(unsubscribed bool) models.StateHandler {
		return func(setUp bool, _ models.ProviderState) (models.ProviderStateResponse, error) {
			unsubscribedDeployment.Store(setUp && unsubscribed)
			return nil, nil
		}
	}
	return models.StateHandlers{
		"alt-data-hub has a push delivery queue with pending rows and registered devices": set(false),
		"alt-data-hub has a drained push delivery queue and no registered devices":        set(true),
	}
}

// deepHealthStates selects which /health/deep envelope the stub returns.
// Teardown resets to pass so a fail/warn left set cannot leak into the next
// interaction.
func deepHealthStates() models.StateHandlers {
	set := func(mode deepHealthMode) models.StateHandler {
		return func(setUp bool, _ models.ProviderState) (models.ProviderStateResponse, error) {
			if setUp {
				deepHealth.Store(uint32(mode))
			} else {
				deepHealth.Store(uint32(deepHealthPass))
			}
			return nil, nil
		}
	}
	return models.StateHandlers{
		"alt-data-hub database is reachable":   set(deepHealthPass),
		"alt-data-hub data path is degraded":   set(deepHealthWarn),
		"alt-data-hub database is unavailable": set(deepHealthFail),
	}
}

type readStateProviderMode uint32

const (
	readStateDefault readStateProviderMode = iota
	readStateForUser
	readStateSinceTimestamp
	readStateNone
)

var readStateMode atomic.Uint32

func readStateStates() models.StateHandlers {
	set := func(mode readStateProviderMode) models.StateHandler {
		return func(setUp bool, _ models.ProviderState) (models.ProviderStateResponse, error) {
			if setUp {
				readStateMode.Store(uint32(mode))
			} else {
				readStateMode.Store(uint32(readStateDefault))
			}
			return nil, nil
		}
	}
	return models.StateHandlers{
		"read feeds exist for the user":                   set(readStateForUser),
		"read feeds exist for the user since a timestamp": set(readStateSinceTimestamp),
		"read feeds exist in the period":                  set(readStateSinceTimestamp),
		"alt-data-hub has read marks for the user":        set(readStateForUser),
	}
}

type pactReadStatePort struct {
	feedIDs []uuid.UUID
}

func (p *pactReadStatePort) MarkFeedRead(context.Context, string, uuid.UUID) error    { return nil }
func (p *pactReadStatePort) MarkArticleRead(context.Context, string, uuid.UUID) error { return nil }
func (p *pactReadStatePort) ReadFeedIDs(context.Context, uuid.UUID, []uuid.UUID) ([]uuid.UUID, error) {
	return p.feedIDs, nil
}
func (p *pactReadStatePort) AllReadFeedIDs(context.Context, uuid.UUID, *time.Time) ([]uuid.UUID, error) {
	mode := readStateMode.Load()
	if mode == uint32(readStateNone) {
		return []uuid.UUID{}, nil
	}
	return p.feedIDs, nil
}
func (p *pactReadStatePort) SubscribedFeedLinkIDs(context.Context, uuid.UUID) ([]uuid.UUID, error) {
	return nil, nil
}
func (p *pactReadStatePort) ListSubscriptions(context.Context, uuid.UUID) ([]*domain.FeedSource, error) {
	return nil, nil
}
func (p *pactReadStatePort) Subscribe(context.Context, uuid.UUID, uuid.UUID) error   { return nil }
func (p *pactReadStatePort) Unsubscribe(context.Context, uuid.UUID, uuid.UUID) error { return nil }
func (p *pactReadStatePort) AddFavorite(context.Context, string, uuid.UUID) error    { return nil }
func (p *pactReadStatePort) RemoveFavorite(context.Context, string, uuid.UUID) error { return nil }

type pactTagReadPort struct{}

func (pactTagReadPort) ArticleTags(context.Context, string) ([]*domain.FeedTag, error) {
	return nil, nil
}
func (pactTagReadPort) FeedTags(context.Context, string, *time.Time, int) ([]*domain.FeedTag, error) {
	return nil, nil
}
func (pactTagReadPort) Cooccurrences(context.Context, []string) ([]*domain.TagCooccurrence, error) {
	return nil, nil
}
func (pactTagReadPort) SearchByPrefix(context.Context, string, int) ([]domain.GlobalTagHit, error) {
	return nil, nil
}
func (pactTagReadPort) ArticleCounts(context.Context, uuid.UUID, time.Time) ([]domain.TagArticleCount, error) {
	return nil, nil
}

type pactSystemUser struct{}

func (pactSystemUser) GetFirstIdentityID(context.Context) (string, error) { return "", nil }

type pactRecentArticles struct{}

func (pactRecentArticles) Execute(context.Context, fetch_recent_articles_usecase.FetchRecentArticlesInput) (*fetch_recent_articles_usecase.FetchRecentArticlesOutput, error) {
	return nil, nil
}
