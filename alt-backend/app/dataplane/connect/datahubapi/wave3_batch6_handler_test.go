package datahubapi

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"alt/dataplane/port/datahub_capability_port"
	"alt/domain"
	datahubv1 "alt/gen/proto/services/datahub/v1"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// ---------------------------------------------------------------------------
// Fakes
// ---------------------------------------------------------------------------

type fakeTagTrailPort struct {
	articles []*domain.TagTrailArticle
	err      error

	sawTagID   string
	sawTagName string
	sawCursor  *time.Time
	sawLimit   int
}

func (f *fakeTagTrailPort) ArticlesByTagID(_ context.Context, tagID string, cursor *time.Time, limit int) ([]*domain.TagTrailArticle, error) {
	f.sawTagID = tagID
	f.sawCursor = cursor
	f.sawLimit = limit
	return f.articles, f.err
}

func (f *fakeTagTrailPort) ArticlesByTagName(_ context.Context, tagName string, cursor *time.Time, limit int) ([]*domain.TagTrailArticle, error) {
	f.sawTagName = tagName
	f.sawCursor = cursor
	f.sawLimit = limit
	return f.articles, f.err
}

type fakeArticleRefPort struct {
	ref *domain.ArticleRef
	err error

	sawArticleID string
}

func (f *fakeArticleRefPort) ArticleRef(_ context.Context, articleID string) (*domain.ArticleRef, error) {
	f.sawArticleID = articleID
	return f.ref, f.err
}

func batch6Handler(t *testing.T, trail *fakeTagTrailPort, ref *fakeArticleRefPort) *Handler {
	t.Helper()
	if trail == nil {
		trail = &fakeTagTrailPort{}
	}
	if ref == nil {
		ref = &fakeArticleRefPort{}
	}
	return NewHandler(nil, nil, nil, nil, nil, &fakeSystemUser{}, &fakeRecentArticles{}, slog.Default(),
		WithWave3Batch6Capabilities(trail, ref))
}

const (
	testTrailTagID   = "44444444-dddd-4ddd-8ddd-dddddddddddd"
	testTrailFeedID  = "55555555-eeee-4eee-8eee-eeeeeeeeeeee"
	testTrailArticle = "66666666-ffff-4fff-8fff-ffffffffffff"
)

// ---------------------------------------------------------------------------
// Wiring
// ---------------------------------------------------------------------------

// TestWithWave3Batch6CapabilitiesRefusesNil is the rule-8 test for the last
// batch.
//
// Both ports here are the *only* route their caller has left. A nil tag-trail
// port makes every tag look like it has no articles, which the Tag Trail
// renders as an empty but successful page; a nil article-ref port makes the
// recall rail silently drop the items whose projection has not caught up —
// exactly the case the fallback exists to cover. Neither failure raises an
// error anywhere, which is why construction refuses instead.
func TestWithWave3Batch6CapabilitiesRefusesNil(t *testing.T) {
	tests := []struct {
		name  string
		trail datahub_capability_port.TagTrailPort
		ref   datahub_capability_port.ArticleRefPort
	}{
		{name: "nil tag trail port", ref: &fakeArticleRefPort{}},
		{name: "nil article ref port", trail: &fakeTagTrailPort{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Panics(t, func() {
				WithWave3Batch6Capabilities(tt.trail, tt.ref)
			})
		})
	}
}

// ---------------------------------------------------------------------------
// §2.J Tag Trail paging
// ---------------------------------------------------------------------------

func TestListArticlesByTagID(t *testing.T) {
	published := time.Date(2026, 7, 30, 8, 0, 0, 0, time.UTC)
	cursor := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)

	trail := &fakeTagTrailPort{articles: []*domain.TagTrailArticle{{
		ID:          testTrailArticle,
		Title:       "Tagged article",
		Link:        "https://example.com/tagged",
		PublishedAt: published,
		FeedID:      testTrailFeedID,
		FeedTitle:   "Example Feed",
	}}}
	h := batch6Handler(t, trail, nil)

	resp, err := h.ListArticlesByTagID(context.Background(), connect.NewRequest(&datahubv1.ListArticlesByTagIDRequest{
		TagId:  testTrailTagID,
		Cursor: timestamppb.New(cursor),
		Limit:  25,
	}))
	require.NoError(t, err)

	assert.Equal(t, testTrailTagID, trail.sawTagID)
	require.NotNil(t, trail.sawCursor)
	assert.Equal(t, cursor, trail.sawCursor.UTC())
	assert.Equal(t, 25, trail.sawLimit)

	require.Len(t, resp.Msg.GetArticles(), 1)
	got := resp.Msg.GetArticles()[0]
	assert.Equal(t, testTrailArticle, got.GetId())
	assert.Equal(t, "Tagged article", got.GetTitle())
	assert.Equal(t, "https://example.com/tagged", got.GetUrl())
	assert.Equal(t, published, got.GetPublishedAt().AsTime())
	assert.Equal(t, testTrailFeedID, got.GetFeedId())
	assert.Equal(t, "Example Feed", got.GetFeedTitle())
}

// TestListArticlesByTagIDFirstPage pins that an unset cursor reaches the port
// as nil rather than as the zero time. The two produce different SQL — no
// WHERE clause versus `created_at < '0001-01-01'` — and the second returns
// nothing at all.
func TestListArticlesByTagIDFirstPage(t *testing.T) {
	trail := &fakeTagTrailPort{}
	h := batch6Handler(t, trail, nil)

	_, err := h.ListArticlesByTagID(context.Background(), connect.NewRequest(&datahubv1.ListArticlesByTagIDRequest{
		TagId: testTrailTagID,
	}))
	require.NoError(t, err)
	assert.Nil(t, trail.sawCursor)
	assert.Equal(t, defaultTagLimit, trail.sawLimit)
}

func TestListArticlesByTagIDRejectsEmptyTagID(t *testing.T) {
	h := batch6Handler(t, nil, nil)

	_, err := h.ListArticlesByTagID(context.Background(), connect.NewRequest(&datahubv1.ListArticlesByTagIDRequest{}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
}

func TestListArticlesByTagIDClampsLimit(t *testing.T) {
	trail := &fakeTagTrailPort{}
	h := batch6Handler(t, trail, nil)

	_, err := h.ListArticlesByTagID(context.Background(), connect.NewRequest(&datahubv1.ListArticlesByTagIDRequest{
		TagId: testTrailTagID,
		Limit: maxTagLimit + 1,
	}))
	require.NoError(t, err)
	assert.Equal(t, maxTagLimit, trail.sawLimit)
}

func TestListArticlesByTagIDPortFailure(t *testing.T) {
	h := batch6Handler(t, &fakeTagTrailPort{err: errors.New("boom")}, nil)

	_, err := h.ListArticlesByTagID(context.Background(), connect.NewRequest(&datahubv1.ListArticlesByTagIDRequest{
		TagId: testTrailTagID,
	}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeInternal, connect.CodeOf(err))
}

func TestListArticlesByTagName(t *testing.T) {
	cursor := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	trail := &fakeTagTrailPort{articles: []*domain.TagTrailArticle{{
		ID:          testTrailArticle,
		Title:       "Cross-feed article",
		Link:        "https://example.com/cross",
		PublishedAt: cursor.Add(-time.Hour),
	}}}
	h := batch6Handler(t, trail, nil)

	resp, err := h.ListArticlesByTagName(context.Background(), connect.NewRequest(&datahubv1.ListArticlesByTagNameRequest{
		TagName: "golang",
		Cursor:  timestamppb.New(cursor),
		Limit:   10,
	}))
	require.NoError(t, err)

	assert.Equal(t, "golang", trail.sawTagName)
	require.NotNil(t, trail.sawCursor)
	assert.Equal(t, cursor, trail.sawCursor.UTC())
	assert.Equal(t, 10, trail.sawLimit)
	require.Len(t, resp.Msg.GetArticles(), 1)
	// A feedless row keeps its empty strings rather than becoming a nil entry:
	// the cross-feed query does not always resolve a feed title.
	assert.Empty(t, resp.Msg.GetArticles()[0].GetFeedId())
}

func TestListArticlesByTagNameRejectsEmptyName(t *testing.T) {
	h := batch6Handler(t, nil, nil)

	_, err := h.ListArticlesByTagName(context.Background(), connect.NewRequest(&datahubv1.ListArticlesByTagNameRequest{}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
}

// ---------------------------------------------------------------------------
// §2.C Article reference for the recall rail
// ---------------------------------------------------------------------------

func TestGetArticleTitleAndLink(t *testing.T) {
	published := time.Date(2026, 6, 1, 7, 30, 0, 0, time.UTC)
	ref := &fakeArticleRefPort{ref: &domain.ArticleRef{
		Title:       "Recalled",
		Link:        "https://example.com/recalled",
		PublishedAt: &published,
	}}
	h := batch6Handler(t, nil, ref)

	resp, err := h.GetArticleTitleAndLink(context.Background(), connect.NewRequest(&datahubv1.GetArticleTitleAndLinkRequest{
		ArticleId: testTrailArticle,
	}))
	require.NoError(t, err)

	assert.Equal(t, testTrailArticle, ref.sawArticleID)
	assert.True(t, resp.Msg.GetFound())
	assert.Equal(t, "Recalled", resp.Msg.GetTitle())
	assert.Equal(t, "https://example.com/recalled", resp.Msg.GetUrl())
	require.NotNil(t, resp.Msg.GetPublishedAt())
	assert.Equal(t, published, resp.Msg.GetPublishedAt().AsTime())
}

// TestGetArticleTitleAndLinkNotFound pins the whole reason `found` is a field.
//
// A missing article is not an error — the recall rail asks about candidates
// whose projection may legitimately have been pruned — and over the wire an
// unset title and an empty title are the same bytes. Without this flag the
// caller cannot tell the two apart, and NotFound as a Connect code would make
// a routine cache miss look like a fault.
func TestGetArticleTitleAndLinkNotFound(t *testing.T) {
	h := batch6Handler(t, nil, &fakeArticleRefPort{ref: nil})

	resp, err := h.GetArticleTitleAndLink(context.Background(), connect.NewRequest(&datahubv1.GetArticleTitleAndLinkRequest{
		ArticleId: testTrailArticle,
	}))
	require.NoError(t, err)
	assert.False(t, resp.Msg.GetFound())
	assert.Empty(t, resp.Msg.GetTitle())
	assert.Nil(t, resp.Msg.GetPublishedAt())
}

// TestGetArticleTitleAndLinkNullPublishedAt pins that an article with no date
// stays dateless rather than becoming the zero time, which would sort it to
// the bottom of every rail forever.
func TestGetArticleTitleAndLinkNullPublishedAt(t *testing.T) {
	h := batch6Handler(t, nil, &fakeArticleRefPort{ref: &domain.ArticleRef{
		Title: "Undated",
		Link:  "https://example.com/undated",
	}})

	resp, err := h.GetArticleTitleAndLink(context.Background(), connect.NewRequest(&datahubv1.GetArticleTitleAndLinkRequest{
		ArticleId: testTrailArticle,
	}))
	require.NoError(t, err)
	assert.True(t, resp.Msg.GetFound())
	assert.Nil(t, resp.Msg.GetPublishedAt())
}

func TestGetArticleTitleAndLinkRejectsEmptyID(t *testing.T) {
	h := batch6Handler(t, nil, nil)

	_, err := h.GetArticleTitleAndLink(context.Background(), connect.NewRequest(&datahubv1.GetArticleTitleAndLinkRequest{}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
}

func TestGetArticleTitleAndLinkPortFailure(t *testing.T) {
	h := batch6Handler(t, nil, &fakeArticleRefPort{err: errors.New("boom")})

	_, err := h.GetArticleTitleAndLink(context.Background(), connect.NewRequest(&datahubv1.GetArticleTitleAndLinkRequest{
		ArticleId: testTrailArticle,
	}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeInternal, connect.CodeOf(err))
}
