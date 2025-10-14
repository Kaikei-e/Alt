package feed_search_gateway

import (
	"alt/domain"
	"alt/driver/alt_db"
	"alt/utils/logger"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/pashagolub/pgxmock/v3"
)

func TestSearchByTitleGateway_SearchFeedsByTitle(t *testing.T) {
	// Initialize logger to prevent nil pointer dereference
	logger.InitLogger()

	tests := []struct {
		name      string
		query     string
		userID    string
		mockSetup func(pgxmock.PgxPoolIface)
		want      []*domain.FeedItem
		wantErr   bool
	}{
		{
			name:   "successful search with results",
			query:  "test",
			userID: "11111111-1111-1111-1111-111111111111",
			mockSetup: func(mock pgxmock.PgxPoolIface) {
				now := time.Now()
				rows := pgxmock.NewRows([]string{"id", "title", "description", "link", "pub_date", "created_at"}).
					AddRow(uuid.New().String(), "Test Feed 1", "Test Description 1", "https://test1.com", &now, now).
					AddRow(uuid.New().String(), "Test Feed 2", "Test Description 2", "https://test2.com", (*time.Time)(nil), now)
				mock.ExpectQuery(`SELECT DISTINCT f\.id, f\.title, f\.description, f\.link, f\.pub_date, f\.created_at`).
					WithArgs("11111111-1111-1111-1111-111111111111", "%test%").
					WillReturnRows(rows)
			},
			want: []*domain.FeedItem{
				{
					Title:       "Test Feed 1",
					Link:        "https://test1.com",
					Description: "Test Description 1",
				},
				{
					Title:       "Test Feed 2",
					Link:        "https://test2.com",
					Description: "Test Description 2",
				},
			},
			wantErr: false,
		},
		{
			name:   "empty query results",
			query:  "nonexistent",
			userID: "11111111-1111-1111-1111-111111111111",
			mockSetup: func(mock pgxmock.PgxPoolIface) {
				rows := pgxmock.NewRows([]string{"id", "title", "description", "link", "pub_date", "created_at"})
				mock.ExpectQuery(`SELECT DISTINCT f\.id, f\.title, f\.description, f\.link, f\.pub_date, f\.created_at`).
					WithArgs("11111111-1111-1111-1111-111111111111", "%nonexistent%").
					WillReturnRows(rows)
			},
			want:    []*domain.FeedItem{},
			wantErr: false,
		},
		{
			name:   "database error",
			query:  "error",
			userID: "11111111-1111-1111-1111-111111111111",
			mockSetup: func(mock pgxmock.PgxPoolIface) {
				mock.ExpectQuery(`SELECT DISTINCT f\.id, f\.title, f\.description, f\.link, f\.pub_date, f\.created_at`).
					WithArgs("11111111-1111-1111-1111-111111111111", "%error%").
					WillReturnError(errors.New("database error"))
			},
			want:    nil,
			wantErr: true,
		},
		{
			name:   "empty query string",
			query:  "",
			userID: "11111111-1111-1111-1111-111111111111",
			mockSetup: func(mock pgxmock.PgxPoolIface) {
				// Empty query returns empty results without database call
			},
			want:    []*domain.FeedItem{},
			wantErr: false,
		},
		{
			name:   "whitespace only query",
			query:  "   ",
			userID: "11111111-1111-1111-1111-111111111111",
			mockSetup: func(mock pgxmock.PgxPoolIface) {
				// Whitespace query returns empty results without database call
			},
			want:    []*domain.FeedItem{},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock database
			mock, err := pgxmock.NewPool()
			if err != nil {
				t.Fatalf("Failed to create mock database: %v", err)
			}
			defer mock.Close()

			// Setup mock expectations
			tt.mockSetup(mock)

			// Create gateway with mock database
			gateway := &SearchByTitleGateway{
				alt_db: alt_db.NewAltDBRepository(mock),
			}

			// Execute test
			got, err := gateway.SearchFeedsByTitle(context.Background(), tt.query, tt.userID)

			// Check error expectation
			if (err != nil) != tt.wantErr {
				t.Errorf("SearchByTitleGateway.SearchFeedsByTitle() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			// Check results
			if !tt.wantErr {
				if len(got) != len(tt.want) {
					t.Errorf("SearchByTitleGateway.SearchFeedsByTitle() got %d results, want %d", len(got), len(tt.want))
					return
				}
				for i, item := range got {
					if i < len(tt.want) {
						if item.Title != tt.want[i].Title || item.Link != tt.want[i].Link {
							t.Errorf("SearchByTitleGateway.SearchFeedsByTitle() result[%d] = %+v, want %+v", i, item, tt.want[i])
						}
					}
				}
			}

			// Ensure all expectations were met
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Errorf("Unfulfilled mock expectations: %v", err)
			}
		})
	}
}

func TestNewSearchByTitleGateway(t *testing.T) {
	// Create mock database
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("Failed to create mock database: %v", err)
	}
	defer mock.Close()

	// Test constructor - need to cast mock to *pgxpool.Pool for the constructor
	// For testing, we create the gateway directly since NewSearchByTitleGateway expects *pgxpool.Pool
	gateway := &SearchByTitleGateway{
		alt_db: alt_db.NewAltDBRepository(mock),
	}

	// Gateway should be properly initialized
	if gateway.alt_db == nil {
		t.Error("SearchByTitleGateway alt_db should be initialized")
	}
}

func TestSearchByTitleGateway_ContextCancellation(t *testing.T) {
	// Initialize logger
	logger.InitLogger()

	// Create mock database
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("Failed to create mock database: %v", err)
	}
	defer mock.Close()

	// Setup mock to expect query but return context cancelled error
	mock.ExpectQuery(`SELECT DISTINCT f\.id, f\.title, f\.description, f\.link, f\.pub_date, f\.created_at`).
		WithArgs("11111111-1111-1111-1111-111111111111", "%test query%").
		WillReturnError(context.Canceled)

	// Create gateway
	gateway := &SearchByTitleGateway{
		alt_db: alt_db.NewAltDBRepository(mock),
	}

	// Create a cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err = gateway.SearchFeedsByTitle(ctx, "test query", "11111111-1111-1111-1111-111111111111")
	if err == nil {
		t.Error("SearchByTitleGateway.SearchFeedsByTitle() expected error with cancelled context, got nil")
	}

	// Verify expectations
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("Unfulfilled mock expectations: %v", err)
	}
}

func TestSearchByTitleGateway_EmptyResultHandling(t *testing.T) {
	// Initialize logger
	logger.InitLogger()

	// Test various query patterns that might return empty results
	testCases := []struct {
		query  string
		userID string
	}{
		{"nonexistent_feed_title_12345", "11111111-1111-1111-1111-111111111111"},
		{"", "11111111-1111-1111-1111-111111111111"},
		{"   ", "11111111-1111-1111-1111-111111111111"},
	}

	for _, tc := range testCases {
		t.Run("query: "+tc.query, func(t *testing.T) {
			// Create mock database
			mock, err := pgxmock.NewPool()
			if err != nil {
				t.Fatalf("Failed to create mock database: %v", err)
			}
			defer mock.Close()

			// Empty or whitespace queries should not make database calls
			if tc.query != "" && tc.query != "   " {
				// Setup mock to return empty results
				rows := pgxmock.NewRows([]string{"id", "title", "description", "link", "pub_date", "created_at"})
				mock.ExpectQuery(`SELECT DISTINCT f\.id, f\.title, f\.description, f\.link, f\.pub_date, f\.created_at`).
					WithArgs(tc.userID, "%"+tc.query+"%").
					WillReturnRows(rows)
			}

			// Create gateway
			gateway := &SearchByTitleGateway{
				alt_db: alt_db.NewAltDBRepository(mock),
			}

			results, err := gateway.SearchFeedsByTitle(context.Background(), tc.query, tc.userID)
			if err != nil {
				t.Errorf("SearchByTitleGateway.SearchFeedsByTitle() unexpected error: %v", err)
			}
			if len(results) != 0 {
				t.Errorf("SearchByTitleGateway.SearchFeedsByTitle() expected empty results, got %d", len(results))
			}

			// Verify expectations
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Errorf("Unfulfilled mock expectations: %v", err)
			}
		})
	}
}

func TestSearchByTitleGateway_ErrorPropagation(t *testing.T) {
	// Initialize logger
	logger.InitLogger()

	// Create mock database
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("Failed to create mock database: %v", err)
	}
	defer mock.Close()

	// Setup mock to return an error
	mock.ExpectQuery(`SELECT DISTINCT f\.id, f\.title, f\.description, f\.link, f\.pub_date, f\.created_at`).
		WithArgs("11111111-1111-1111-1111-111111111111", "%test%").
		WillReturnError(errors.New("database error"))

	// Create gateway
	gateway := &SearchByTitleGateway{
		alt_db: alt_db.NewAltDBRepository(mock),
	}

	// Test that errors from the database layer are properly propagated
	_, err = gateway.SearchFeedsByTitle(context.Background(), "test", "11111111-1111-1111-1111-111111111111")
	if err == nil {
		t.Error("SearchByTitleGateway.SearchFeedsByTitle() should propagate database errors")
	}

	// Verify expectations
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("Unfulfilled mock expectations: %v", err)
	}
}
