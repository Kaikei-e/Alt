package rest_feeds

import (
	"fmt"
	"net/http"
	"time"

	"alt/config"
	"alt/di"
	"alt/utils/logger"

	"github.com/labstack/echo/v4"
)

// TrendDataPointResponse represents a single data point in the trend chart response
type TrendDataPointResponse struct {
	Timestamp    string `json:"timestamp"`
	Articles     int    `json:"articles"`
	Summarized   int    `json:"summarized"`
	FeedActivity int    `json:"feed_activity"`
}

// TrendStatsResponse represents the complete trend stats response
type TrendStatsResponse struct {
	DataPoints  []TrendDataPointResponse `json:"data_points"`
	Granularity string                   `json:"granularity"`
	Window      string                   `json:"window"`
}

// RestHandleTrendStats handles the trend stats endpoint
func RestHandleTrendStats(container *di.ApplicationComponents, cfg *config.Config) echo.HandlerFunc {
	return func(c echo.Context) error {
		// Get window parameter (default to 24h)
		window := c.QueryParam("window")
		if window == "" {
			window = "24h"
		}

		// Validate window parameter
		validWindows := map[string]bool{
			"4h":  true,
			"24h": true,
			"3d":  true,
			"7d":  true,
		}
		if !validWindows[window] {
			logger.Logger.Warn("Invalid window parameter",
				"window", window)
			return c.JSON(http.StatusBadRequest, map[string]string{
				"error": "Invalid window parameter. Valid values: 4h, 24h, 3d, 7d",
			})
		}

		// Add caching headers
		c.Response().Header().Set("Cache-Control", fmt.Sprintf("public, max-age=%d", int(cfg.Cache.FeedCacheExpiry.Seconds())))
		c.Response().Header().Set("ETag", fmt.Sprintf(`"trends-%s"`, window))

		// Fetch trend stats
		ctx := c.Request().Context()
		result, err := container.TrendStatsUsecase.Execute(ctx, window)
		if err != nil {
			logger.Logger.Error("Error fetching trend stats",
				"error", err,
				"window", window)
			return c.JSON(http.StatusInternalServerError, map[string]string{
				"error": "Failed to fetch trend statistics",
			})
		}

		// Convert to response format
		dataPoints := make([]TrendDataPointResponse, len(result.DataPoints))
		for i, dp := range result.DataPoints {
			dataPoints[i] = TrendDataPointResponse{
				Timestamp:    dp.Timestamp.Format(time.RFC3339),
				Articles:     dp.Articles,
				Summarized:   dp.Summarized,
				FeedActivity: dp.FeedActivity,
			}
		}

		response := TrendStatsResponse{
			DataPoints:  dataPoints,
			Granularity: result.Granularity,
			Window:      result.Window,
		}

		logger.Logger.Info("Trend stats retrieved successfully",
			"window", window,
			"data_points_count", len(dataPoints),
			"granularity", result.Granularity)

		return c.JSON(http.StatusOK, response)
	}
}
