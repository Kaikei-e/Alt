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
		ctx := c.Request().Context()
		// Log SSE connection attempt for monitoring
		origin := c.Request().Header.Get("Origin")
		remoteAddr := c.RealIP()
		logger.Logger.InfoContext(ctx, "SSE connection attempt",
			"path", c.Path(),
			"origin", origin,
			"remote_addr", remoteAddr,
		)

		// Validate and set CORS headers - use same origins as routes.go
		allowedOrigins := []string{
			"http://localhost:3000",
			"http://localhost:80",
			"http://localhost:4173",
			"https://curionoah.com",
		}

		// Check if origin is allowed
		originAllowed := false
		if origin != "" {
			for _, allowed := range allowedOrigins {
				if origin == allowed {
					originAllowed = true
					break
				}
			}
		} else {
			// If no origin header (same-origin request), allow it
			originAllowed = true
		}

		if originAllowed && origin != "" {
			c.Response().Header().Set("Access-Control-Allow-Origin", origin)
			c.Response().Header().Set("Access-Control-Allow-Credentials", "true")
		}

		// Set SSE headers using Echo's response
		c.Response().Header().Set("Content-Type", "text/event-stream")
		c.Response().Header().Set("Cache-Control", "no-cache")
		c.Response().Header().Set("Connection", "keep-alive")
		c.Response().Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		c.Response().Header().Set("Access-Control-Allow-Headers", "Accept, Cache-Control, Cookie")
		c.Response().Header().Set("Access-Control-Expose-Headers", "Content-Type, Cache-Control")

		// Security headers
		c.Response().Header().Set("X-Content-Type-Options", "nosniff")
		c.Response().Header().Set("X-Frame-Options", "DENY")

		// Disable nginx buffering for SSE (even though nginx config has proxy_buffering off)
		c.Response().Header().Set("X-Accel-Buffering", "no")

		// Don't let Echo write its own status
		c.Response().WriteHeader(http.StatusOK)

		// Get the underlying response writer for flushing
		w := c.Response().Writer
		flusher, canFlush := w.(http.Flusher)
		if !canFlush {
			logger.Logger.ErrorContext(ctx, "Response writer doesn't support flushing")
			return c.String(http.StatusInternalServerError, "Streaming not supported")
		}

		// Send initial data
		amount, err := container.FeedAmountUsecase.Execute(ctx)
		if err != nil {
			logger.Logger.ErrorContext(ctx, "Error fetching initial feed amount", "error", err)
			amount = 0
		}

		unsummarizedCount, err := container.UnsummarizedArticlesCountUsecase.Execute(ctx)
		if err != nil {
			logger.Logger.ErrorContext(ctx, "Error fetching initial unsummarized articles count", "error", err)
			unsummarizedCount = 0
		}

		totalArticlesCount, err := container.TotalArticlesCountUsecase.Execute(ctx)
		if err != nil {
			logger.Logger.ErrorContext(ctx, "Error fetching initial total articles count", "error", err)
			totalArticlesCount = 0
		}

		initialStats := UnsummarizedFeedStatsSummary{
			FeedAmount:             feedAmount{Amount: amount},
			UnsummarizedFeedAmount: unsummarizedFeedAmount{Amount: unsummarizedCount},
			ArticleAmount:          articleAmount{Amount: totalArticlesCount},
		}

		// Send initial data
		if jsonData, err := json.Marshal(initialStats); err == nil {
			if _, writeErr := c.Response().Write([]byte("data: " + string(jsonData) + "\n\n")); writeErr != nil {
				logger.Logger.DebugContext(ctx, "Failed to send initial SSE data", "error", writeErr)
				return nil
			}
			flusher.Flush()
		}

		// Create ticker for periodic updates
		ticker := time.NewTicker(cfg.Server.SSEInterval)
		defer ticker.Stop()

		// Create heartbeat ticker to keep connection alive (every 10 seconds)
		heartbeatTicker := time.NewTicker(10 * time.Second)
		defer heartbeatTicker.Stop()

		// Track connection start time for monitoring
		connectionStartTime := time.Now()

		// Keep connection alive
		for {
			select {
			case <-heartbeatTicker.C:
				// Send heartbeat comment to keep connection alive
				_, err := c.Response().Write([]byte(": heartbeat\n\n"))
				if err != nil {
					duration := time.Since(connectionStartTime)
					logger.Logger.InfoContext(ctx, "Client disconnected during heartbeat",
						"error", err,
						"duration", duration,
						"remote_addr", remoteAddr,
					)
					return nil
				}
				flusher.Flush()

			case <-ticker.C:
				// Fetch fresh data
				amount, err := container.FeedAmountUsecase.Execute(ctx)
				if err != nil {
					logger.Logger.ErrorContext(ctx, "Error fetching feed amount", "error", err)
					continue
				}

				unsummarizedCount, err := container.UnsummarizedArticlesCountUsecase.Execute(ctx)
				if err != nil {
					logger.Logger.ErrorContext(ctx, "Error fetching unsummarized articles count", "error", err)
					continue
				}

				totalArticlesCount, err := container.TotalArticlesCountUsecase.Execute(ctx)
				if err != nil {
					logger.Logger.ErrorContext(ctx, "Error fetching total articles count", "error", err)
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
					logger.Logger.ErrorContext(ctx, "Error marshaling stats", "error", err)
					continue
				}

				// Write in SSE format
				_, err = c.Response().Write([]byte("data: " + string(jsonData) + "\n\n"))
				if err != nil {
					duration := time.Since(connectionStartTime)
					logger.Logger.InfoContext(ctx, "Client disconnected during data send",
						"error", err,
						"duration", duration,
						"remote_addr", remoteAddr,
					)
					return nil
				}

				// Flush the data
				flusher.Flush()

			case <-ctx.Done():
				duration := time.Since(connectionStartTime)
				logger.Logger.InfoContext(ctx, "SSE connection closed by client",
					"duration", duration,
					"remote_addr", remoteAddr,
				)
				return nil
			}
		}
	}
}
