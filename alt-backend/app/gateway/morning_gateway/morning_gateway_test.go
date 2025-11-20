package morning_gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/pashagolub/pgxmock/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetArticleGroups(t *testing.T) {
	// Mock DB
	mockPool, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mockPool.Close()

	// Mock API
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

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/morning/updates", r.URL.Path)
		json.NewEncoder(w).Encode(apiResponse)
	}))
	defer server.Close()

	// Gateway
	gateway := &MorningGateway{
		pool:           mockPool,
		httpClient:     server.Client(),
		recapWorkerURL: server.URL,
	}

	// Expect DB Query
	rows := pgxmock.NewRows([]string{
		"id", "feed_id", "tenant_id", "title", "content", "summary", "url", "author", "language", "tags", "published_at", "created_at", "updated_at",
	}).AddRow(
		articleID, uuid.New(), uuid.New(), "Test Title", "Content", "Summary", "http://example.com", "Author", "en", []string{"tag1"}, time.Now(), time.Now(), time.Now(),
	)

	mockPool.ExpectQuery("SELECT id, .* FROM articles WHERE id = ANY").
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
