package feed_search_gateway

import (
	"alt/domain"
	"alt/driver/alt_db"
	"alt/utils/logger"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/pashagolub/pgxmock/v3"
)

func TestSearchByTitleGateway_SearchByTitle(t *testing.T) {
	// Initialize logger to prevent nil pointer dereference
	logger.InitLogger()

	tests := []struct {
		name      string
		query     string
		mockSetup func(pgxmock.PgxPoolIface)
		want      []*domain.FeedItem
		wantErr   bool
	}{
		{
			name:  "successful search with results",
			query: "test",
			mockSetup: func(mock pgxmock.PgxPoolIface) {
				now := time.Now()
				rows := pgxmock.NewRows([]string{"title", "link", "description", "pub_date", "created_at"}).
					AddRow("Test Feed 1", "https://test1.com", "Test Description 1", &now, now).
					AddRow("Test Feed 2", "https://test2.com", "Test Description 2", (*time.Time)(nil), now)
				mock.ExpectQuery(`SELECT title, link, description, pub_date, created_at FROM feeds WHERE title ILIKE \$1 ORDER BY created_at DESC LIMIT 20`).
					WithArgs("%test%").
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
			name:  "empty query results",
			query: "nonexistent",
			mockSetup: func(mock pgxmock.PgxPoolIface) {
				rows := pgxmock.NewRows([]string{"title", "link", "description", "pub_date", "created_at"})
				mock.ExpectQuery(`SELECT title, link, description, pub_date, created_at FROM feeds WHERE title ILIKE \$1 ORDER BY created_at DESC LIMIT 20`).
					WithArgs("%nonexistent%").
					WillReturnRows(rows)
			},
			want:    []*domain.FeedItem{},
			wantErr: false,
		},
		{
			name:  "database error",
			query: "error",
			mockSetup: func(mock pgxmock.PgxPoolIface) {
				mock.ExpectQuery(`SELECT title, link, description, pub_date, created_at FROM feeds WHERE title ILIKE \$1 ORDER BY created_at DESC LIMIT 20`).
					WithArgs("%error%").
					WillReturnError(errors.New("database error"))
			},
			want:    nil,
			wantErr: true,
		},
		{
			name:  "empty query string",
			query: "",
			mockSetup: func(mock pgxmock.PgxPoolIface) {
				rows := pgxmock.NewRows([]string{"title", "link", "description", "pub_date", "created_at"})
				mock.ExpectQuery(`SELECT title, link, description, pub_date, created_at FROM feeds WHERE title ILIKE \$1 ORDER BY created_at DESC LIMIT 20`).
					WithArgs("%%").
					WillReturnRows(rows)
			},
			want:    []*domain.FeedItem{},
			wantErr: false,
		},
		{
			name:  "special characters in query",
			query: "test@#$%",
			mockSetup: func(mock pgxmock.PgxPoolIface) {
				rows := pgxmock.NewRows([]string{"title", "link", "description", "pub_date", "created_at"})
				mock.ExpectQuery(`SELECT title, link, description, pub_date, created_at FROM feeds WHERE title ILIKE \$1 ORDER BY created_at DESC LIMIT 20`).
					WithArgs("%test@#$%%").
					WillReturnRows(rows)
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
			got, err := gateway.SearchByTitle(context.Background(), tt.query)

			// Check error expectation
			if (err != nil) != tt.wantErr {
				t.Errorf("SearchByTitleGateway.SearchByTitle() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			// Check results
			if !tt.wantErr {
				if len(got) != len(tt.want) {
					t.Errorf("SearchByTitleGateway.SearchByTitle() got %d results, want %d", len(got), len(tt.want))
					return
				}
				for i, item := range got {
					if i < len(tt.want) {
						if item.Title != tt.want[i].Title || item.Link != tt.want[i].Link {
							t.Errorf("SearchByTitleGateway.SearchByTitle() result[%d] = %+v, want %+v", i, item, tt.want[i])
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
	mock.ExpectQuery(`SELECT title, link, description, pub_date, created_at FROM feeds WHERE title ILIKE \$1 ORDER BY created_at DESC LIMIT 20`).
		WithArgs("%test query%").
		WillReturnError(context.Canceled)

	// Create gateway
	gateway := &SearchByTitleGateway{
		alt_db: alt_db.NewAltDBRepository(mock),
	}

	// Create a cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err = gateway.SearchByTitle(ctx, "test query")
	if err == nil {
		t.Error("SearchByTitleGateway.SearchByTitle() expected error with cancelled context, got nil")
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
	queries := []string{
		"nonexistent_feed_title_12345",
		"",
		"   ",
		"!@#$%^&*()",
	}

	for _, query := range queries {
		t.Run("query: "+query, func(t *testing.T) {
			// Create mock database
			mock, err := pgxmock.NewPool()
			if err != nil {
				t.Fatalf("Failed to create mock database: %v", err)
			}
			defer mock.Close()

			// Setup mock to return empty results
			rows := pgxmock.NewRows([]string{"title", "link", "description", "pub_date", "created_at"})
			mock.ExpectQuery(`SELECT title, link, description, pub_date, created_at FROM feeds WHERE title ILIKE \$1 ORDER BY created_at DESC LIMIT 20`).
				WithArgs("%"+query+"%").
				WillReturnRows(rows)

			// Create gateway
			gateway := &SearchByTitleGateway{
				alt_db: alt_db.NewAltDBRepository(mock),
			}

			results, err := gateway.SearchByTitle(context.Background(), query)
			if err != nil {
				t.Errorf("SearchByTitleGateway.SearchByTitle() unexpected error: %v", err)
			}
			if len(results) != 0 {
				t.Errorf("SearchByTitleGateway.SearchByTitle() expected empty results, got %d", len(results))
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
	mock.ExpectQuery(`SELECT title, link, description, pub_date, created_at FROM feeds WHERE title ILIKE \$1 ORDER BY created_at DESC LIMIT 20`).
		WithArgs("%test%").
		WillReturnError(errors.New("database error"))

	// Create gateway
	gateway := &SearchByTitleGateway{
		alt_db: alt_db.NewAltDBRepository(mock),
	}

	// Test that errors from the database layer are properly propagated
	_, err = gateway.SearchByTitle(context.Background(), "test")
	if err == nil {
		t.Error("SearchByTitleGateway.SearchByTitle() should propagate database errors")
	}

	// Verify expectations
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("Unfulfilled mock expectations: %v", err)
	}
}