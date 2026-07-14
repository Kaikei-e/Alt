package get_article_source_url_usecase

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// stubLookupPort lets tests control the LookupArticleURL response without
// running a real DB. Returns (url, err) verbatim from the recorded call.
type stubLookupPort struct {
	gotArticleID string
	gotUserID    uuid.UUID
	url          string
	err          error
}

func (s *stubLookupPort) LookupArticleURL(_ context.Context, articleID string, userID uuid.UUID) (string, error) {
	s.gotArticleID = articleID
	s.gotUserID = userID
	return s.url, s.err
}

func TestGetArticleSourceURL_Happy(t *testing.T) {
	stub := &stubLookupPort{url: "https://example.com/article-1"}
	uc := NewGetArticleSourceURLUsecase(stub)

	userID := uuid.New()
	articleID := uuid.New().String()

	url, err := uc.Execute(context.Background(), articleID, userID)
	require.NoError(t, err)
	require.Equal(t, "https://example.com/article-1", url)
	require.Equal(t, articleID, stub.gotArticleID, "article_id must be passed through")
	require.Equal(t, userID, stub.gotUserID, "user_id must be passed through (tenant scope)")
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
	stub := &stubLookupPort{url: ""}
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
	stub := &stubLookupPort{url: "https://example.com/article-2"}
	uc := NewGetArticleSourceURLUsecase(stub)

	userA := uuid.New()
	articleID := uuid.New().String()
	_, _ = uc.Execute(context.Background(), articleID, userA)
	require.Equal(t, userA, stub.gotUserID)

	userB := uuid.New()
	_, _ = uc.Execute(context.Background(), articleID, userB)
	require.Equal(t, userB, stub.gotUserID)
}
