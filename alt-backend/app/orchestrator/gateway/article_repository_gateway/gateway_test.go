package article_repository_gateway

import (
	"context"
	"errors"
	"testing"

	"alt/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubArticle struct {
	fetchedURL string
	savedURL   string
}

func (s *stubArticle) FetchArticleByURL(_ context.Context, articleURL string) (*domain.ArticleContent, error) {
	s.fetchedURL = articleURL
	return &domain.ArticleContent{}, nil
}

func (s *stubArticle) SaveArticle(_ context.Context, url, _, _ string) (string, error) {
	s.savedURL = url
	return "article-1", nil
}

type stubOgImage struct {
	head      *domain.ArticleHead
	headErr   error
	ogURL     string
	askedHead string
	askedOg   string
	savedHead string
}

func (s *stubOgImage) SaveArticleHead(_ context.Context, articleID, _, _ string) error {
	s.savedHead = articleID
	return nil
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

func newTestGateway() (*Gateway, *stubArticle, *stubOgImage, *stubDeclined) {
	article := &stubArticle{}
	og := &stubOgImage{}
	declined := &stubDeclined{}
	return New(article, og, declined), article, og, declined
}

// TestNewRequiresEveryCollaborator: this gateway satisfies one interface out of
// three sources, and a nil one would fail on whichever of the seven methods
// happened to be called first — during a user request, not at boot.
func TestNewRequiresEveryCollaborator(t *testing.T) {
	article := &stubArticle{}
	og := &stubOgImage{}
	declined := &stubDeclined{}

	tests := []struct {
		name     string
		article  articleStore
		ogImage  ogImageStore
		declined declinedDomainStore
	}{
		{name: "no article store", ogImage: og, declined: declined},
		{name: "no og image store", article: article, declined: declined},
		{name: "no declined domain store", article: article, ogImage: og},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Panics(t, func() { New(tt.article, tt.ogImage, tt.declined) })
		})
	}
}

// TestRoutingByCapabilityGroup is the attribution table for all seven methods.
//
// Batch 1 used this file to pin which half had moved and which was still a
// direct database call. That distinction is gone — every method here crosses
// the RPC boundary now — so what it pins instead is which capability gateway
// answers which method. A later batch that re-points one has to say so here.
func TestRoutingByCapabilityGroup(t *testing.T) {
	ctx := context.Background()

	t.Run("§2.B / §2.C article store", func(t *testing.T) {
		gw, article, og, declined := newTestGateway()

		_, err := gw.FetchArticleByURL(ctx, "https://example.com/post")
		require.NoError(t, err)
		assert.Equal(t, "https://example.com/post", article.fetchedURL)

		id, err := gw.SaveArticle(ctx, "https://example.com/post", "title", "body")
		require.NoError(t, err)
		assert.Equal(t, "article-1", id)
		assert.Equal(t, "https://example.com/post", article.savedURL)

		assert.Empty(t, og.askedHead)
		assert.Empty(t, declined.askedHost)
	})

	t.Run("§2.D article_heads (with §2.B's SaveArticleHead)", func(t *testing.T) {
		gw, article, og, declined := newTestGateway()
		og.head = &domain.ArticleHead{ArticleID: "a1", OgImageURL: "https://cdn.example/og.png"}
		og.ogURL = "https://cdn.example/og.png"

		require.NoError(t, gw.SaveArticleHead(ctx, "a1", "<head></head>", "https://cdn.example/og.png"))
		assert.Equal(t, "a1", og.savedHead)

		head, err := gw.FetchArticleHeadByArticleID(ctx, "a1")
		require.NoError(t, err)
		require.NotNil(t, head)
		assert.Equal(t, "a1", og.askedHead)

		url, err := gw.FetchOgImageURLByArticleID(ctx, "a2")
		require.NoError(t, err)
		assert.Equal(t, "https://cdn.example/og.png", url)
		assert.Equal(t, "a2", og.askedOg)

		assert.Empty(t, article.fetchedURL)
		assert.Empty(t, declined.askedHost)
	})

	t.Run("§2.L declined domains", func(t *testing.T) {
		gw, article, og, declined := newTestGateway()
		declined.declined = true

		got, err := gw.IsDomainDeclined(ctx, "u1", "paywalled.example")
		require.NoError(t, err)
		assert.True(t, got)
		assert.Equal(t, "u1", declined.askedUser)
		assert.Equal(t, "paywalled.example", declined.askedHost)

		require.NoError(t, gw.SaveDeclinedDomain(ctx, "u1", "paywalled.example"))
		assert.Equal(t, "paywalled.example", declined.savedHost)

		assert.Empty(t, article.fetchedURL)
		assert.Empty(t, og.askedHead)
	})
}

// TestPropagatesDataHubErrors: a failed RPC must not read as "no head found",
// which is the answer that makes the caller re-scrape the page.
func TestPropagatesDataHubErrors(t *testing.T) {
	sentinel := errors.New("data-hub unreachable")
	og := &stubOgImage{headErr: sentinel}
	gw := New(&stubArticle{}, og, &stubDeclined{})

	head, err := gw.FetchArticleHeadByArticleID(context.Background(), "a1")
	require.Error(t, err)
	assert.ErrorIs(t, err, sentinel)
	assert.Nil(t, head)
}
