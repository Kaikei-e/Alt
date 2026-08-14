package backend_api

import (
	"context"
	"errors"
	"testing"

	"connectrpc.com/connect"

	"pre-processor/domain"
	datahubv1 "pre-processor/gen/proto/services/datahub/v1"
)

// TestUpsertArticlesReportingSkipped_ReportsFeedNotFound — a feed that
// alt-data-hub has not registered yet is a refusal external to the article:
// inoreader_articles.fetched_at is never bumped when the subscription lands
// hours later, so a caller tracking a fetched_at watermark has to hear about
// the refusal or the article is dropped for good.
func TestUpsertArticlesReportingSkipped_ReportsFeedNotFound(t *testing.T) {
	mock := &mockDataHubClient{
		getFeedIDFunc: func(_ context.Context, req *connect.Request[datahubv1.GetFeedIDRequest]) (*connect.Response[datahubv1.GetFeedIDResponse], error) {
			if req.Msg.FeedUrl == "https://unknown-feed.com" {
				return nil, connect.NewError(connect.CodeNotFound, errors.New("feed not found"))
			}
			return connect.NewResponse(&datahubv1.GetFeedIDResponse{FeedId: "feed-1"}), nil
		},
	}
	repo := newTestRepo(mock)

	articles := []*domain.Article{
		{URL: "https://example.com/unknown", FeedURL: "https://unknown-feed.com"},
		{URL: "https://example.com/known", FeedURL: "https://known-feed.com"},
	}

	skipped, err := repo.UpsertArticlesReportingSkipped(context.Background(), articles)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(skipped) != 1 {
		t.Fatalf("expected 1 skipped article, got %d", len(skipped))
	}
	if skipped[0].URL != "https://example.com/unknown" {
		t.Errorf("expected the unregistered-feed article to be reported, got %q", skipped[0].URL)
	}
}

// TestUpsertArticlesReportingSkipped_ReportsEmptyFeedURL — the sync query
// LEFT JOINs inoreader_subscriptions, so a missing subscription row surfaces
// as an empty feed_url. That row too becomes writable once the subscription
// arrives, hence it counts as a refusal rather than a permanently invalid row.
func TestUpsertArticlesReportingSkipped_ReportsEmptyFeedURL(t *testing.T) {
	mock := &mockDataHubClient{
		getFeedIDFunc: func(_ context.Context, _ *connect.Request[datahubv1.GetFeedIDRequest]) (*connect.Response[datahubv1.GetFeedIDResponse], error) {
			return connect.NewResponse(&datahubv1.GetFeedIDResponse{FeedId: "feed-1"}), nil
		},
	}
	repo := newTestRepo(mock)

	articles := []*domain.Article{
		{URL: "https://example.com/no-feed"},
		{URL: "https://example.com/has-feed", FeedURL: "https://feed.com"},
	}

	skipped, err := repo.UpsertArticlesReportingSkipped(context.Background(), articles)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(skipped) != 1 {
		t.Fatalf("expected 1 skipped article, got %d", len(skipped))
	}
	if skipped[0].URL != "https://example.com/no-feed" {
		t.Errorf("expected the subscription-less article to be reported, got %q", skipped[0].URL)
	}
}

// TestUpsertArticlesReportingSkipped_ReportsNothingWhenEverythingIsWritten
// keeps the happy path silent — a caller must be free to advance its
// watermark over a fully written batch.
func TestUpsertArticlesReportingSkipped_ReportsNothingWhenEverythingIsWritten(t *testing.T) {
	mock := &mockDataHubClient{
		getFeedIDFunc: func(_ context.Context, _ *connect.Request[datahubv1.GetFeedIDRequest]) (*connect.Response[datahubv1.GetFeedIDResponse], error) {
			return connect.NewResponse(&datahubv1.GetFeedIDResponse{FeedId: "feed-1"}), nil
		},
	}
	repo := newTestRepo(mock)

	articles := []*domain.Article{
		{URL: "https://example.com/article1", FeedURL: "https://feed.com"},
		{URL: "https://example.com/article2", FeedID: "preset-feed"},
	}

	skipped, err := repo.UpsertArticlesReportingSkipped(context.Background(), articles)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(skipped) != 0 {
		t.Fatalf("expected no skipped articles, got %d", len(skipped))
	}
}
