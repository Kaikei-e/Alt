package rest

import (
	"alt/config"
	"alt/di"
	"alt/utils/logger"
	"encoding/json"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
)

func registerSSERoutes(v1 *echo.Group, container *di.ApplicationComponents, cfg *config.Config) {
	v1.GET("/sse/feeds/stats", handleSSEFeedsStats(container, cfg))
}

func handleSSEFeedsStats(container *di.ApplicationComponents, cfg *config.Config) echo.HandlerFunc {
	return func(c echo.Context) error {
		// Set SSE headers using Echo's response
		c.Response().Header().Set("Content-Type", "text/event-stream")
		c.Response().Header().Set("Cache-Control", "no-cache")
		c.Response().Header().Set("Connection", "keep-alive")
		c.Response().Header().Set("Access-Control-Allow-Origin", "*")
		c.Response().Header().Set("Access-Control-Allow-Headers", "Cache-Control")

		// Don't let Echo write its own status
		c.Response().WriteHeader(http.StatusOK)

		// Get the underlying response writer for flushing
		w := c.Response().Writer
		flusher, canFlush := w.(http.Flusher)
		if !canFlush {
			logger.Logger.Error("Response writer doesn't support flushing")
			return c.String(http.StatusInternalServerError, "Streaming not supported")
		}

		// Send initial data
		amount, err := container.FeedAmountUsecase.Execute(c.Request().Context())
		if err != nil {
			logger.Logger.Error("Error fetching initial feed amount", "error", err)
			amount = 0
		}

		unsummarizedCount, err := container.UnsummarizedArticlesCountUsecase.Execute(c.Request().Context())
		if err != nil {
			logger.Logger.Error("Error fetching initial unsummarized articles count", "error", err)
			unsummarizedCount = 0
		}

		totalArticlesCount, err := container.TotalArticlesCountUsecase.Execute(c.Request().Context())
		if err != nil {
			logger.Logger.Error("Error fetching initial total articles count", "error", err)
			totalArticlesCount = 0
		}

		initialStats := UnsummarizedFeedStatsSummary{
			FeedAmount:             feedAmount{Amount: amount},
			UnsummarizedFeedAmount: unsummarizedFeedAmount{Amount: unsummarizedCount},
			ArticleAmount:          articleAmount{Amount: totalArticlesCount},
		}

		// Send initial data
		if jsonData, err := json.Marshal(initialStats); err == nil {
			c.Response().Write([]byte("data: " + string(jsonData) + "\n\n"))
			flusher.Flush()
		}

		// Create ticker for periodic updates
		ticker := time.NewTicker(cfg.Server.SSEInterval)
		defer ticker.Stop()

		// Create heartbeat ticker to keep connection alive (every 10 seconds)
		heartbeatTicker := time.NewTicker(10 * time.Second)
		defer heartbeatTicker.Stop()

		// Keep connection alive
		for {
			select {
			case <-heartbeatTicker.C:
				// Send heartbeat comment to keep connection alive
				_, err := c.Response().Write([]byte(": heartbeat\n\n"))
				if err != nil {
					logger.Logger.Info("Client disconnected during heartbeat", "error", err)
					return nil
				}
				flusher.Flush()

			case <-ticker.C:
				// Fetch fresh data
				amount, err := container.FeedAmountUsecase.Execute(c.Request().Context())
				if err != nil {
					logger.Logger.Error("Error fetching feed amount", "error", err)
					continue
				}

				unsummarizedCount, err := container.UnsummarizedArticlesCountUsecase.Execute(c.Request().Context())
				if err != nil {
					logger.Logger.Error("Error fetching unsummarized articles count", "error", err)
					continue
				}

				totalArticlesCount, err := container.TotalArticlesCountUsecase.Execute(c.Request().Context())
				if err != nil {
					logger.Logger.Error("Error fetching total articles count", "error", err)
					continue
				}

				stats := UnsummarizedFeedStatsSummary{
					FeedAmount:             feedAmount{Amount: amount},
					UnsummarizedFeedAmount: unsummarizedFeedAmount{Amount: unsummarizedCount},
					ArticleAmount:          articleAmount{Amount: totalArticlesCount},
				}

				// Convert to JSON and send
				jsonData, err := json.Marshal(stats)
				if err != nil {
					logger.Logger.Error("Error marshaling stats", "error", err)
					continue
				}

				// Write in SSE format
				_, err = c.Response().Write([]byte("data: " + string(jsonData) + "\n\n"))
				if err != nil {
					logger.Logger.Info("Client disconnected", "error", err)
					return nil
				}

				// Flush the data
				flusher.Flush()

			case <-c.Request().Context().Done():
				logger.Logger.Info("SSE connection closed by client")
				return nil
			}
		}
	}
}