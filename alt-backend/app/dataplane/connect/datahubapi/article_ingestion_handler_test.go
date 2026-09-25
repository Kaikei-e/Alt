package datahubapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"connectrpc.com/connect"
	"go.uber.org/mock/gomock"
	"google.golang.org/protobuf/types/known/timestamppb"

	"alt/dataplane/port/internal_article_port"
	"alt/domain"
	datahubv1 "alt/gen/proto/services/datahub/v1"
	"alt/mocks"
	"alt/shared/port/event_publisher_port"
)

// ── Article ingestion RPC tests ──

func TestCheckArticleExists_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockCheckExists := mocks.NewMockCheckArticleExistsPort(ctrl)

	h := NewHandler(nil, nil, nil, nil, nil, &fakeSystemUser{}, &fakeRecentArticles{}, nil,
		WithArticleIngestionPorts(mockCheckExists, nil, nil, nil, nil, nil))

	mockCheckExists.EXPECT().
		CheckArticleExists(gomock.Any(), "http://example.com/article", "feed-1").
		Return(true, "article-123", nil)

	req := connect.NewRequest(&datahubv1.CheckArticleExistsRequest{
		Url:    "http://example.com/article",
		FeedId: "feed-1",
	})

	resp, err := h.CheckArticleExists(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Msg.Exists {
		t.Error("expected exists to be true")
	}
	if resp.Msg.ArticleId != "article-123" {
		t.Errorf("expected article_id article-123, got %s", resp.Msg.ArticleId)
	}
}

func TestCheckArticleExists_NotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockCheckExists := mocks.NewMockCheckArticleExistsPort(ctrl)

	h := NewHandler(nil, nil, nil, nil, nil, &fakeSystemUser{}, &fakeRecentArticles{}, nil,
		WithArticleIngestionPorts(mockCheckExists, nil, nil, nil, nil, nil))

	mockCheckExists.EXPECT().
		CheckArticleExists(gomock.Any(), "http://example.com/new", "feed-1").
		Return(false, "", nil)

	req := connect.NewRequest(&datahubv1.CheckArticleExistsRequest{
		Url:    "http://example.com/new",
		FeedId: "feed-1",
	})

	resp, err := h.CheckArticleExists(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Msg.Exists {
		t.Error("expected exists to be false")
	}
}

func TestCheckArticleExists_MissingURL(t *testing.T) {
	h := NewHandler(nil, nil, nil, nil, nil, &fakeSystemUser{}, &fakeRecentArticles{}, nil,
		WithArticleIngestionPorts(mocks.NewMockCheckArticleExistsPort(gomock.NewController(t)), nil, nil, nil, nil, nil))

	req := connect.NewRequest(&datahubv1.CheckArticleExistsRequest{FeedId: "feed-1"})
	_, err := h.CheckArticleExists(context.Background(), req)
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("expected CodeInvalidArgument, got %v", connect.CodeOf(err))
	}
}

func TestCreateArticle_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockCreate := mocks.NewMockCreateArticlePort(ctrl)

	h := NewHandler(nil, nil, nil, nil, nil, &fakeSystemUser{}, &fakeRecentArticles{}, nil,
		WithArticleIngestionPorts(nil, mockCreate, nil, nil, nil, nil),
		WithEventPublisher(&stubEventPublisher{}),
		WithKnowledgeEventPort(&stubKnowledgeEventPort{}))

	mockCreate.EXPECT().
		CreateArticle(gomock.Any(), internal_article_port.CreateArticleParams{
			Title:   "Test Article",
			URL:     "http://example.com/test",
			Content: "Hello world",
			FeedID:  "feed-1",
			UserID:  testTenantID,
		}).
		Return("new-article-id", true, nil)

	req := connect.NewRequest(&datahubv1.CreateArticleRequest{
		Title:   "Test Article",
		Url:     "http://example.com/test",
		Content: "Hello world",
		FeedId:  "feed-1",
		UserId:  testTenantID,
	})

	resp, err := h.CreateArticle(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Msg.ArticleId != "new-article-id" {
		t.Errorf("expected article_id new-article-id, got %s", resp.Msg.ArticleId)
	}
}

func TestCreateArticle_MissingURL(t *testing.T) {
	h := NewHandler(nil, nil, nil, nil, nil, &fakeSystemUser{}, &fakeRecentArticles{}, nil,
		WithArticleIngestionPorts(nil, mocks.NewMockCreateArticlePort(gomock.NewController(t)), nil, nil, nil, nil),
		WithKnowledgeEventPort(&stubKnowledgeEventPort{}))

	req := connect.NewRequest(&datahubv1.CreateArticleRequest{FeedId: "feed-1"})
	_, err := h.CreateArticle(context.Background(), req)
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("expected CodeInvalidArgument, got %v", connect.CodeOf(err))
	}
}

// The owner is not optional and never was: articles.user_id is UUID NOT NULL,
// so a request without a parseable one used to reach the database and come
// back as CodeInternal. It is also the tenant the Knowledge Home event is
// keyed by, which is why the check moved in front of the write rather than
// being skipped past on the event path.
func TestCreateArticle_RejectsUnusableUserID(t *testing.T) {
	tests := []struct {
		name   string
		userID string
	}{
		{name: "empty", userID: ""},
		{name: "not a uuid", userID: "user-1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewHandler(nil, nil, nil, nil, nil, &fakeSystemUser{}, &fakeRecentArticles{}, nil,
				WithArticleIngestionPorts(nil, mocks.NewMockCreateArticlePort(gomock.NewController(t)), nil, nil, nil, nil),
				WithKnowledgeEventPort(&stubKnowledgeEventPort{}))

			req := connect.NewRequest(&datahubv1.CreateArticleRequest{
				Url:    "http://example.com/test",
				FeedId: "feed-1",
				UserId: tt.userID,
			})
			_, err := h.CreateArticle(context.Background(), req)
			if connect.CodeOf(err) != connect.CodeInvalidArgument {
				t.Errorf("expected CodeInvalidArgument, got %v", connect.CodeOf(err))
			}
		})
	}
}

// ── CreateArticle event publishing tests ──

func TestCreateArticle_PublishesArticleCreatedEvent(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockCreate := mocks.NewMockCreateArticlePort(ctrl)
	mockPublisher := mocks.NewMockEventPublisherPort(ctrl)
	// IsEnabled is read twice: once by the option, which names the
	// publisher's wiring state in the boot log, and once on the publish
	// path itself.
	mockPublisher.EXPECT().IsEnabled().Return(true).AnyTimes()

	h := NewHandler(nil, nil, nil, nil, nil, &fakeSystemUser{}, &fakeRecentArticles{}, nil,
		WithArticleIngestionPorts(nil, mockCreate, nil, nil, nil, nil),
		WithEventPublisher(mockPublisher),
		WithKnowledgeEventPort(&stubKnowledgeEventPort{}),
	)

	publishedAt := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)

	mockCreate.EXPECT().
		CreateArticle(gomock.Any(), internal_article_port.CreateArticleParams{
			Title:       "Test Article",
			URL:         "http://example.com/test",
			Content:     "Hello world",
			FeedID:      "feed-1",
			UserID:      testTenantID,
			PublishedAt: publishedAt,
		}).
		Return("new-article-id", true, nil)

	mockPublisher.EXPECT().
		PublishArticleCreated(gomock.Any(), event_publisher_port.ArticleCreatedEvent{
			ArticleID:   "new-article-id",
			UserID:      testTenantID,
			FeedID:      "feed-1",
			Title:       "Test Article",
			URL:         "http://example.com/test",
			Content:     "Hello world",
			PublishedAt: publishedAt,
		}).
		Return(nil)

	req := connect.NewRequest(&datahubv1.CreateArticleRequest{
		Title:       "Test Article",
		Url:         "http://example.com/test",
		Content:     "Hello world",
		FeedId:      "feed-1",
		UserId:      testTenantID,
		PublishedAt: timestamppb.New(publishedAt),
	})

	resp, err := h.CreateArticle(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Msg.ArticleId != "new-article-id" {
		t.Errorf("expected article_id new-article-id, got %s", resp.Msg.ArticleId)
	}
}

// CLAUDE.md rule 8, the same refusal the knowledge event port gets one
// function below: an unwired producer must not be mistakable for a switched
// off one. mq-hub answers IsEnabled from its own configuration, so "publish
// nothing" is a state this handler can legitimately be in — reached by wiring
// a disabled publisher, never by omitting the option. Skipping a nil one
// writes the article, answers 200, and leaves summarisation and indexing with
// nothing to consume, which is the ADR-000928 failure mode exactly.
//
// This test replaces TestCreateArticle_NilEventPublisher, which pinned the
// skip.
func TestCreateArticle_PanicsWhenEventPublisherIsUnwired(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockCreate := mocks.NewMockCreateArticlePort(ctrl)

	// No WithEventPublisher — eventPublisher is nil
	h := NewHandler(nil, nil, nil, nil, nil, &fakeSystemUser{}, &fakeRecentArticles{}, nil,
		WithArticleIngestionPorts(nil, mockCreate, nil, nil, nil, nil),
		WithKnowledgeEventPort(&stubKnowledgeEventPort{}),
	)

	mockCreate.EXPECT().
		CreateArticle(gomock.Any(), gomock.Any()).
		Return("article-1", true, nil)

	req := connect.NewRequest(&datahubv1.CreateArticleRequest{
		Title:  "Test",
		Url:    "http://example.com",
		FeedId: "feed-1",
		UserId: testTenantID,
	})

	defer func() {
		if recover() == nil {
			t.Fatal("CreateArticle returned instead of panicking on an unwired event publisher")
		}
	}()
	_, _ = h.CreateArticle(context.Background(), req)
}

// And the disabled half of the same distinction: a publisher that is wired and
// reports itself off is a deployment choice, so the RPC succeeds and publishes
// nothing.
func TestCreateArticle_WiredButDisabledPublisherPublishesNothing(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockCreate := mocks.NewMockCreateArticlePort(ctrl)
	publisher := &stubEventPublisher{}

	h := NewHandler(nil, nil, nil, nil, nil, &fakeSystemUser{}, &fakeRecentArticles{}, nil,
		WithArticleIngestionPorts(nil, mockCreate, nil, nil, nil, nil),
		WithEventPublisher(publisher),
		WithKnowledgeEventPort(&stubKnowledgeEventPort{}),
	)

	mockCreate.EXPECT().
		CreateArticle(gomock.Any(), gomock.Any()).
		Return("article-1", true, nil)

	req := connect.NewRequest(&datahubv1.CreateArticleRequest{
		Title:  "Test",
		Url:    "http://example.com",
		FeedId: "feed-1",
		UserId: testTenantID,
	})

	resp, err := h.CreateArticle(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Msg.ArticleId != "article-1" {
		t.Errorf("expected article_id article-1, got %s", resp.Msg.ArticleId)
	}
	if len(publisher.created) != 0 {
		t.Errorf("expected no publish while disabled, got %d", len(publisher.created))
	}
}

func TestCreateArticle_EventPublishFailureDoesNotFailRPC(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockCreate := mocks.NewMockCreateArticlePort(ctrl)
	mockPublisher := mocks.NewMockEventPublisherPort(ctrl)
	// IsEnabled is read twice: once by the option, which names the
	// publisher's wiring state in the boot log, and once on the publish
	// path itself.
	mockPublisher.EXPECT().IsEnabled().Return(true).AnyTimes()

	h := NewHandler(nil, nil, nil, nil, nil, &fakeSystemUser{}, &fakeRecentArticles{}, nil,
		WithArticleIngestionPorts(nil, mockCreate, nil, nil, nil, nil),
		WithEventPublisher(mockPublisher),
		WithKnowledgeEventPort(&stubKnowledgeEventPort{}),
	)

	mockCreate.EXPECT().
		CreateArticle(gomock.Any(), gomock.Any()).
		Return("article-2", true, nil)

	mockPublisher.EXPECT().
		PublishArticleCreated(gomock.Any(), gomock.Any()).
		Return(errors.New("redis connection refused"))

	req := connect.NewRequest(&datahubv1.CreateArticleRequest{
		Title:  "Test",
		Url:    "http://example.com/test2",
		FeedId: "feed-1",
		UserId: testTenantID,
	})

	// RPC should succeed even though event publishing failed
	resp, err := h.CreateArticle(context.Background(), req)
	if err != nil {
		t.Fatalf("expected no error despite publish failure, got: %v", err)
	}
	if resp.Msg.ArticleId != "article-2" {
		t.Errorf("expected article_id article-2, got %s", resp.Msg.ArticleId)
	}
}

func TestCreateArticle_PublishesArticleUpdatedEventForUpsert(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockCreate := mocks.NewMockCreateArticlePort(ctrl)
	mockPublisher := mocks.NewMockEventPublisherPort(ctrl)
	// IsEnabled is read twice: once by the option, which names the
	// publisher's wiring state in the boot log, and once on the publish
	// path itself.
	mockPublisher.EXPECT().IsEnabled().Return(true).AnyTimes()

	h := NewHandler(nil, nil, nil, nil, nil, &fakeSystemUser{}, &fakeRecentArticles{}, nil,
		WithArticleIngestionPorts(nil, mockCreate, nil, nil, nil, nil),
		WithEventPublisher(mockPublisher),
		WithKnowledgeEventPort(&stubKnowledgeEventPort{}),
	)

	publishedAt := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)

	mockCreate.EXPECT().
		CreateArticle(gomock.Any(), gomock.Any()).
		Return("existing-article-id", false, nil)

	mockPublisher.EXPECT().
		PublishArticleUpdated(gomock.Any(), event_publisher_port.ArticleUpdatedEvent{
			ArticleID:   "existing-article-id",
			UserID:      testTenantID,
			FeedID:      "feed-1",
			Title:       "Updated Article",
			URL:         "http://example.com/existing",
			Content:     "Updated body",
			PublishedAt: publishedAt,
		}).
		Return(nil)

	req := connect.NewRequest(&datahubv1.CreateArticleRequest{
		Title:       "Updated Article",
		Url:         "http://example.com/existing",
		Content:     "Updated body",
		FeedId:      "feed-1",
		UserId:      testTenantID,
		PublishedAt: timestamppb.New(publishedAt),
	})

	resp, err := h.CreateArticle(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Msg.ArticleId != "existing-article-id" {
		t.Errorf("expected article_id existing-article-id, got %s", resp.Msg.ArticleId)
	}
}

func TestCreateArticle_EventPublisherDisabled(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockCreate := mocks.NewMockCreateArticlePort(ctrl)
	mockPublisher := mocks.NewMockEventPublisherPort(ctrl)
	// IsEnabled is read twice: once by the option, which names the
	// publisher's wiring state in the boot log, and once on the publish
	// path itself.
	mockPublisher.EXPECT().IsEnabled().Return(false).AnyTimes()

	h := NewHandler(nil, nil, nil, nil, nil, &fakeSystemUser{}, &fakeRecentArticles{}, nil,
		WithArticleIngestionPorts(nil, mockCreate, nil, nil, nil, nil),
		WithEventPublisher(mockPublisher),
		WithKnowledgeEventPort(&stubKnowledgeEventPort{}),
	)

	mockCreate.EXPECT().
		CreateArticle(gomock.Any(), gomock.Any()).
		Return("article-3", true, nil)

	// PublishArticleCreated should NOT be called when disabled

	req := connect.NewRequest(&datahubv1.CreateArticleRequest{
		Title:  "Test",
		Url:    "http://example.com/test3",
		FeedId: "feed-1",
		UserId: testTenantID,
	})

	resp, err := h.CreateArticle(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Msg.ArticleId != "article-3" {
		t.Errorf("expected article_id article-3, got %s", resp.Msg.ArticleId)
	}
}

func TestCreateArticle_AppendsKnowledgeEvent(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockCreate := mocks.NewMockCreateArticlePort(ctrl)
	stub := &stubKnowledgeEventPort{}

	h := NewHandler(nil, nil, nil, nil, nil, &fakeSystemUser{}, &fakeRecentArticles{}, nil,
		WithArticleIngestionPorts(nil, mockCreate, nil, nil, nil, nil),
		WithEventPublisher(&stubEventPublisher{}),
		WithKnowledgeEventPort(stub),
	)

	mockCreate.EXPECT().
		CreateArticle(gomock.Any(), gomock.Any()).
		Return("new-article-id", true, nil)

	req := connect.NewRequest(&datahubv1.CreateArticleRequest{
		Title:  "Test Article",
		Url:    "http://example.com/test",
		FeedId: "feed-1",
		UserId: testTenantID,
	})

	resp, err := h.CreateArticle(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Msg.ArticleId != "new-article-id" {
		t.Errorf("expected article_id new-article-id, got %s", resp.Msg.ArticleId)
	}

	// Knowledge event should have been appended
	if !stub.called {
		t.Fatal("expected knowledge event to be appended")
	}
	if stub.lastEvent.EventType != domain.EventArticleCreated {
		t.Errorf("expected event type ArticleCreated, got %s", stub.lastEvent.EventType)
	}
	if stub.lastEvent.AggregateID != "new-article-id" {
		t.Errorf("expected aggregate_id new-article-id, got %s", stub.lastEvent.AggregateID)
	}
}

// Not every feed item carries a pubDate, and published_at is a message field:
// an omitted one arrives here as a nil Timestamp and used to be formatted into
// the payload as the Go zero time, 0001-01-01.
//
// That is not a harmless placeholder. knowledge_events is INSERT-only, so the
// value the projector reads is the value Knowledge Home keeps forever, and the
// read model ranks recency over COALESCE(published_at, generated_at) — a
// year-1 item scores as two millennia stale and never appears in a recent or
// today window again. The payload's own "unknown" is the empty string, which
// the projector folds to a NULL published_at and the ranking then replaces
// with the event's generated_at.
func TestCreateArticle_OmittedPublishedAtIsUnknownRatherThanYearOne(t *testing.T) {
	publishedAt := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name        string
		publishedAt *timestamppb.Timestamp
		want        string
	}{
		{name: "omitted", publishedAt: nil, want: ""},
		{name: "sent", publishedAt: timestamppb.New(publishedAt), want: publishedAt.Format(time.RFC3339)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			mockCreate := mocks.NewMockCreateArticlePort(ctrl)
			stub := &stubKnowledgeEventPort{}

			h := NewHandler(nil, nil, nil, nil, nil, &fakeSystemUser{}, &fakeRecentArticles{}, nil,
				WithArticleIngestionPorts(nil, mockCreate, nil, nil, nil, nil),
				WithEventPublisher(&stubEventPublisher{}),
				WithKnowledgeEventPort(stub),
			)

			mockCreate.EXPECT().
				CreateArticle(gomock.Any(), gomock.Any()).
				Return("article-published-at", true, nil)

			req := connect.NewRequest(&datahubv1.CreateArticleRequest{
				Title:       "Test Article",
				Url:         "http://example.com/test",
				FeedId:      "feed-1",
				UserId:      testTenantID,
				PublishedAt: tt.publishedAt,
			})

			if _, err := h.CreateArticle(context.Background(), req); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			var payload domain.ArticleCreatedPayload
			if err := json.Unmarshal(stub.lastEvent.Payload, &payload); err != nil {
				t.Fatalf("unmarshal ArticleCreated payload: %v", err)
			}
			if payload.PublishedAt != tt.want {
				t.Errorf("ArticleCreated published_at = %q, want %q", payload.PublishedAt, tt.want)
			}
		})
	}
}

// An upsert is where the repair happens, not where it is skipped.
//
// created=false says alt-db already held the (url, user_id) row; it says
// nothing about whether sovereign ever received the article. Every retry of a
// CreateArticle whose append failed comes back on this branch, and so does
// every re-crawl of an unchanged article — so "no event on update" made the
// first miss permanent and left the Home row with the blank title
// SummaryVersionCreated gives it. The append is idempotent on dedupe_key.
func TestCreateArticle_AppendsKnowledgeEventOnUpsert(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockCreate := mocks.NewMockCreateArticlePort(ctrl)
	stub := &stubKnowledgeEventPort{}

	h := NewHandler(nil, nil, nil, nil, nil, &fakeSystemUser{}, &fakeRecentArticles{}, nil,
		WithArticleIngestionPorts(nil, mockCreate, nil, nil, nil, nil),
		WithEventPublisher(&stubEventPublisher{}),
		WithKnowledgeEventPort(stub),
	)

	mockCreate.EXPECT().
		CreateArticle(gomock.Any(), gomock.Any()).
		Return("existing-article-id", false, nil) // created=false means upsert

	req := connect.NewRequest(&datahubv1.CreateArticleRequest{
		Title:  "Updated",
		Url:    "http://example.com/existing",
		FeedId: "feed-1",
		UserId: testTenantID,
	})

	_, err := h.CreateArticle(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !stub.called {
		t.Fatal("expected knowledge event to be appended for an upsert")
	}
	if stub.lastEvent.AggregateID != "existing-article-id" {
		t.Errorf("expected aggregate_id existing-article-id, got %s", stub.lastEvent.AggregateID)
	}
	wantDedupe := fmt.Sprintf(domain.DedupeKeyArticleCreated, "existing-article-id")
	if stub.lastEvent.DedupeKey != wantDedupe {
		t.Errorf("expected dedupe_key %s, got %s", wantDedupe, stub.lastEvent.DedupeKey)
	}
}

// A dedupe hit answers event_seq 0 with no error, and that is a success: the
// article already has its ArticleCreated. Only a real failure may fail the RPC.
func TestCreateArticle_DedupedKnowledgeEventIsNotAFailure(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockCreate := mocks.NewMockCreateArticlePort(ctrl)
	stub := &stubKnowledgeEventPort{seq: 0}

	h := NewHandler(nil, nil, nil, nil, nil, &fakeSystemUser{}, &fakeRecentArticles{}, nil,
		WithArticleIngestionPorts(nil, mockCreate, nil, nil, nil, nil),
		WithEventPublisher(&stubEventPublisher{}),
		WithKnowledgeEventPort(stub),
	)

	mockCreate.EXPECT().
		CreateArticle(gomock.Any(), gomock.Any()).
		Return("article-dedupe", false, nil)

	req := connect.NewRequest(&datahubv1.CreateArticleRequest{
		Title:  "Already known",
		Url:    "http://example.com/known",
		FeedId: "feed-1",
		UserId: testTenantID,
	})

	if _, err := h.CreateArticle(context.Background(), req); err != nil {
		t.Fatalf("expected a dedupe hit to succeed, got: %v", err)
	}
}

// Losing ArticleCreated is what produces the blank-title Home rows, so the
// caller has to hear about it. There is no outbox row on this path to release
// back to PENDING the way the outbox worker does — the RPC's own result is the
// acknowledgement, so it is what gets withheld. pre-processor propagates the
// error and its next crawl re-sends the article.
func TestCreateArticle_KnowledgeEventFailureFailsRPC(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockCreate := mocks.NewMockCreateArticlePort(ctrl)
	stub := &stubKnowledgeEventPort{err: errors.New("sovereign unavailable")}

	h := NewHandler(nil, nil, nil, nil, nil, &fakeSystemUser{}, &fakeRecentArticles{}, nil,
		WithArticleIngestionPorts(nil, mockCreate, nil, nil, nil, nil),
		WithEventPublisher(&stubEventPublisher{}),
		WithKnowledgeEventPort(stub),
	)

	mockCreate.EXPECT().
		CreateArticle(gomock.Any(), gomock.Any()).
		Return("article-x", true, nil)

	req := connect.NewRequest(&datahubv1.CreateArticleRequest{
		Title:  "Test",
		Url:    "http://example.com/test-fail",
		FeedId: "feed-1",
		UserId: testTenantID,
	})

	_, err := h.CreateArticle(context.Background(), req)
	if err == nil {
		t.Fatal("expected CreateArticle to fail when the ArticleCreated append fails")
	}
	if connect.CodeOf(err) != connect.CodeUnavailable {
		t.Errorf("expected CodeUnavailable so the caller retries, got %v", connect.CodeOf(err))
	}
}

// CLAUDE.md rule 8: an unwired producer must be distinguishable from a
// disabled one. There is no configuration in which alt-data-hub writes
// articles without appending their ArticleCreated, so the only way to reach a
// nil port here is a composition-root mistake, and a mistake that answers 200
// is the ADR-000928 failure mode.
func TestCreateArticle_PanicsWhenKnowledgeEventPortIsUnwired(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockCreate := mocks.NewMockCreateArticlePort(ctrl)

	h := NewHandler(nil, nil, nil, nil, nil, &fakeSystemUser{}, &fakeRecentArticles{}, nil,
		WithArticleIngestionPorts(nil, mockCreate, nil, nil, nil, nil),
	)

	mockCreate.EXPECT().
		CreateArticle(gomock.Any(), gomock.Any()).
		Return("article-unwired", true, nil)

	req := connect.NewRequest(&datahubv1.CreateArticleRequest{
		Title:  "Test",
		Url:    "http://example.com/unwired",
		FeedId: "feed-1",
		UserId: testTenantID,
	})

	defer func() {
		if recover() == nil {
			t.Fatal("CreateArticle returned instead of panicking on an unwired knowledge event port")
		}
	}()
	_, _ = h.CreateArticle(context.Background(), req)
}

// And the same refusal one step earlier, where it costs a boot instead of a
// request: passing the option with nothing in it is the DI mistake itself.
func TestWithKnowledgeEventPort_RefusesNil(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("WithKnowledgeEventPort accepted a nil port")
		}
	}()
	NewHandler(nil, nil, nil, nil, nil, &fakeSystemUser{}, &fakeRecentArticles{}, nil,
		WithKnowledgeEventPort(nil),
	)
}

func TestClampLimit(t *testing.T) {
	tests := []struct {
		input    int
		expected int
	}{
		{0, 200},
		{-1, 200},
		{100, 100},
		{500, 500},
		{501, 500},
		{1000, 500},
	}

	for _, tt := range tests {
		got := clampLimit(tt.input)
		if got != tt.expected {
			t.Errorf("clampLimit(%d) = %d, want %d", tt.input, got, tt.expected)
		}
	}
}
