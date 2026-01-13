package rest_feeds

import (
	"alt/config"
	"alt/di"
	middleware_custom "alt/middleware"
	"alt/utils/logger"
	summarizationpkg "alt/rest/rest_feeds/summarization"

	"github.com/labstack/echo/v4"
)

func RegisterFeedRoutes(v1 *echo.Group, container *di.ApplicationComponents, cfg *config.Config) {
	// 認証ミドルウェアの初期化（ヘッダベースの認証）
	authMiddleware := middleware_custom.NewAuthMiddleware(logger.Logger, cfg.Auth.SharedSecret, cfg)

	// TODO.md案A: privateグループ化で認証を適用
	// v1にまとめて適用する代わりに、feedsグループに認証ミドルウェアを適用
	feedsGroup := v1.Group("/feeds", authMiddleware.RequireAuth())

	// Private endpoints (authentication required)
	feedsGroup.GET("/fetch/single", RestHandleFetchSingleFeed(container, cfg))
	feedsGroup.GET("/fetch/list", RestHandleFetchFeedsList(container, cfg))
	feedsGroup.GET("/fetch/limit/:limit", RestHandleFetchFeedsLimit(container, cfg))
	feedsGroup.GET("/fetch/page/:page", RestHandleFetchFeedsPage(container))

	// User-specific endpoints (authentication required) - 認証必須パス
	feedsGroup.GET("/count/unreads", RestHandleUnreadCount(container))
	feedsGroup.GET("/fetch/cursor", RestHandleFetchUnreadFeedsCursor(container))
	feedsGroup.GET("/fetch/viewed/cursor", RestHandleFetchReadFeedsCursor(container))
	feedsGroup.GET("/fetch/favorites/cursor", RestHandleFetchFavoriteFeedsCursor(container))
	feedsGroup.POST("/read", RestHandleMarkFeedAsRead(container))
	feedsGroup.POST("/register/favorite", RestHandleRegisterFavoriteFeed(container))

	// Authentication needed endpoints (for personalized results)
	feedsGroup.POST("/search", RestHandleSearchFeeds(container))
	feedsGroup.POST("/fetch/details", RestHandleFetchFeedDetails(container))
	feedsGroup.GET("/stats", RestHandleFeedStats(container, cfg))
	feedsGroup.GET("/stats/detailed", RestHandleDetailedFeedStats(container, cfg))
	feedsGroup.GET("/stats/trends", RestHandleTrendStats(container, cfg))
	feedsGroup.POST("/tags", RestHandleFetchFeedTags(container))
	feedsGroup.POST("/fetch/summary/provided", RestHandleFetchInoreaderSummary(container))
	feedsGroup.POST("/fetch/summary", RestHandleFetchArticleSummary(container, cfg))

	// Article summarization endpoints
	feedsGroup.POST("/summarize", summarizationpkg.RestHandleSummarizeFeed(container, cfg))                     // Legacy synchronous endpoint
	feedsGroup.POST("/summarize/stream", summarizationpkg.RestHandleSummarizeFeedStream(container, cfg))        // Streaming endpoint (using fetch stream)
	feedsGroup.POST("/summarize/queue", summarizationpkg.RestHandleSummarizeFeedQueue(container, cfg))          // New async queue endpoint
	feedsGroup.GET("/summarize/status/:job_id", summarizationpkg.RestHandleSummarizeFeedStatus(container, cfg)) // Job status endpoint

	// RSS feed registration (require auth) - 認証ミドルウェア付きでグループ作成
	rss := v1.Group("/rss-feed-link", authMiddleware.RequireAuth())
	rss.POST("/register", RestHandleRegisterRSSFeed(container))
	rss.GET("/list", RestHandleListRSSFeedLinks(container))
	rss.DELETE("/:id", RestHandleDeleteRSSFeedLink(container))
}
