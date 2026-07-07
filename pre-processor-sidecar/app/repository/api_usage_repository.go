// ABOUTME: PostgreSQL implementation of the InoreaderService APIUsageRepository interface
// ABOUTME: Persists the daily Zone1/Zone2 counters backing the 100-req/day rate limit

package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"

	"pre-processor-sidecar/models"
)

// PostgreSQLAPIUsageRepository implements APIUsageRepository using PostgreSQL.
type PostgreSQLAPIUsageRepository struct {
	pool   PgxIface
	logger *slog.Logger
}

// NewPostgreSQLAPIUsageRepository creates a new PostgreSQL-backed API usage repository.
func NewPostgreSQLAPIUsageRepository(pool PgxIface, logger *slog.Logger) APIUsageRepository {
	return &PostgreSQLAPIUsageRepository{
		pool:   pool,
		logger: logger,
	}
}

// GetTodaysUsage retrieves today's usage record.
func (r *PostgreSQLAPIUsageRepository) GetTodaysUsage(ctx context.Context) (*models.APIUsageTracking, error) {
	query := `SELECT id, date, zone1_requests, zone2_requests, last_reset, rate_limit_headers
		FROM api_usage_tracking
		WHERE date = CURRENT_DATE`

	var usage models.APIUsageTracking
	var headers []byte
	err := r.pool.QueryRow(ctx, query).Scan(
		&usage.ID,
		&usage.Date,
		&usage.Zone1Requests,
		&usage.Zone2Requests,
		&usage.LastReset,
		&headers,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("no api usage record found for today: %w", err)
		}
		return nil, fmt.Errorf("failed to get today's api usage: %w", err)
	}

	usage.RateLimitHeaders = make(map[string]interface{})
	if len(headers) > 0 {
		if err := json.Unmarshal(headers, &usage.RateLimitHeaders); err != nil {
			r.logger.Warn("failed to decode api usage rate limit headers", "error", err)
			usage.RateLimitHeaders = make(map[string]interface{})
		}
	}

	return &usage, nil
}

// CreateUsageRecord inserts a new usage record for the day.
func (r *PostgreSQLAPIUsageRepository) CreateUsageRecord(ctx context.Context, usage *models.APIUsageTracking) error {
	headers, err := json.Marshal(usage.RateLimitHeaders)
	if err != nil {
		return fmt.Errorf("failed to encode api usage rate limit headers: %w", err)
	}

	query := `INSERT INTO api_usage_tracking (id, date, zone1_requests, zone2_requests, last_reset, rate_limit_headers)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (date) DO NOTHING`

	if _, err := r.pool.Exec(ctx, query,
		usage.ID, usage.Date, usage.Zone1Requests, usage.Zone2Requests, usage.LastReset, headers,
	); err != nil {
		r.logger.Error("Failed to create api usage record", "error", err)
		return fmt.Errorf("failed to create api usage record: %w", err)
	}

	return nil
}

// UpdateUsageRecord updates the usage record for the given date.
func (r *PostgreSQLAPIUsageRepository) UpdateUsageRecord(ctx context.Context, usage *models.APIUsageTracking) error {
	headers, err := json.Marshal(usage.RateLimitHeaders)
	if err != nil {
		return fmt.Errorf("failed to encode api usage rate limit headers: %w", err)
	}

	query := `UPDATE api_usage_tracking
		SET zone1_requests = $2, zone2_requests = $3, last_reset = $4, rate_limit_headers = $5
		WHERE date = $1`

	tag, err := r.pool.Exec(ctx, query, usage.Date, usage.Zone1Requests, usage.Zone2Requests, usage.LastReset, headers)
	if err != nil {
		r.logger.Error("Failed to update api usage record", "error", err)
		return fmt.Errorf("failed to update api usage record: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("api usage record not found for update: %s", usage.Date.Format("2006-01-02"))
	}

	return nil
}
