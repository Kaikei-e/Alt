// Package feeds implements the FeedService Connect-RPC handlers.
package feeds

import (
	"context"
	"log/slog"

	"alt/config"
	"alt/domain"
	"alt/gen/proto/alt/feeds/v2/feedsv2connect"
	"alt/orchestrator/driver/preprocessor_connect"
	"alt/orchestrator/usecase/cached_feed_list_usecase"
	"alt/orchestrator/usecase/fetch_feed_stats_usecase"
	"alt/orchestrator/usecase/fetch_feed_usecase"
	"alt/orchestrator/usecase/image_proxy_usecase"
	"alt/orchestrator/usecase/reading_status"
	"alt/orchestrator/usecase/search_feed_usecase"
	"alt/orchestrator/usecase/subscription_usecase"
	"alt/shared/driver/alt_db"
	"alt/shared/usecase/create_summary_version_usecase"
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
	// Feed summary.
	//
	// ArticleStore is the article half (catalog §2.B / §2.C), served by
	// alt-data-hub since ADR-000954 Wave 3 batch 2. SummaryStore is the
	// summary *read*, which moved in batch 3 (catalog §2.H W3-H8).
	// AltDBRepository is what is left: the article_summaries write and the
	// feed-tag read, each migrating with its own capability group.
	ArticleStore         ArticleStore
	SummaryStore         SummaryStore
	AltDBRepository      *alt_db.AltDBRepository
	PreProcessorClient   *preprocessor_connect.ConnectPreProcessorClient
	CreateSummaryVersion *create_summary_version_usecase.CreateSummaryVersionUsecase
	// Shared
	ImageProxy *image_proxy_usecase.ImageProxyUsecase
}

// ArticleStore is the slice of the article capabilities StreamSummarize needs
// to resolve or create the article it is about to summarise.
//
// Naming it here rather than reaching through AltDBRepository is the point:
// these three cross a process boundary now, and a handler that still held the
// driver would be one edit away from calling a query alt-backend no longer has
// a database for.
type ArticleStore interface {
	FetchArticleByID(ctx context.Context, articleID string) (*domain.ArticleContent, error)
	FetchArticleByURL(ctx context.Context, articleURL string) (*domain.ArticleContent, error)
	SaveArticle(ctx context.Context, url, title, content string) (string, error)
}

// SummaryStore is the cached-summary read StreamSummarize consults before it
// spends a model call (capability catalog §2.H W3-H8).
//
// Named here for the same reason ArticleStore is: the handler used to reach
// through AltDBRepository for it, which is a Handler → Driver hop, and after
// the batch there is no database on this side to reach. A miss is
// (nil, nil) — the driver used to raise pgx.ErrNoRows and this path read the
// error as "no cached summary, go generate one", which over an RPC would have
// meant reporting a data plane fault for every unsummarised article.
type SummaryStore interface {
	FetchArticleSummaryByArticleID(ctx context.Context, articleID string) (*domain.FeedSummary, error)
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
