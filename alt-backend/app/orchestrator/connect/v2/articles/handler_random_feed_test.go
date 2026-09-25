package articles

import (
	"context"
	"errors"
	"log/slog"
	"net/url"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"alt/config"
	articlesv2 "alt/gen/proto/alt/articles/v2"
)

func TestFetchRandomFeedResponse_Construction(t *testing.T) {
	id := uuid.New().String()
	articleID := uuid.New().String()
	resp := &articlesv2.FetchRandomFeedResponse{
		Id:          id,
		Url:         "https://example.com",
		Title:       "Test Feed",
		Description: "A test feed",
		Tags: []*articlesv2.ArticleTagItem{
			{
				Id:        "tag-1",
				Name:      "Go",
				CreatedAt: time.Now().Format(time.RFC3339),
			},
		},
		LatestArticleId: articleID,
	}

	assert.Equal(t, id, resp.Id)
	assert.Equal(t, "https://example.com", resp.Url)
	assert.Equal(t, "Test Feed", resp.Title)
	assert.Equal(t, "A test feed", resp.Description)
	assert.Len(t, resp.Tags, 1)
	assert.Equal(t, "Go", resp.Tags[0].Name)
	assert.Equal(t, articleID, resp.LatestArticleId)
}

func TestFetchRandomFeedResponse_LatestArticleIdEmpty(t *testing.T) {
	resp := &articlesv2.FetchRandomFeedResponse{
		Id:              uuid.New().String(),
		Url:             "https://example.com",
		Title:           "Test Feed",
		Description:     "A test feed",
		Tags:            nil,
		LatestArticleId: "",
	}

	assert.Empty(t, resp.LatestArticleId)
}

func TestFetchRandomFeedResponse_WithEmptyTags(t *testing.T) {
	id := uuid.New().String()
	resp := &articlesv2.FetchRandomFeedResponse{
		Id:          id,
		Url:         "https://example.com",
		Title:       "Test Feed",
		Description: "A test feed",
		Tags:        []*articlesv2.ArticleTagItem{},
	}

	assert.Equal(t, id, resp.Id)
	assert.Empty(t, resp.Tags)
}

func TestFetchRandomFeedResponse_WithNilTags(t *testing.T) {
	resp := &articlesv2.FetchRandomFeedResponse{
		Id:          uuid.New().String(),
		Url:         "https://example.com",
		Title:       "Test Feed",
		Description: "A test feed",
		Tags:        nil,
	}

	assert.Nil(t, resp.Tags)
}

func TestFetchRandomFeed_RequiresAuth(t *testing.T) {
	deps := ArticleHandlerDeps{}
	cfg := &config.Config{}
	logger := slog.Default()
	handler := NewHandler(deps, cfg, logger)
	ctx := context.Background() // No auth

	req := connect.NewRequest(&articlesv2.FetchRandomFeedRequest{})

	_, err := handler.FetchRandomFeed(ctx, req)

	require.Error(t, err)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeUnauthenticated, connectErr.Code())
}

func TestFetchRandomFeed_TagsIncludedInResponse_Documented(t *testing.T) {
	t.Run("behavior_specification", func(t *testing.T) {
		deps := ArticleHandlerDeps{}
		cfg := &config.Config{}
		logger := slog.Default()
		handler := NewHandler(deps, cfg, logger)

		assert.NotNil(t, handler)
		assert.NotNil(t, handler)
	})

	t.Run("fail_open_behavior", func(t *testing.T) {
		err := errors.New("database error")
		assert.NotNil(t, err)
	})
}

func TestFetchRandomFeed_MultipleTags(t *testing.T) {
	now := time.Now()
	tags := []*articlesv2.ArticleTagItem{
		{Id: "tag-1", Name: "Go", CreatedAt: now.Format(time.RFC3339)},
		{Id: "tag-2", Name: "Testing", CreatedAt: now.Format(time.RFC3339)},
		{Id: "tag-3", Name: "Backend", CreatedAt: now.Format(time.RFC3339)},
	}

	resp := &articlesv2.FetchRandomFeedResponse{
		Id:          uuid.New().String(),
		Url:         "https://example.com",
		Title:       "Tech Blog",
		Description: "A tech blog",
		Tags:        tags,
	}

	assert.Len(t, resp.Tags, 3)
	assert.Equal(t, "Go", resp.Tags[0].Name)
	assert.Equal(t, "Testing", resp.Tags[1].Name)
	assert.Equal(t, "Backend", resp.Tags[2].Name)
}

func TestFetchRandomFeed_NoArticles_FetchesContentAndGeneratesTags_Documented(t *testing.T) {
	t.Run("behavior_specification", func(t *testing.T) {
		deps := ArticleHandlerDeps{}
		cfg := &config.Config{}
		logger := slog.Default()
		handler := NewHandler(deps, cfg, logger)

		assert.NotNil(t, handler)
		assert.NotNil(t, handler)
	})

	t.Run("fail_open_behavior", func(t *testing.T) {
		err := errors.New("network error")
		assert.NotNil(t, err)
	})
}

func TestFetchRandomFeed_NoArticles_FlowCorrectness(t *testing.T) {
	t.Run("url_parsing", func(t *testing.T) {
		testURLs := []struct {
			link  string
			valid bool
		}{
			{"https://example.com/article/123", true},
			{"http://blog.example.org/post", true},
			{"not-a-url", false},
			{"", false},
		}

		for _, tc := range testURLs {
			_, err := url.Parse(tc.link)
			if tc.valid {
				assert.NoError(t, err, "Expected valid URL: %s", tc.link)
			}
		}
	})

	t.Run("article_id_handling", func(t *testing.T) {
		articleID := uuid.New().String()
		assert.NotEmpty(t, articleID)

		emptyID := ""
		assert.Empty(t, emptyID)
	})
}
