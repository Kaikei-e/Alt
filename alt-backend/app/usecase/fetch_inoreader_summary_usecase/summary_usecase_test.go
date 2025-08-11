package fetch_inoreader_summary_usecase

import (
	"alt/domain"
	"alt/mocks"
	"alt/utils/logger"
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestFetchInoreaderSummaryUsecase_Execute_Success(t *testing.T) {
	logger.InitLogger()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPort := mocks.NewMockFetchInoreaderSummaryPort(ctrl)

	tests := []struct {
		name        string
		urls        []string
		mockResult  []*domain.InoreaderSummary
		want        []*domain.InoreaderSummary
		wantErr     bool
	}{
		{
			name: "successful fetch with multiple articles",
			urls: []string{"https://example.com/article1", "https://example.com/article2"},
			mockResult: []*domain.InoreaderSummary{
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
			},
			wantErr: false,
		},
		{
			name:        "empty URLs should return empty result",
			urls:        []string{},
			mockResult:  []*domain.InoreaderSummary{},
			want:        []*domain.InoreaderSummary{},
			wantErr:     false,
		},
		{
			name:        "too many URLs should return error",
			urls:        make([]string, 51), // More than 50
			mockResult:  nil,
			want:        nil,
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup mock expectations only for valid cases that actually call the port
			if !tt.wantErr && len(tt.urls) > 0 && len(tt.urls) <= 50 {
				mockPort.EXPECT().
					FetchSummariesByURLs(gomock.Any(), tt.urls).
					Return(tt.mockResult, nil).
					Times(1)
			}

			// Create usecase - this should fail since we haven't implemented it yet
			usecase := NewFetchInoreaderSummaryUsecase(mockPort)

			// Execute
			result, err := usecase.Execute(context.Background(), tt.urls)

			// Assert
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, len(tt.want), len(result))
				
				for i, expected := range tt.want {
					if i < len(result) {
						assert.Equal(t, expected.ArticleURL, result[i].ArticleURL)
						assert.Equal(t, expected.Title, result[i].Title)
						assert.Equal(t, expected.Content, result[i].Content)
						assert.Equal(t, expected.InoreaderID, result[i].InoreaderID)
					}
				}
			}
		})
	}
}

func TestFetchInoreaderSummaryUsecase_Execute_URLValidation(t *testing.T) {
	logger.InitLogger()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPort := mocks.NewMockFetchInoreaderSummaryPort(ctrl)

	tests := []struct {
		name    string
		urls    []string
		wantErr bool
	}{
		{
			name:    "valid HTTPS URLs should pass",
			urls:    []string{"https://example.com/article1"},
			wantErr: false,
		},
		{
			name:    "valid HTTP URLs should pass",
			urls:    []string{"http://example.com/article1"},
			wantErr: false,
		},
		{
			name:    "localhost should be rejected for security",
			urls:    []string{"http://localhost/article1"},
			wantErr: true,
		},
		{
			name:    "private IP should be rejected for security",
			urls:    []string{"http://192.168.1.1/article1"},
			wantErr: true,
		},
		{
			name:    "invalid scheme should be rejected",
			urls:    []string{"ftp://example.com/article1"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup mock expectation only for valid cases
			if !tt.wantErr {
				mockPort.EXPECT().
					FetchSummariesByURLs(gomock.Any(), tt.urls).
					Return([]*domain.InoreaderSummary{}, nil).
					Times(1)
			}

			usecase := NewFetchInoreaderSummaryUsecase(mockPort)
			_, err := usecase.Execute(context.Background(), tt.urls)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestFetchInoreaderSummaryUsecase_Execute_PortError(t *testing.T) {
	logger.InitLogger()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPort := mocks.NewMockFetchInoreaderSummaryPort(ctrl)

	// Setup mock to return error
	urls := []string{"https://example.com/article1"}
	mockPort.EXPECT().
		FetchSummariesByURLs(gomock.Any(), urls).
		Return(nil, assert.AnError).
		Times(1)

	usecase := NewFetchInoreaderSummaryUsecase(mockPort)
	result, err := usecase.Execute(context.Background(), urls)

	assert.Error(t, err)
	assert.Nil(t, result)
}

// Helper function
func stringPtr(s string) *string {
	return &s
}