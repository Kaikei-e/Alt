package morning_gateway

import (
	"alt/domain"
	"alt/utils/logger"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestGetArticleGroups(t *testing.T) {
	// Initialize logger for testing to prevent nil pointer dereference
	logger.InitLogger()
	// Mock DB
	// Mock API response payload
	groupID := uuid.New()
	articleID := uuid.New()
	createdAt := time.Now().UTC()

	apiResponse := []MorningArticleGroupResponse{
		{
			GroupID:   groupID,
			ArticleID: articleID,
			IsPrimary: true,
			CreatedAt: createdAt,
		},
	}

	mockTransport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		assert.Equal(t, "/v1/morning/updates", r.URL.Path)
		bodyBytes, err := json.Marshal(apiResponse)
		require.NoError(t, err)

		resp := &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(bytes.NewReader(bodyBytes)),
		}
		resp.Header.Set("Content-Type", "application/json")
		return resp, nil
	})

	// Gateway. The article read crosses to alt-data-hub since ADR-000954
	// Wave 3 batch 2, so the stub stands where a pgx mock used to.
	gateway := &MorningGateway{
		articles: stubArticleBatchReader{articles: []*domain.Article{{
			ID:        articleID,
			FeedID:    uuid.New(),
			Title:     "Test Title",
			Content:   "Content",
			URL:       "http://example.com",
			CreatedAt: time.Now(),
			Tags:      []string{"tag1"},
		}}},
		httpClient: &http.Client{
			Transport: mockTransport,
		},
		recapWorkerURL: "http://recap-worker.test",
	}

	// Execute
	groups, err := gateway.GetMorningArticleGroups(context.Background(), time.Now().Add(-24*time.Hour))
	require.NoError(t, err)
	assert.Len(t, groups, 1)
	assert.Equal(t, groupID, groups[0].GroupID)
	assert.Equal(t, articleID, groups[0].ArticleID)
	assert.Equal(t, "Test Title", groups[0].Article.Title)
}

func TestBuildMorningUpdatesURL(t *testing.T) {
	since := time.Date(2026, 9, 25, 0, 0, 0, 0, time.UTC)
	u := buildMorningUpdatesURL("http://recap-worker:9005", since)
	assert.Contains(t, u, "http://recap-worker:9005/v1/morning/updates?since=2026-09-25T00%3A00%3A00Z")
}

func TestExtractArticleIDs(t *testing.T) {
	id1 := uuid.New()
	id2 := uuid.New()
	resps := []MorningArticleGroupResponse{
		{ArticleID: id1},
		{ArticleID: id2},
	}
	ids := extractArticleIDs(resps)
	assert.Equal(t, []uuid.UUID{id1, id2}, ids)
}

func TestMapArticleGroupToDomain(t *testing.T) {
	id1 := uuid.New()
	gid := uuid.New()
	now := time.Now().UTC()
	resp := MorningArticleGroupResponse{
		GroupID:   gid,
		ArticleID: id1,
		IsPrimary: true,
		CreatedAt: now,
	}
	article := &domain.Article{ID: id1, Title: "Art 1"}

	group := mapArticleGroupToDomain(resp, article)
	assert.Equal(t, id1, group.ArticleID)
	assert.Equal(t, gid, group.GroupID)
	assert.True(t, group.IsPrimary)
	assert.Equal(t, now, group.CreatedAt)
	assert.Equal(t, article, group.Article)
}
