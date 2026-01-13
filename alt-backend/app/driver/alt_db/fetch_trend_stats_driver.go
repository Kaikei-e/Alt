package alt_db

import (
	"context"
	"errors"
	"fmt"
	"time"

	"alt/domain"
	"alt/port/trend_stats_port"
	"alt/utils/logger"
)

// TrendStatsRow represents a single row from the trend stats query
type TrendStatsRow struct {
	Bucket       time.Time
	Articles     int
	Summarized   int
	FeedActivity int
}

// FetchTrendStats fetches trend statistics for the given time window
func (r *AltDBRepository) FetchTrendStats(ctx context.Context, window string) (*trend_stats_port.TrendDataResponse, error) {
	// Validate user context and extract user_id for multi-tenant filtering
	user, err := domain.GetUserFromContext(ctx)
	if err != nil {
		logger.SafeError("user context not found for trend stats", "error", err)
		return nil, errors.New("authentication required")
	}

	// Parse window
	windowSeconds, granularity, err := parseWindow(window)
	if err != nil {
		logger.SafeError("invalid window parameter", "window", window, "error", err)
		return nil, fmt.Errorf("invalid window: %w", err)
	}

	// Check if pool is nil
	if r.pool == nil {
		return nil, errors.New("database connection pool is nil")
	}

	// Calculate since time
	since := time.Now().Add(-time.Duration(windowSeconds) * time.Second)

	// Build and execute query with user_id filter for multi-tenant isolation
	query := buildTrendQuery(granularity)

	rows, err := r.pool.Query(ctx, query, since, user.UserID)
	if err != nil {
		logger.SafeError("failed to fetch trend stats", "error", err)
		return nil, errors.New("failed to fetch trend stats")
	}
	defer rows.Close()

	var dataPoints []trend_stats_port.TrendDataPoint
	for rows.Next() {
		var bucket time.Time
		var articles, summarized, feedActivity int

		if err := rows.Scan(&bucket, &articles, &summarized, &feedActivity); err != nil {
			logger.SafeError("failed to scan trend stats row", "error", err)
			continue
		}

		dataPoints = append(dataPoints, trend_stats_port.TrendDataPoint{
			Timestamp:    bucket,
			Articles:     articles,
			Summarized:   summarized,
			FeedActivity: feedActivity,
		})
	}

	if err := rows.Err(); err != nil {
		logger.SafeError("error iterating trend stats rows", "error", err)
		return nil, errors.New("failed to fetch trend stats")
	}

	logger.SafeInfo("trend stats fetched successfully",
		"window", window,
		"granularity", granularity,
		"data_points", len(dataPoints))

	return &trend_stats_port.TrendDataResponse{
		DataPoints:  dataPoints,
		Granularity: granularity,
		Window:      window,
	}, nil
}

// parseWindow parses the window string and returns duration in seconds and granularity
func parseWindow(window string) (int, string, error) {
	switch window {
	case "4h":
		return 4 * 3600, "hourly", nil
	case "24h":
		return 24 * 3600, "hourly", nil
	case "3d":
		return 3 * 24 * 3600, "daily", nil
	case "7d":
		return 7 * 24 * 3600, "daily", nil
	default:
		return 0, "", fmt.Errorf("unsupported window: %s", window)
	}
}

// buildTrendQuery builds the SQL query for trend stats
// Parameters: $1 = since (timestamp), $2 = user_id (UUID)
func buildTrendQuery(granularity string) string {
	truncFunc := "hour"
	if granularity == "daily" {
		truncFunc = "day"
	}

	// Query that aggregates articles, summarized articles, and feed activity by time bucket
	// Uses LEFT JOINs to ensure we get counts even when some data is missing
	// Note: articles are filtered by user_id for multi-tenant isolation
	return fmt.Sprintf(`
		WITH time_buckets AS (
			SELECT date_trunc('%s', a.created_at) AS bucket,
				   COUNT(DISTINCT a.id) AS articles,
				   COUNT(DISTINCT CASE WHEN a.summarized = true THEN a.id END) AS summarized
			FROM articles a
			WHERE a.created_at >= $1
			  AND a.deleted_at IS NULL
			  AND a.user_id = $2
			GROUP BY bucket
		),
		feed_activity AS (
			SELECT date_trunc('%s', created_at) AS bucket,
				   COUNT(*) AS feed_count
			FROM feeds
			WHERE created_at >= $1
			GROUP BY bucket
		)
		SELECT COALESCE(tb.bucket, fa.bucket) AS bucket,
			   COALESCE(tb.articles, 0) AS articles,
			   COALESCE(tb.summarized, 0) AS summarized,
			   COALESCE(fa.feed_count, 0) AS feed_activity
		FROM time_buckets tb
		FULL OUTER JOIN feed_activity fa ON tb.bucket = fa.bucket
		ORDER BY bucket ASC
	`, truncFunc, truncFunc)
}
