// ABOUTME: Tests for the PostgreSQL-backed API usage repository
// ABOUTME: Verifies the 100-req/day Inoreader usage counters round-trip through Postgres

package repository

import (
	"context"
	"log/slog"
	"regexp"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/pashagolub/pgxmock/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"pre-processor-sidecar/models"
)

func newTestAPIUsageRepo(t *testing.T) (*PostgreSQLAPIUsageRepository, pgxmock.PgxPoolIface) {
	t.Helper()
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	t.Cleanup(mock.Close)

	repo := &PostgreSQLAPIUsageRepository{pool: mock, logger: slog.Default()}
	return repo, mock
}

var (
	selectTodaysUsageQuery = regexp.QuoteMeta(
		`SELECT id, date, zone1_requests, zone2_requests, last_reset, rate_limit_headers
		FROM api_usage_tracking
		WHERE date = CURRENT_DATE`,
	)
	insertUsageQuery = regexp.QuoteMeta(
		`INSERT INTO api_usage_tracking (id, date, zone1_requests, zone2_requests, last_reset, rate_limit_headers)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (date) DO NOTHING`,
	)
	updateUsageQuery = regexp.QuoteMeta(
		`UPDATE api_usage_tracking
		SET zone1_requests = $2, zone2_requests = $3, last_reset = $4, rate_limit_headers = $5
		WHERE date = $1`,
	)
)

func TestGetTodaysUsage_Found(t *testing.T) {
	repo, mock := newTestAPIUsageRepo(t)

	id := uuid.New()
	today := time.Now()
	lastReset := time.Now()

	mock.ExpectQuery(selectTodaysUsageQuery).
		WillReturnRows(pgxmock.NewRows([]string{
			"id", "date", "zone1_requests", "zone2_requests", "last_reset", "rate_limit_headers",
		}).AddRow(id, today, 3, 0, lastReset, []byte(`{"X-Reader-Zone1-Limit":"100"}`)))

	usage, err := repo.GetTodaysUsage(context.Background())
	require.NoError(t, err)
	assert.Equal(t, id, usage.ID)
	assert.Equal(t, 3, usage.Zone1Requests)
	assert.Equal(t, "100", usage.RateLimitHeaders["X-Reader-Zone1-Limit"])
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGetTodaysUsage_NotFound(t *testing.T) {
	repo, mock := newTestAPIUsageRepo(t)

	mock.ExpectQuery(selectTodaysUsageQuery).WillReturnError(pgx.ErrNoRows)

	usage, err := repo.GetTodaysUsage(context.Background())
	assert.Error(t, err)
	assert.Nil(t, usage)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCreateUsageRecord(t *testing.T) {
	repo, mock := newTestAPIUsageRepo(t)
	usage := models.NewAPIUsageTracking()

	mock.ExpectExec(insertUsageQuery).
		WithArgs(usage.ID, usage.Date, usage.Zone1Requests, usage.Zone2Requests, usage.LastReset, pgxmock.AnyArg()).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))

	err := repo.CreateUsageRecord(context.Background(), usage)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUpdateUsageRecord_Success(t *testing.T) {
	repo, mock := newTestAPIUsageRepo(t)
	usage := models.NewAPIUsageTracking()
	usage.Zone1Requests = 5

	mock.ExpectExec(updateUsageQuery).
		WithArgs(usage.Date, usage.Zone1Requests, usage.Zone2Requests, usage.LastReset, pgxmock.AnyArg()).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	err := repo.UpdateUsageRecord(context.Background(), usage)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUpdateUsageRecord_NotFound(t *testing.T) {
	repo, mock := newTestAPIUsageRepo(t)
	usage := models.NewAPIUsageTracking()

	mock.ExpectExec(updateUsageQuery).
		WithArgs(usage.Date, usage.Zone1Requests, usage.Zone2Requests, usage.LastReset, pgxmock.AnyArg()).
		WillReturnResult(pgxmock.NewResult("UPDATE", 0))

	err := repo.UpdateUsageRecord(context.Background(), usage)
	assert.Error(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}
