package article_repository_gateway

import (
	"context"
	"errors"
	"testing"

	"alt/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubLocal struct {
	fetchedURL string
	savedURL   string
	savedHead  string
}

func (s *stubLocal) FetchArticleByURL(_ context.Context, articleURL string) (*domain.ArticleContent, error) {
	s.fetchedURL = articleURL
	return &domain.ArticleContent{}, nil
}

func (s *stubLocal) SaveArticle(_ context.Context, url, _, _ string) (string, error) {
	s.savedURL = url
	return "article-1", nil
}

func (s *stubLocal) SaveArticleHead(_ context.Context, articleID, _, _ string) error {
	s.savedHead = articleID
	return nil
}

type stubOgImage struct {
	head      *domain.ArticleHead
	headErr   error
	ogURL     string
	askedHead string
	askedOg   string
}

func (s *stubOgImage) FetchArticleHeadByArticleID(_ context.Context, articleID string) (*domain.ArticleHead, error) {
	s.askedHead = articleID
	return s.head, s.headErr
}

func (s *stubOgImage) FetchOgImageURLByArticleID(_ context.Context, articleID string) (string, error) {
	s.askedOg = articleID
	return s.ogURL, nil
}

type stubDeclined struct {
	declined   bool
	savedUser  string
	savedHost  string
	askedUser  string
	askedHost  string
	declineErr error
}

func (s *stubDeclined) SaveDeclinedDomain(_ context.Context, userID, domainName string) error {
	s.savedUser, s.savedHost = userID, domainName
	return s.declineErr
}

func (s *stubDeclined) IsDomainDeclined(_ context.Context, userID, domainName string) (bool, error) {
	s.askedUser, s.askedHost = userID, domainName
	return s.declined, nil
}

func newTestGateway() (*Gateway, *stubLocal, *stubOgImage, *stubDeclined) {
	local := &stubLocal{}
	og := &stubOgImage{}
	declined := &stubDeclined{}
	return New(local, og, declined), local, og, declined
}

// TestNewRequiresEveryCollaborator: this gateway satisfies one interface out of
// three sources, and a nil one would fail on whichever of the seven methods
// happened to be called first — during a user request, not at boot.
func TestNewRequiresEveryCollaborator(t *testing.T) {
	local := &stubLocal{}
	og := &stubOgImage{}
	declined := &stubDeclined{}

	tests := []struct {
		name     string
		local    localArticleStore
		ogImage  ogImageReader
		declined declinedDomainStore
	}{
		{name: "no local store", ogImage: og, declined: declined},
		{name: "no og image reader", local: local, declined: declined},
		{name: "no declined domain store", local: local, ogImage: og},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Panics(t, func() { New(tt.local, tt.ogImage, tt.declined) })
		})
	}
}

// TestRoutesToDataHub covers the four methods this batch moved: whichever
// collaborator answers, it must not be the local database.
func TestRoutesToDataHub(t *testing.T) {
	gw, local, og, declined := newTestGateway()
	og.head = &domain.ArticleHead{ArticleID: "a1", OgImageURL: "https://cdn.example/og.png"}
	og.ogURL = "https://cdn.example/og.png"
	declined.declined = true

	ctx := context.Background()

	head, err := gw.FetchArticleHeadByArticleID(ctx, "a1")
	require.NoError(t, err)
	require.NotNil(t, head)
	assert.Equal(t, "a1", og.askedHead)

	url, err := gw.FetchOgImageURLByArticleID(ctx, "a2")
	require.NoError(t, err)
	assert.Equal(t, "https://cdn.example/og.png", url)
	assert.Equal(t, "a2", og.askedOg)

	got, err := gw.IsDomainDeclined(ctx, "u1", "paywalled.example")
	require.NoError(t, err)
	assert.True(t, got)
	assert.Equal(t, "u1", declined.askedUser)
	assert.Equal(t, "paywalled.example", declined.askedHost)

	require.NoError(t, gw.SaveDeclinedDomain(ctx, "u1", "paywalled.example"))
	assert.Equal(t, "paywalled.example", declined.savedHost)

	assert.Empty(t, local.fetchedURL)
	assert.Empty(t, local.savedURL)
	assert.Empty(t, local.savedHead)
}

// TestRoutesToLocalStore covers the three methods this batch does NOT move.
// They belong to catalog §2.B / §2.C and stay on the direct driver until those
// batches land; pinning it here means a later batch cannot quietly forget to
// move one.
func TestRoutesToLocalStore(t *testing.T) {
	gw, local, og, declined := newTestGateway()
	ctx := context.Background()

	_, err := gw.FetchArticleByURL(ctx, "https://example.com/post")
	require.NoError(t, err)
	assert.Equal(t, "https://example.com/post", local.fetchedURL)

	id, err := gw.SaveArticle(ctx, "https://example.com/post", "title", "body")
	require.NoError(t, err)
	assert.Equal(t, "article-1", id)

	require.NoError(t, gw.SaveArticleHead(ctx, "a1", "<head></head>", "https://cdn.example/og.png"))
	assert.Equal(t, "a1", local.savedHead)

	assert.Empty(t, og.askedHead)
	assert.Empty(t, declined.askedHost)
}

// TestPropagatesDataHubErrors: a failed RPC must not read as "no head found",
// which is the answer that makes the caller re-scrape the page.
func TestPropagatesDataHubErrors(t *testing.T) {
	local := &stubLocal{}
	sentinel := errors.New("data-hub unreachable")
	og := &stubOgImage{headErr: sentinel}
	gw := New(local, og, &stubDeclined{})

	head, err := gw.FetchArticleHeadByArticleID(context.Background(), "a1")
	require.Error(t, err)
	assert.ErrorIs(t, err, sentinel)
	assert.Nil(t, head)
}
