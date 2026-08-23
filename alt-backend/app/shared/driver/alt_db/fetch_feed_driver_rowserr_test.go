package alt_db

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	pgxmock "github.com/pashagolub/pgxmock/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFetchFeedsList_RowsErrSurfaces pins the pgx rows.Err() contract: when the
// result stream is cut short mid-iteration (dropped connection, server error),
// rows.Next() returns false exactly as it does at a clean end-of-rows. Without a
// rows.Err() check the driver returns the rows read so far as if that were the
// whole page — a silent short read. The driver must surface the error instead.
func TestFetchFeedsList_RowsErrSurfaces(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &FeedRepository{pool: mock}

	now := time.Now()
	boom := errors.New("connection reset by peer mid-stream")
	rows := pgxmock.NewRows([]string{
		"id", "title", "description", "website_url", "pub_date", "created_at", "updated_at",
	}).
		AddRow(uuid.New().String(), "One", "d1", "http://example.com/1", now, now, now).
		AddRow(uuid.New().String(), "Two", "d2", "http://example.com/2", now, now, now).
		// Error at the terminal index (== row count): both rows Scan cleanly, then
		// Next() returns false and Err() returns the error — the exact shape of a
		// connection cut mid-stream that a missing rows.Err() check swallows.
		RowError(2, boom)

	mock.ExpectQuery("SELECT id, title, description, website_url, pub_date, created_at, updated_at FROM feeds").
		WillReturnRows(rows)

	_, gotErr := repo.FetchFeedsList(context.Background())
	assert.Error(t, gotErr, "a truncated result stream must surface as an error, not a short page")
}
