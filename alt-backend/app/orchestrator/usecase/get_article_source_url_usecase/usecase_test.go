package get_article_source_url_usecase

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"alt/domain"
)

// stubLookupPort lets tests control the lookup response without running a real
// DB. Returns (source, err) verbatim from the recorded call.
type stubLookupPort struct {
	gotArticleID string
	gotUserID    uuid.UUID
	source       domain.ArticleSource
	err          error
}

func (s *stubLookupPort) LookupArticleSource(_ context.Context, articleID string, userID uuid.UUID) (domain.ArticleSource, error) {
	s.gotArticleID = articleID
	s.gotUserID = userID
	return s.source, s.err
}

func TestGetArticleSourceURL_Happy(t *testing.T) {
	stub := &stubLookupPort{source: domain.ArticleSource{
		URL:   "https://example.com/article-1",
		Title: "How the masthead learned its name",
	}}
	uc := NewGetArticleSourceURLUsecase(stub)

	userID := uuid.New()
	articleID := uuid.New().String()

	source, err := uc.Execute(context.Background(), articleID, userID)
	require.NoError(t, err)
	require.Equal(t, "https://example.com/article-1", source.URL)
	require.Equal(t, "How the masthead learned its name", source.Title,
		"the stored title is the only headline an id-only caller can get")
	require.Equal(t, articleID, stub.gotArticleID, "article_id must be passed through")
	require.Equal(t, userID, stub.gotUserID, "user_id must be passed through (tenant scope)")
}

func TestGetArticleSourceURL_EmptyTitleIsStillAHit(t *testing.T) {
	// A row whose title was never populated is a data gap for the caller to
	// render around, not a lookup failure — the URL is what decides found from
	// not-found.
	stub := &stubLookupPort{source: domain.ArticleSource{URL: "https://example.com/untitled"}}
	uc := NewGetArticleSourceURLUsecase(stub)

	source, err := uc.Execute(context.Background(), uuid.New().String(), uuid.New())
	require.NoError(t, err)
	require.Equal(t, "https://example.com/untitled", source.URL)
	require.Empty(t, source.Title)
}

func TestGetArticleSourceURL_MalformedUUID_InvalidArgument(t *testing.T) {
	stub := &stubLookupPort{}
	uc := NewGetArticleSourceURLUsecase(stub)

	_, err := uc.Execute(context.Background(), "not-a-uuid", uuid.New())
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrInvalidArgument))
	// Driver must NOT have been called when the input is malformed.
	require.Empty(t, stub.gotArticleID, "lookup must short-circuit on malformed UUID")
}

func TestGetArticleSourceURL_LookupReturnsEmpty_NotFound(t *testing.T) {
	stub := &stubLookupPort{}
	uc := NewGetArticleSourceURLUsecase(stub)

	_, err := uc.Execute(context.Background(), uuid.New().String(), uuid.New())
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrNotFound))
}

func TestGetArticleSourceURL_LookupReturnsError_Wrapped(t *testing.T) {
	wantInner := errors.New("db connection refused")
	stub := &stubLookupPort{err: wantInner}
	uc := NewGetArticleSourceURLUsecase(stub)

	_, err := uc.Execute(context.Background(), uuid.New().String(), uuid.New())
	require.Error(t, err)
	require.True(t, errors.Is(err, wantInner), "underlying driver error must remain wrap-traceable")
}

func TestGetArticleSourceURL_TenantScopeIsolation(t *testing.T) {
	// Two distinct user_ids must propagate to the lookup unchanged. This
	// pins the contract that the usecase MUST NOT cache or substitute the
	// user_id — cross-tenant URL disclosure was security audit finding #1
	// on the ACT Open fix (ADR-879).
	stub := &stubLookupPort{source: domain.ArticleSource{URL: "https://example.com/article-2"}}
	uc := NewGetArticleSourceURLUsecase(stub)

	userA := uuid.New()
	articleID := uuid.New().String()
	_, _ = uc.Execute(context.Background(), articleID, userA)
	require.Equal(t, userA, stub.gotUserID)

	userB := uuid.New()
	_, _ = uc.Execute(context.Background(), articleID, userB)
	require.Equal(t, userB, stub.gotUserID)
}
