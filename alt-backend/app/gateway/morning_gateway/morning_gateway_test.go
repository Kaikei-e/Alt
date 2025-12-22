package morning_gateway

import (
	"alt/driver/alt_db"
	"alt/utils/logger"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/pashagolub/pgxmock/v3"
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
	mockPool, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mockPool.Close()

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

	// Gateway
	gateway := &MorningGateway{
		altDBRepository: alt_db.NewAltDBRepository(mockPool),
		httpClient: &http.Client{
			Transport: mockTransport,
		},
		recapWorkerURL: "http://recap-worker.test",
	}

	// Expect DB Query
	// Note: articles table structure - actual columns: id, feed_id, title, content, url, created_at
	// Tags are joined from article_tags and feed_tags
	rows := pgxmock.NewRows([]string{
		"id", "feed_id", "title", "content", "url", "created_at", "tags",
	}).AddRow(
		articleID, uuid.New(), "Test Title", "Content", "http://example.com", time.Now(), []string{"tag1"},
	)

	mockPool.ExpectQuery("SELECT.*FROM articles a.*WHERE a.id = ANY").
		WithArgs([]uuid.UUID{articleID}).
		WillReturnRows(rows)

	// Execute
	groups, err := gateway.GetMorningArticleGroups(context.Background(), time.Now().Add(-24*time.Hour))
	require.NoError(t, err)
	assert.Len(t, groups, 1)
	assert.Equal(t, groupID, groups[0].GroupID)
	assert.Equal(t, articleID, groups[0].ArticleID)
	assert.Equal(t, "Test Title", groups[0].Article.Title)

	require.NoError(t, mockPool.ExpectationsWereMet())
}
