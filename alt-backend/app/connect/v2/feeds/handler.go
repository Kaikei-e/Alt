// Package feeds implements the FeedService Connect-RPC handlers.
package feeds

import (
	"log/slog"

	"alt/config"
	"alt/domain"
	"alt/driver/alt_db"
	"alt/driver/preprocessor_connect"
	"alt/gen/proto/alt/feeds/v2/feedsv2connect"
	"alt/usecase/cached_feed_list_usecase"
	"alt/usecase/create_summary_version_usecase"
	"alt/usecase/fetch_feed_stats_usecase"
	"alt/usecase/fetch_feed_usecase"
	"alt/usecase/image_proxy_usecase"
	"alt/usecase/reading_status"
	"alt/usecase/search_feed_usecase"
	"alt/usecase/subscription_usecase"
)

// FeedHandlerDeps holds all dependencies for the Feed service handler.
type FeedHandlerDeps struct {
	// Feed read
	CachedFeedList           *cached_feed_list_usecase.CachedFeedListUsecase
	FetchReadFeedsCursor     *fetch_feed_usecase.FetchReadFeedsListCursorUsecase
	FetchFavoriteFeedsCursor *fetch_feed_usecase.FetchFavoriteFeedsListCursorUsecase
	FeedSearch               *search_feed_usecase.SearchFeedMeilisearchUsecase
	ListSubscriptions        *subscription_usecase.ListSubscriptionsUsecase
	// Feed write
	ArticlesReadingStatus *reading_status.ArticlesReadingStatusUsecase
	Subscribe             *subscription_usecase.SubscribeUsecase
	Unsubscribe           *subscription_usecase.UnsubscribeUsecase
	// Feed stats
	FeedAmount        *fetch_feed_stats_usecase.FeedsCountUsecase
	UnsummarizedCount *fetch_feed_stats_usecase.UnsummarizedArticlesCountUsecase
	SummarizedCount   *fetch_feed_stats_usecase.SummarizedArticlesCountUsecase
	TotalCount        *fetch_feed_stats_usecase.TotalArticlesCountUsecase
	TodayUnreadCount  *fetch_feed_stats_usecase.TodayUnreadArticlesCountUsecase
	// Feed summary
	AltDBRepository      *alt_db.AltDBRepository
	PreProcessorClient   *preprocessor_connect.ConnectPreProcessorClient
	CreateSummaryVersion *create_summary_version_usecase.CreateSummaryVersionUsecase
	// Shared
	ImageProxy *image_proxy_usecase.ImageProxyUsecase
}

// Handler implements the FeedService Connect-RPC service.
type Handler struct {
	deps   FeedHandlerDeps
	logger *slog.Logger
	cfg    *config.Config
}

// NewHandler creates a new Feed service handler.
func NewHandler(deps FeedHandlerDeps, cfg *config.Config, logger *slog.Logger) *Handler {
	return &Handler{
		deps:   deps,
		logger: logger,
		cfg:    cfg,
	}
}

// Verify interface implementation at compile time.
var _ feedsv2connect.FeedServiceHandler = (*Handler)(nil)

// enrichWithProxyURLs sets OgImageProxyURL on each feed item using the image proxy signer.
func (h *Handler) enrichWithProxyURLs(feeds []*domain.FeedItem) {
	if h.deps.ImageProxy == nil {
		return
	}
	for _, feed := range feeds {
		if feed.OgImageURL != "" {
			feed.OgImageProxyURL = h.deps.ImageProxy.GenerateProxyURL(feed.OgImageURL)
		}
	}
}
