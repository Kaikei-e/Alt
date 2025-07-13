package fetch_feed_usecase

import (
	"alt/domain"
	"alt/utils/errors"
	"context"
	stdErrors "errors"
	"testing"
)

// Mock port for testing error handling
type mockFetchSingleFeedPort struct {
	returnError error
	returnFeed  *domain.RSSFeed
}

func (m *mockFetchSingleFeedPort) FetchSingleFeed(ctx context.Context) (*domain.RSSFeed, error) {
	if m.returnError != nil {
		return nil, m.returnError
	}
	return m.returnFeed, nil
}

func TestFetchSingleFeedUsecase_EnhancedErrorHandling(t *testing.T) {
	tests := []struct {
		name       string
		portError  error
		wantErr    bool
		checkError func(t *testing.T, err error)
	}{
		{
			name: "database error gets enriched with usecase context",
			portError: errors.NewDatabaseUnavailableError(
				"gateway",
				"SingleFeedGateway",
				"FetchSingleFeed",
				stdErrors.New("connection failed"),
				map[string]interface{}{
					"host": "localhost",
				},
			),
			wantErr: true,
			checkError: func(t *testing.T, err error) {
				// Check that it's still a database error
				if !errors.IsDatabaseError(err) {
					t.Error("Expected database error to be detectable with IsDatabaseError()")
				}

				// Check that we can extract AppContextError with usecase context
				var appContextErr *errors.AppContextError
				if !stdErrors.As(err, &appContextErr) {
					t.Error("Expected error to be extractable as AppContextError")
				} else {
					if appContextErr.Layer != "usecase" {
						t.Errorf("Expected enriched layer to be 'usecase', got %s", appContextErr.Layer)
					}
					if appContextErr.Component != "FetchSingleFeedUsecase" {
						t.Errorf("Expected component to be 'FetchSingleFeedUsecase', got %s", appContextErr.Component)
					}
					if appContextErr.Operation != "Execute" {
						t.Errorf("Expected operation to be 'Execute', got %s", appContextErr.Operation)
					}

					// Check that original gateway context is preserved
					if appContextErr.Context["host"] != "localhost" {
						t.Error("Expected original gateway context to be preserved")
					}

					// Check that usecase context is added
					if appContextErr.Context["usecase_operation"] != "execute_fetch_single_feed" {
						t.Error("Expected usecase context to be added")
					}
				}
			},
		},
		{
			name: "rate limit error remains retryable after enrichment",
			portError: errors.NewRateLimitExceededError(
				"gateway",
				"SingleFeedGateway",
				"FetchSingleFeed",
				stdErrors.New("rate limit exceeded"),
				map[string]interface{}{
					"host": "api.example.com",
				},
			),
			wantErr: true,
			checkError: func(t *testing.T, err error) {
				// Check that it's still retryable after enrichment
				if !errors.IsRetryableError(err) {
					t.Error("Expected rate limit error to remain retryable after usecase enrichment")
				}

				// Check that it's still detectable as rate limit error
				if !errors.IsRateLimitError(err) {
					t.Error("Expected rate limit error to be detectable with IsRateLimitError()")
				}
			},
		},
		{
			name: "unknown error gets wrapped with usecase context",
			portError: stdErrors.New("unknown error from gateway"),
			wantErr: true,
			checkError: func(t *testing.T, err error) {
				// Check that unknown errors get wrapped properly
				var appContextErr *errors.AppContextError
				if !stdErrors.As(err, &appContextErr) {
					t.Error("Expected unknown error to be wrapped as AppContextError")
				} else {
					if appContextErr.Code != "UNKNOWN_ERROR" {
						t.Errorf("Expected error code to be 'UNKNOWN_ERROR', got %s", appContextErr.Code)
					}
					if appContextErr.Layer != "usecase" {
						t.Errorf("Expected layer to be 'usecase', got %s", appContextErr.Layer)
					}
				}

				// Check that unknown errors are not retryable
				if errors.IsRetryableError(err) {
					t.Error("Expected unknown errors to not be retryable")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockPort := &mockFetchSingleFeedPort{
				returnError: tt.portError,
			}

			usecase := NewFetchSingleFeedUsecase(mockPort)
			ctx := context.Background()

			_, err := usecase.Execute(ctx)

			if (err != nil) != tt.wantErr {
				t.Errorf("Execute() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.checkError != nil {
				tt.checkError(t, err)
			}
		})
	}
}

func TestFetchSingleFeedUsecase_SuccessfulExecution(t *testing.T) {
	mockFeed := &domain.RSSFeed{
		Title:       "Test Feed",
		Description: "Test Description",
		Items:       []domain.FeedItem{},
	}

	mockPort := &mockFetchSingleFeedPort{
		returnFeed: mockFeed,
	}

	usecase := NewFetchSingleFeedUsecase(mockPort)
	ctx := context.Background()

	result, err := usecase.Execute(ctx)

	if err != nil {
		t.Errorf("Execute() unexpected error = %v", err)
		return
	}

	if result == nil {
		t.Error("Execute() returned nil result")
		return
	}

	if result.Title != "Test Feed" {
		t.Errorf("Execute() result.Title = %v, want %v", result.Title, "Test Feed")
	}
}

func TestErrorContextEnrichment(t *testing.T) {
	// Test that errors are properly enriched with usecase context
	originalErr := errors.NewDatabaseUnavailableError(
		"gateway",
		"SingleFeedGateway",
		"FetchSingleFeed",
		stdErrors.New("connection timeout"),
		map[string]interface{}{
			"host": "localhost",
			"port": 5432,
		},
	)

	mockPort := &mockFetchSingleFeedPort{
		returnError: originalErr,
	}

	usecase := NewFetchSingleFeedUsecase(mockPort)
	ctx := context.Background()

	_, err := usecase.Execute(ctx)

	if err == nil {
		t.Fatal("Expected error but got none")
	}

	// Extract AppContextError to check context enrichment
	var appContextErr *errors.AppContextError
	if !stdErrors.As(err, &appContextErr) {
		t.Fatal("Expected error to be AppContextError")
	}

	// Check that original gateway context is preserved
	if appContextErr.Context["host"] != "localhost" {
		t.Error("Expected original gateway context to be preserved")
	}
	if appContextErr.Context["port"] != 5432 {
		t.Error("Expected original gateway context to be preserved")
	}

	// Check that usecase context is added
	if appContextErr.Context["usecase_operation"] != "execute_fetch_single_feed" {
		t.Error("Expected usecase context to be added")
	}

	// Check that layer is updated to usecase
	if appContextErr.Layer != "usecase" {
		t.Error("Expected layer to be updated to 'usecase'")
	}

	// Check that error chain preserves original sentinel error
	if !errors.IsDatabaseError(err) {
		t.Error("Expected error chain to preserve database error detection")
	}
}