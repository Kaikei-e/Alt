package datahubapi

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"connectrpc.com/connect"
	"go.uber.org/mock/gomock"

	"alt/dataplane/port/internal_feed_port"
	datahubv1 "alt/gen/proto/services/datahub/v1"
	"alt/mocks"
	"alt/shared/driver/alt_db"
)

func TestGetFeedID_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockGetFeed := mocks.NewMockGetFeedIDPort(ctrl)

	h := NewHandler(nil, nil, nil, nil, nil, &fakeSystemUser{}, &fakeRecentArticles{}, nil,
		WithArticleIngestionPorts(nil, nil, nil, nil, mockGetFeed, nil))

	mockGetFeed.EXPECT().
		GetFeedID(gomock.Any(), "http://example.com/feed.xml").
		Return("feed-123", nil)

	req := connect.NewRequest(&datahubv1.GetFeedIDRequest{FeedUrl: "http://example.com/feed.xml"})
	resp, err := h.GetFeedID(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Msg.FeedId != "feed-123" {
		t.Errorf("expected feed_id feed-123, got %s", resp.Msg.FeedId)
	}
}

func TestGetFeedID_NotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockGetFeed := mocks.NewMockGetFeedIDPort(ctrl)

	h := NewHandler(nil, nil, nil, nil, nil, &fakeSystemUser{}, &fakeRecentArticles{}, nil,
		WithArticleIngestionPorts(nil, nil, nil, nil, mockGetFeed, nil))

	// internal_article_gateway wraps whatever the driver returns, and the
	// driver reports "no feed at this URL" as alt_db.ErrFeedNotFoundByURL.
	mockGetFeed.EXPECT().
		GetFeedID(gomock.Any(), "http://missing.com/feed.xml").
		Return("", fmt.Errorf("GetFeedID: %w", alt_db.ErrFeedNotFoundByURL))

	req := connect.NewRequest(&datahubv1.GetFeedIDRequest{FeedUrl: "http://missing.com/feed.xml"})
	_, err := h.GetFeedID(context.Background(), req)
	if connect.CodeOf(err) != connect.CodeNotFound {
		t.Errorf("expected CodeNotFound, got %v", connect.CodeOf(err))
	}
}

// pre-processor reads NotFound from this RPC as "the feed is not registered":
// UpsertArticles skips every article of that batch and CheckExists treats the
// URL as unknown. A pool exhaustion or a transient DB blip answered with the
// same code would therefore drop a whole batch of ingested articles and can
// misreport a registered feed as unregistered, so only the driver's absence
// sentinel may produce it.
func TestGetFeedID_RepositoryFailureIsNotNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockGetFeed := mocks.NewMockGetFeedIDPort(ctrl)

	h := NewHandler(nil, nil, nil, nil, nil, &fakeSystemUser{}, &fakeRecentArticles{}, nil,
		WithArticleIngestionPorts(nil, nil, nil, nil, mockGetFeed, nil))

	mockGetFeed.EXPECT().
		GetFeedID(gomock.Any(), "http://example.com/feed.xml").
		Return("", fmt.Errorf("GetFeedID: %w", errors.New("timeout: pool exhausted")))

	req := connect.NewRequest(&datahubv1.GetFeedIDRequest{FeedUrl: "http://example.com/feed.xml"})
	_, err := h.GetFeedID(context.Background(), req)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if connect.CodeOf(err) != connect.CodeInternal {
		t.Errorf("expected CodeInternal so the caller retries instead of skipping the batch, got %v", connect.CodeOf(err))
	}
}

func TestListFeedURLs_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockListFeeds := mocks.NewMockListFeedURLsPort(ctrl)

	h := NewHandler(nil, nil, nil, nil, nil, &fakeSystemUser{}, &fakeRecentArticles{}, nil,
		WithArticleIngestionPorts(nil, nil, nil, nil, nil, mockListFeeds))

	mockListFeeds.EXPECT().
		ListFeedURLs(gomock.Any(), "", 200).
		Return([]internal_feed_port.FeedURL{
			{FeedID: "f1", URL: "http://example.com/feed1.xml"},
			{FeedID: "f2", URL: "http://example.com/feed2.xml"},
		}, "f2", true, nil)

	req := connect.NewRequest(&datahubv1.ListFeedURLsRequest{Limit: 200})
	resp, err := h.ListFeedURLs(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Msg.Feeds) != 2 {
		t.Fatalf("expected 2 feeds, got %d", len(resp.Msg.Feeds))
	}
	if resp.Msg.Feeds[0].FeedId != "f1" {
		t.Errorf("expected first feed_id f1, got %s", resp.Msg.Feeds[0].FeedId)
	}
	if !resp.Msg.HasMore {
		t.Error("expected has_more to be true")
	}
	if resp.Msg.NextCursor != "f2" {
		t.Errorf("expected next_cursor f2, got %s", resp.Msg.NextCursor)
	}
}

// ── GetEmptyFeedID RPC tests ──

func TestGetEmptyFeedID_Found(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockGet := mocks.NewMockGetEmptyFeedIDPort(ctrl)

	h := NewHandler(nil, nil, nil, nil, nil, &fakeSystemUser{}, &fakeRecentArticles{}, nil,
		WithBackfillPorts(mockGet))

	mockGet.EXPECT().
		GetEmptyFeedID(gomock.Any(), "http://example.com/feed.xml").
		Return("empty-feed-123", nil)

	req := connect.NewRequest(&datahubv1.GetEmptyFeedIDRequest{FeedUrl: "http://example.com/feed.xml"})
	resp, err := h.GetEmptyFeedID(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Msg.FeedId != "empty-feed-123" {
		t.Errorf("expected feed_id empty-feed-123, got %s", resp.Msg.FeedId)
	}
}

func TestGetEmptyFeedID_NotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockGet := mocks.NewMockGetEmptyFeedIDPort(ctrl)

	h := NewHandler(nil, nil, nil, nil, nil, &fakeSystemUser{}, &fakeRecentArticles{}, nil,
		WithBackfillPorts(mockGet))

	mockGet.EXPECT().
		GetEmptyFeedID(gomock.Any(), "http://example.com/feed.xml").
		Return("", nil)

	req := connect.NewRequest(&datahubv1.GetEmptyFeedIDRequest{FeedUrl: "http://example.com/feed.xml"})
	resp, err := h.GetEmptyFeedID(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Msg.FeedId != "" {
		t.Errorf("expected empty feed_id, got %s", resp.Msg.FeedId)
	}
}

func TestGetEmptyFeedID_EmptyFeedURL(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockGet := mocks.NewMockGetEmptyFeedIDPort(ctrl)

	h := NewHandler(nil, nil, nil, nil, nil, &fakeSystemUser{}, &fakeRecentArticles{}, nil,
		WithBackfillPorts(mockGet))

	req := connect.NewRequest(&datahubv1.GetEmptyFeedIDRequest{FeedUrl: ""})
	_, err := h.GetEmptyFeedID(context.Background(), req)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("expected CodeInvalidArgument, got %v", connect.CodeOf(err))
	}
}

func TestGetEmptyFeedID_Error(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockGet := mocks.NewMockGetEmptyFeedIDPort(ctrl)

	h := NewHandler(nil, nil, nil, nil, nil, &fakeSystemUser{}, &fakeRecentArticles{}, nil,
		WithBackfillPorts(mockGet))

	mockGet.EXPECT().
		GetEmptyFeedID(gomock.Any(), "http://example.com/feed.xml").
		Return("", errors.New("db error"))

	req := connect.NewRequest(&datahubv1.GetEmptyFeedIDRequest{FeedUrl: "http://example.com/feed.xml"})
	_, err := h.GetEmptyFeedID(context.Background(), req)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if connect.CodeOf(err) != connect.CodeInternal {
		t.Errorf("expected CodeInternal, got %v", connect.CodeOf(err))
	}
}

func TestGetEmptyFeedID_Unimplemented(t *testing.T) {
	h := NewHandler(nil, nil, nil, nil, nil, &fakeSystemUser{}, &fakeRecentArticles{}, nil)

	req := connect.NewRequest(&datahubv1.GetEmptyFeedIDRequest{FeedUrl: "http://example.com/feed.xml"})
	_, err := h.GetEmptyFeedID(context.Background(), req)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if connect.CodeOf(err) != connect.CodeUnimplemented {
		t.Errorf("expected CodeUnimplemented, got %v", connect.CodeOf(err))
	}
}
