package alt_db

import (
	"context"
	"errors"
	"fmt"
	"time"

	"alt/utils/logger"

	"github.com/google/uuid"
)

// TrendStatsRow is one time bucket of the dashboard chart.
type TrendStatsRow struct {
	Bucket       time.Time
	Articles     int
	Summarized   int
	FeedActivity int
}

// TrendStats is what FetchTrendStatsForUser returns.
//
// A driver-local type rather than trend_stats_port.TrendDataResponse. This
// file used to import that port, which pointed the dependency backwards —
// driver → orchestrator's port layer — and after ADR-000954 Wave 3 batch 5 it
// would have been worse than untidy: the caller that owns the port is in
// another process, and the type would have travelled here only to be mapped
// onto a wire message anyway.
type TrendStats struct {
	Rows []TrendStatsRow
	// "hourly" or "daily" — chosen from the window, not requested.
	Granularity string
}

// FetchTrendStatsForUser aggregates one user's articles, summaries and feed
// activity into time buckets over a window (catalog §2.M W3-M6).
//
// The window string is the provider's vocabulary and the set is closed:
// parseWindow rejects anything else, because each accepted value picks both a
// lower bound and the date_trunc unit the query groups by, and those two only
// make sense together.
func (r *DashboardRepository) FetchTrendStatsForUser(ctx context.Context, userID uuid.UUID, window string) (*TrendStats, error) {
	if userID == uuid.Nil {
		return nil, errors.New("authentication required")
	}

	windowSeconds, granularity, err := parseWindow(window)
	if err != nil {
		logger.SafeErrorContext(ctx, "invalid window parameter", "window", window, "error", err)
		return nil, fmt.Errorf("invalid window: %w", err)
	}

	if r == nil || r.pool == nil {
		return nil, errors.New("database connection pool is nil")
	}

	since := time.Now().Add(-time.Duration(windowSeconds) * time.Second)

	query, err := buildTrendQuery(granularity)
	if err != nil {
		logger.SafeErrorContext(ctx, "invalid granularity", "granularity", granularity, "error", err)
		return nil, fmt.Errorf("invalid granularity: %w", err)
	}

	rows, err := r.pool.Query(ctx, query, since, userID)
	if err != nil {
		logger.SafeErrorContext(ctx, "failed to fetch trend stats", "error", err)
		return nil, errors.New("failed to fetch trend stats")
	}
	defer rows.Close()

	var dataPoints []TrendStatsRow
	for rows.Next() {
		var bucket time.Time
		var articles, summarized, feedActivity int

		if err := rows.Scan(&bucket, &articles, &summarized, &feedActivity); err != nil {
			logger.SafeErrorContext(ctx, "failed to scan trend stats row", "error", err)
			continue
		}

		dataPoints = append(dataPoints, TrendStatsRow{
			Bucket:       bucket,
			Articles:     articles,
			Summarized:   summarized,
			FeedActivity: feedActivity,
		})
	}

	if err := rows.Err(); err != nil {
		logger.SafeErrorContext(ctx, "error iterating trend stats rows", "error", err)
		return nil, errors.New("failed to fetch trend stats")
	}

	logger.SafeInfoContext(ctx, "trend stats fetched successfully",
		"window", window,
		"granularity", granularity,
		"data_points", len(dataPoints))

	return &TrendStats{Rows: dataPoints, Granularity: granularity}, nil
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
// Returns error for invalid granularity values to prevent SQL injection via format string
func buildTrendQuery(granularity string) (string, error) {
	// Defensive validation: whitelist valid granularity values
	// This prevents SQL injection even though parseWindow already validates
	validGranularities := map[string]string{
		"hourly": "hour",
		"daily":  "day",
	}

	truncFunc, ok := validGranularities[granularity]
	if !ok {
		return "", fmt.Errorf("invalid granularity: %s", granularity)
	}

	// Query that aggregates articles, summarized articles, and feed activity by time bucket
	// Uses FULL OUTER JOINs to ensure we get counts even when some data is missing
	// Note: articles are filtered by user_id for multi-tenant isolation
	// Note: summarized count is aggregated by summary creation time and filtered by user_id
	return fmt.Sprintf(`
		WITH articles_buckets AS (
			SELECT date_trunc('%s', a.created_at) AS bucket,
				   COUNT(DISTINCT a.id) AS articles
			FROM articles a
			WHERE a.created_at >= $1
			  AND a.deleted_at IS NULL
			  AND a.user_id = $2
			GROUP BY bucket
		),
		summarized_buckets AS (
			SELECT date_trunc('%s', asumm.created_at) AS bucket,
				   COUNT(DISTINCT asumm.article_id) AS summarized
			FROM article_summaries asumm
			WHERE asumm.created_at >= $1
			  AND asumm.user_id = $2
			GROUP BY bucket
		),
		feed_activity AS (
			SELECT date_trunc('%s', f.created_at) AS bucket,
				   COUNT(*) AS feed_count
			FROM feeds f
			WHERE f.created_at >= $1
			AND f.feed_link_id IN (
				SELECT feed_link_id FROM user_feed_subscriptions WHERE user_id = $2
			)
			GROUP BY bucket
		)
		SELECT COALESCE(ab.bucket, sb.bucket, fa.bucket) AS bucket,
			   COALESCE(ab.articles, 0) AS articles,
			   COALESCE(sb.summarized, 0) AS summarized,
			   COALESCE(fa.feed_count, 0) AS feed_activity
		FROM articles_buckets ab
		FULL OUTER JOIN summarized_buckets sb ON ab.bucket = sb.bucket
		FULL OUTER JOIN feed_activity fa ON COALESCE(ab.bucket, sb.bucket) = fa.bucket
		ORDER BY bucket ASC
	`, truncFunc, truncFunc, truncFunc), nil
}
