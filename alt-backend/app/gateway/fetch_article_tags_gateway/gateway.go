package fetch_article_tags_gateway

import (
	"alt/domain"
	"alt/driver/alt_db"
	"alt/driver/mqhub_connect"
	"alt/utils/logger"
	"context"
	"time"
)

// Config holds configuration for the gateway.
type Config struct {
	// OnTheFlyEnabled enables on-the-fly tag generation when no tags exist.
	OnTheFlyEnabled bool
	// TagGenerationTimeoutMs is the timeout for tag generation in milliseconds.
	TagGenerationTimeoutMs int32
}

// DefaultConfig returns the default configuration.
func DefaultConfig() Config {
	return Config{
		OnTheFlyEnabled:        true,
		TagGenerationTimeoutMs: 30000, // 30 seconds
	}
}

// FetchArticleTagsGateway implements the port for fetching article tags.
type FetchArticleTagsGateway struct {
	altDB       *alt_db.AltDBRepository
	mqhubClient *mqhub_connect.Client
	config      Config
}

// NewFetchArticleTagsGateway creates a new gateway instance.
func NewFetchArticleTagsGateway(altDB *alt_db.AltDBRepository) *FetchArticleTagsGateway {
	return &FetchArticleTagsGateway{
		altDB:  altDB,
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
		altDB:       altDB,
		mqhubClient: mqhubClient,
		config:      config,
	}
}

// FetchArticleTags retrieves tags associated with a specific article.
// If no tags exist and on-the-fly generation is enabled, generates tags via mq-hub.
func (g *FetchArticleTagsGateway) FetchArticleTags(ctx context.Context, articleID string) ([]*domain.FeedTag, error) {
	// 1. Try to fetch existing tags from DB
	tags, err := g.altDB.FetchArticleTags(ctx, articleID)
	if err != nil {
		return nil, err
	}

	// Return tags if found
	if len(tags) > 0 {
		return tags, nil
	}

	// 2. No tags exist - check if on-the-fly generation is enabled
	if !g.config.OnTheFlyEnabled || g.mqhubClient == nil || !g.mqhubClient.IsEnabled() {
		logger.Logger.DebugContext(ctx, "no tags found and on-the-fly generation disabled",
			"articleID", articleID)
		return []*domain.FeedTag{}, nil
	}

	// 3. Fetch article content for tag generation
	article, err := g.altDB.FetchArticleByID(ctx, articleID)
	if err != nil {
		logger.Logger.WarnContext(ctx, "failed to fetch article for on-the-fly tag generation",
			"articleID", articleID, "error", err)
		return []*domain.FeedTag{}, nil // Return empty tags, don't fail the request
	}
	if article == nil {
		logger.Logger.WarnContext(ctx, "article not found for on-the-fly tag generation",
			"articleID", articleID)
		return []*domain.FeedTag{}, nil
	}

	// 4. Generate tags via mq-hub
	logger.Logger.InfoContext(ctx, "generating tags on-the-fly",
		"articleID", articleID,
		"titleLen", len(article.Title),
		"contentLen", len(article.Content))

	resp, err := g.mqhubClient.GenerateTagsForArticle(ctx, mqhub_connect.GenerateTagsRequest{
		ArticleID: articleID,
		Title:     article.Title,
		Content:   article.Content,
		FeedID:    "", // FeedID not available from ArticleContent
		TimeoutMs: g.config.TagGenerationTimeoutMs,
	})
	if err != nil {
		logger.Logger.WarnContext(ctx, "failed to generate tags on-the-fly",
			"articleID", articleID, "error", err)
		return []*domain.FeedTag{}, nil // Return empty tags, don't fail the request
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

	return domainTags, nil
}
