package fetch_inoreader_summary_gateway

import (
	"alt/domain"
	"alt/driver/models"
	"alt/utils/logger"
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// mockDatabase implements the Database interface for testing
type mockDatabase struct {
	expectedURLs []string
	returnData   []*models.InoreaderSummary
	returnError  error
	callCount    int
}

func (m *mockDatabase) FetchInoreaderSummariesByURLs(ctx context.Context, urls []string) ([]*models.InoreaderSummary, error) {
	m.callCount++
	if m.returnError != nil {
		return nil, m.returnError
	}
	return m.returnData, nil
}

func TestInoreaderSummaryGateway_FetchSummariesByURLs_Success(t *testing.T) {
	// Initialize logger
	logger.InitLogger()

	tests := []struct {
		name        string
		urls        []string
		mockSummaries []*models.InoreaderSummary
		want        []*domain.InoreaderSummary
		wantErr     bool
	}{
		{
			name: "successful fetch with multiple articles",
			urls: []string{"https://example.com/article1", "https://example.com/article2"},
			mockSummaries: []*models.InoreaderSummary{
				{
					ArticleURL:     "https://example.com/article1",
					Title:          "Test Article 1",
					Author:         stringPtr("Test Author 1"),
					Content:        "This is test content 1",
					ContentType:    "html",
					PublishedAt:    time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
					FetchedAt:      time.Date(2024, 1, 2, 12, 0, 0, 0, time.UTC),
					InoreaderID:    "inoreader123",
				},
				{
					ArticleURL:     "https://example.com/article2", 
					Title:          "Test Article 2",
					Author:         nil,
					Content:        "This is test content 2",
					ContentType:    "html",
					PublishedAt:    time.Date(2024, 1, 3, 12, 0, 0, 0, time.UTC),
					FetchedAt:      time.Date(2024, 1, 4, 12, 0, 0, 0, time.UTC),
					InoreaderID:    "inoreader456",
				},
			},
			want: []*domain.InoreaderSummary{
				{
					ArticleURL:     "https://example.com/article1",
					Title:          "Test Article 1", 
					Author:         stringPtr("Test Author 1"),
					Content:        "This is test content 1",
					ContentType:    "html",
					PublishedAt:    time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
					FetchedAt:      time.Date(2024, 1, 2, 12, 0, 0, 0, time.UTC),
					InoreaderID:    "inoreader123",
				},
				{
					ArticleURL:     "https://example.com/article2",
					Title:          "Test Article 2",
					Author:         nil,
					Content:        "This is test content 2", 
					ContentType:    "html",
					PublishedAt:    time.Date(2024, 1, 3, 12, 0, 0, 0, time.UTC),
					FetchedAt:      time.Date(2024, 1, 4, 12, 0, 0, 0, time.UTC),
					InoreaderID:    "inoreader456",
				},
			},
			wantErr: false,
		},
		{
			name:          "empty URLs should return empty result",
			urls:          []string{},
			mockSummaries: []*models.InoreaderSummary{},
			want:          []*domain.InoreaderSummary{},
			wantErr:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup mock database
			mockDB := &mockDatabase{
				expectedURLs: tt.urls,
				returnData:   tt.mockSummaries,
				returnError:  nil,
			}

			// Create gateway
			gateway := NewInoreaderSummaryGateway(mockDB)

			// Execute
			result, err := gateway.FetchSummariesByURLs(context.Background(), tt.urls)

			// Assert
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, len(tt.want), len(result))
				
				for i, expectedSummary := range tt.want {
					if i < len(result) {
						assert.Equal(t, expectedSummary.ArticleURL, result[i].ArticleURL)
						assert.Equal(t, expectedSummary.Title, result[i].Title)
						assert.Equal(t, expectedSummary.Content, result[i].Content)
						assert.Equal(t, expectedSummary.InoreaderID, result[i].InoreaderID)
					}
				}
			}
		})
	}
}

func TestInoreaderSummaryGateway_FetchSummariesByURLs_DatabaseError(t *testing.T) {
	// Initialize logger
	logger.InitLogger()

	// Setup mock database to return error
	urls := []string{"https://example.com/article1"}
	mockDB := &mockDatabase{
		expectedURLs: urls,
		returnData:   nil,
		returnError:  assert.AnError,
	}

	// Create gateway
	gateway := NewInoreaderSummaryGateway(mockDB)

	// Execute
	result, err := gateway.FetchSummariesByURLs(context.Background(), urls)

	// Assert
	assert.Error(t, err)
	assert.Nil(t, result)
}

// Helper function
func stringPtr(s string) *string {
	return &s
}