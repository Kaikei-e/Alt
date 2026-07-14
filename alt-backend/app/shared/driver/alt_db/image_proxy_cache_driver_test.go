package alt_db

import (
	"context"
	"testing"
	"time"

	pgxmock "github.com/pashagolub/pgxmock/v5"
	"github.com/stretchr/testify/require"
)

// CleanupImageProxyCacheOlderThan enforces the copyright hard cap on cached
// image bytes by created_at (independent of each entry's TTL expires_at).
func TestImageRepository_CleanupImageProxyCacheOlderThan(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &ImageRepository{pool: mock}

	mock.ExpectExec(`(?s)DELETE FROM image_proxy_cache.*created_at <`).
		WithArgs(pgxmock.AnyArg()).
		WillReturnResult(pgxmock.NewResult("DELETE", 7))

	deleted, err := repo.CleanupImageProxyCacheOlderThan(context.Background(), 7*24*time.Hour)
	require.NoError(t, err)
	require.EqualValues(t, 7, deleted)
	require.NoError(t, mock.ExpectationsWereMet())
}
