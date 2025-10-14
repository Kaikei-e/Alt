package alt_db

import (
	"context"
	"testing"
	"time"

	"github.com/pashagolub/pgxmock/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAltDBRepository_SearchFeedsByTitle(t *testing.T) {
	tests := []struct {
		name          string
		query         string
		userID        string
		mockSetup     func(pgxmock.PgxPoolIface)
		expectedCount int
		expectedError bool
	}{
		{
			name:   "successful search with results",
			query:  "tech",
			userID: "11111111-1111-1111-1111-111111111111",
			mockSetup: func(mock pgxmock.PgxPoolIface) {
				rows := pgxmock.NewRows([]string{"id", "title", "description", "link", "pub_date", "created_at"}).
					AddRow(
						"22222222-2222-2222-2222-222222222222",
						"Tech News",
						"Latest technology news",
						"https://example.com/tech",
						&time.Time{},
						time.Now(),
					).
					AddRow(
						"33333333-3333-3333-3333-333333333333",
						"TechCrunch",
						"Technology startup news",
						"https://techcrunch.com/feed",
						&time.Time{},
						time.Now(),
					)

				mock.ExpectQuery(`SELECT DISTINCT f\.id, f\.title, f\.description, f\.link, f\.pub_date, f\.created_at`).
					WithArgs("11111111-1111-1111-1111-111111111111", "%tech%").
					WillReturnRows(rows)
			},
			expectedCount: 2,
			expectedError: false,
		},
		{
			name:   "search with no results",
			query:  "nonexistent",
			userID: "11111111-1111-1111-1111-111111111111",
			mockSetup: func(mock pgxmock.PgxPoolIface) {
				rows := pgxmock.NewRows([]string{"id", "title", "description", "link", "pub_date", "created_at"})

				mock.ExpectQuery(`SELECT DISTINCT f\.id, f\.title, f\.description, f\.link, f\.pub_date, f\.created_at`).
					WithArgs("11111111-1111-1111-1111-111111111111", "%nonexistent%").
					WillReturnRows(rows)
			},
			expectedCount: 0,
			expectedError: false,
		},
		{
			name:   "case insensitive search",
			query:  "TECH",
			userID: "11111111-1111-1111-1111-111111111111",
			mockSetup: func(mock pgxmock.PgxPoolIface) {
				rows := pgxmock.NewRows([]string{"id", "title", "description", "link", "pub_date", "created_at"}).
					AddRow(
						"22222222-2222-2222-2222-222222222222",
						"Tech News",
						"Latest technology news",
						"https://example.com/tech",
						&time.Time{},
						time.Now(),
					)

				mock.ExpectQuery(`SELECT DISTINCT f\.id, f\.title, f\.description, f\.link, f\.pub_date, f\.created_at`).
					WithArgs("11111111-1111-1111-1111-111111111111", "%tech%").
					WillReturnRows(rows)
			},
			expectedCount: 1,
			expectedError: false,
		},
		{
			name:   "empty query returns empty results",
			query:  "",
			userID: "11111111-1111-1111-1111-111111111111",
			mockSetup: func(mock pgxmock.PgxPoolIface) {
				// No query should be executed for empty string
			},
			expectedCount: 0,
			expectedError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock, err := pgxmock.NewPool()
			require.NoError(t, err)
			defer mock.Close()

			tt.mockSetup(mock)

			repo := &AltDBRepository{pool: mock}
			results, err := repo.SearchFeedsByTitle(context.Background(), tt.query, tt.userID)

			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Len(t, results, tt.expectedCount)
			}

			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}
