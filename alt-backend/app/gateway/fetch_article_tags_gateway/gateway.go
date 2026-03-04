package fetch_article_tags_gateway

import (
	"alt/domain"
	"alt/driver/alt_db"
	"alt/driver/mqhub_connect"
	"alt/utils/logger"
	"context"
	"time"

	"golang.org/x/sync/singleflight"
)

// tagGenerator abstracts the mq-hub client for testability.
type tagGenerator interface {
	GenerateTagsForArticle(ctx context.Context, req mqhub_connect.GenerateTagsRequest) (*mqhub_connect.GenerateTagsResponse, error)
	IsEnabled() bool
}

// articleDB abstracts DB operations needed by this gateway.
type articleDB interface {
	FetchArticleTags(ctx context.Context, articleID string) ([]*domain.FeedTag, error)
	FetchArticleByID(ctx context.Context, articleID string) (*domain.ArticleContent, error)
	UpsertArticleTags(ctx context.Context, articleID string, feedID string, tags []alt_db.TagUpsertItem) (int32, error)
}

// Config holds configuration for the gateway.
type Config struct {
	// OnTheFlyEnabled enables on-the-fly tag generation when no tags exist.
	OnTheFlyEnabled bool
	// TagGenerationTimeoutMs is the timeout for tag generation in milliseconds.
	TagGenerationTimeoutMs int32
	// MaxRetries is the number of retry attempts for tag generation (0 = no retry).
	MaxRetries int
	// RetryBackoff is the base backoff duration between retries.
	RetryBackoff time.Duration
}

// DefaultConfig returns the default configuration.
func DefaultConfig() Config {
	return Config{
		OnTheFlyEnabled:        true,
		TagGenerationTimeoutMs: 60000, // 60 seconds
		MaxRetries:             1,     // 1 retry = 2 total attempts
		RetryBackoff:           500 * time.Millisecond,
	}
}

// FetchArticleTagsGateway implements the port for fetching article tags.
type FetchArticleTagsGateway struct {
	db      articleDB
	tagger  tagGenerator
	config  Config
	sfGroup singleflight.Group // deduplicates concurrent on-the-fly generation for the same articleID
}

// NewFetchArticleTagsGateway creates a new gateway instance.
func NewFetchArticleTagsGateway(altDB *alt_db.AltDBRepository) *FetchArticleTagsGateway {
	return &FetchArticleTagsGateway{
		db:     altDB,
		config: DefaultConfig(),
	}
}

// NewFetchArticleTagsGatewayWithMQHub creates a new gateway with mq-hub client for on-the-fly tag generation.
func NewFetchArticleTagsGatewayWithMQHub(
	altDB *alt_db.AltDBRepository,
	mqhubClient *mqhub_connect.Client,
	config Config,
) *FetchArticleTagsGateway {
	return &FetchArticleTagsGateway{
		db:     altDB,
		tagger: mqhubClient,
		config: config,
	}
}

// newGateway is an internal constructor for testing with interfaces.
func newGateway(db articleDB, tagger tagGenerator, config Config) *FetchArticleTagsGateway {
	return &FetchArticleTagsGateway{
		db:     db,
		tagger: tagger,
		config: config,
	}
}

// FetchArticleTags retrieves tags associated with a specific article.
// If no tags exist and on-the-fly generation is enabled, generates tags via mq-hub
// and persists them to the database.
func (g *FetchArticleTagsGateway) FetchArticleTags(ctx context.Context, articleID string) ([]*domain.FeedTag, error) {
	// 1. Try to fetch existing tags from DB
	tags, err := g.db.FetchArticleTags(ctx, articleID)
	if err != nil {
		return nil, err
	}

	// Return tags if found
	if len(tags) > 0 {
		return tags, nil
	}

	// 2. No tags exist - check if on-the-fly generation is enabled
	if !g.config.OnTheFlyEnabled || g.tagger == nil || !g.tagger.IsEnabled() {
		logger.Logger.DebugContext(ctx, "no tags found and on-the-fly generation disabled",
			"articleID", articleID)
		return []*domain.FeedTag{}, nil
	}

	// 3-6. On-the-fly tag generation with singleflight deduplication.
	// Concurrent requests for the same articleID share a single generation call.
	result, err, _ := g.sfGroup.Do(articleID, func() (interface{}, error) {
		return g.generateAndPersistTags(ctx, articleID)
	})
	if err != nil {
		return []*domain.FeedTag{}, nil
	}

	return result.([]*domain.FeedTag), nil
}

// generateAndPersistTags fetches article content, generates tags via mq-hub, and persists them.
func (g *FetchArticleTagsGateway) generateAndPersistTags(ctx context.Context, articleID string) ([]*domain.FeedTag, error) {
	// 3. Fetch article content for tag generation
	article, err := g.db.FetchArticleByID(ctx, articleID)
	if err != nil {
		logger.Logger.WarnContext(ctx, "failed to fetch article for on-the-fly tag generation",
			"articleID", articleID, "error", err)
		return []*domain.FeedTag{}, nil
	}
	if article == nil {
		logger.Logger.WarnContext(ctx, "article not found for on-the-fly tag generation",
			"articleID", articleID)
		return []*domain.FeedTag{}, nil
	}

	// 4. Generate tags via mq-hub with retry
	resp, err := g.generateTagsWithRetry(ctx, articleID, article)
	if err != nil {
		logger.Logger.WarnContext(ctx, "failed to generate tags on-the-fly after retries",
			"articleID", articleID, "error", err)
		return []*domain.FeedTag{}, nil
	}

	if !resp.Success {
		logger.Logger.WarnContext(ctx, "on-the-fly tag generation was unsuccessful",
			"articleID", articleID, "error", resp.ErrorMessage)
		return []*domain.FeedTag{}, nil
	}

	// 5. Convert generated tags to domain tags
	now := time.Now()
	domainTags := make([]*domain.FeedTag, len(resp.Tags))
	for i, tag := range resp.Tags {
		domainTags[i] = &domain.FeedTag{
			ID:         tag.ID,
			TagName:    tag.Name,
			Confidence: float64(tag.Confidence),
			CreatedAt:  now,
		}
	}

	logger.Logger.InfoContext(ctx, "generated tags on-the-fly successfully",
		"articleID", articleID,
		"tagCount", len(domainTags),
		"inferenceMs", resp.InferenceMs)

	// 6. Persist generated tags to DB (fail-open: return tags even if upsert fails)
	if article.FeedID != "" {
		upsertItems := make([]alt_db.TagUpsertItem, len(resp.Tags))
		for i, tag := range resp.Tags {
			upsertItems[i] = alt_db.TagUpsertItem{
				Name:       tag.Name,
				Confidence: tag.Confidence,
			}
		}

		_, upsertErr := g.db.UpsertArticleTags(ctx, articleID, article.FeedID, upsertItems)
		if upsertErr != nil {
			logger.Logger.WarnContext(ctx, "failed to persist generated tags, returning tags anyway",
				"articleID", articleID, "feedID", article.FeedID, "error", upsertErr)
		} else {
			logger.Logger.InfoContext(ctx, "persisted generated tags to DB",
				"articleID", articleID, "feedID", article.FeedID, "tagCount", len(upsertItems))
		}
	} else {
		logger.Logger.WarnContext(ctx, "skipping tag persistence: empty FeedID",
			"articleID", articleID)
	}

	return domainTags, nil
}

// generateTagsWithRetry calls the tag generator with retry logic.
func (g *FetchArticleTagsGateway) generateTagsWithRetry(
	ctx context.Context,
	articleID string,
	article *domain.ArticleContent,
) (*mqhub_connect.GenerateTagsResponse, error) {
	req := mqhub_connect.GenerateTagsRequest{
		ArticleID: articleID,
		Title:     article.Title,
		Content:   article.Content,
		FeedID:    article.FeedID,
		TimeoutMs: g.config.TagGenerationTimeoutMs,
	}

	maxAttempts := 1 + g.config.MaxRetries
	var lastErr error

	for attempt := 0; attempt < maxAttempts; attempt++ {
		if attempt > 0 {
			logger.Logger.InfoContext(ctx, "retrying tag generation",
				"articleID", articleID, "attempt", attempt+1)

			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(g.config.RetryBackoff):
			}
		}

		logger.Logger.InfoContext(ctx, "generating tags on-the-fly",
			"articleID", articleID,
			"attempt", attempt+1,
			"titleLen", len(article.Title),
			"contentLen", len(article.Content))

		resp, err := g.tagger.GenerateTagsForArticle(ctx, req)
		if err != nil {
			lastErr = err
			continue
		}

		return resp, nil
	}

	return nil, lastErr
}
