package auto_fulltext_fetch_usecase

import (
	"alt/domain"
	"alt/port/fetch_article_port"
	"alt/port/internal_article_port"
	"alt/port/rag_integration_port"
	"alt/port/scraping_policy_port"
	"alt/utils"
	"alt/utils/html_parser"
	"alt/utils/logger"
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Repository interface {
	ListSubscribedUserIDsByFeedLinkID(ctx context.Context, feedLinkID string) ([]string, error)
	CheckArticleExistsByURLForUser(ctx context.Context, url string, userID string) (bool, string, error)
	IsDomainDeclined(ctx context.Context, userID, domain string) (bool, error)
	SaveDeclinedDomain(ctx context.Context, userID, domain string) error
	SaveArticleHead(ctx context.Context, articleID, headHTML, ogImageURL string) error
}

type AutoFulltextFetchUsecase struct {
	articleFetcher fetch_article_port.FetchArticlePort
	policy         scraping_policy_port.ScrapingPolicyPort
	articleCreator internal_article_port.CreateArticlePort
	repo           Repository
	ragIntegration rag_integration_port.RagIntegrationPort
}

func NewAutoFulltextFetchUsecase(
	articleFetcher fetch_article_port.FetchArticlePort,
	policy scraping_policy_port.ScrapingPolicyPort,
	articleCreator internal_article_port.CreateArticlePort,
	repo Repository,
	ragIntegration rag_integration_port.RagIntegrationPort,
) *AutoFulltextFetchUsecase {
	return &AutoFulltextFetchUsecase{
		articleFetcher: articleFetcher,
		policy:         policy,
		articleCreator: articleCreator,
		repo:           repo,
		ragIntegration: ragIntegration,
	}
}

func (u *AutoFulltextFetchUsecase) Process(ctx context.Context, feedItems []*domain.FeedItem, feedIDs []string) error {
	if len(feedItems) != len(feedIDs) {
		return fmt.Errorf("feed item count %d does not match feed id count %d", len(feedItems), len(feedIDs))
	}

	for i, feedItem := range feedItems {
		if feedItem == nil {
			continue
		}
		if err := u.processFeedItem(ctx, feedItem, feedIDs[i]); err != nil {
			return err
		}
	}

	return nil
}

func (u *AutoFulltextFetchUsecase) processFeedItem(ctx context.Context, feedItem *domain.FeedItem, feedID string) error {
	if strings.TrimSpace(feedItem.Link) == "" {
		return nil
	}
	if feedItem.FeedLinkID == nil || strings.TrimSpace(*feedItem.FeedLinkID) == "" {
		logger.Logger.WarnContext(ctx, "Skipping auto fulltext fetch because feed_link_id is missing", "article_url", feedItem.Link)
		return nil
	}
	if strings.TrimSpace(feedID) == "" {
		logger.Logger.WarnContext(ctx, "Skipping auto fulltext fetch because feed id is missing", "article_url", feedItem.Link)
		return nil
	}

	normalizedURL, err := utils.NormalizeURL(feedItem.Link)
	if err != nil {
		logger.Logger.WarnContext(ctx, "Failed to normalize article URL for auto fulltext fetch; using original",
			"article_url", feedItem.Link, "error", err)
		normalizedURL = feedItem.Link
	}

	userIDs, err := u.repo.ListSubscribedUserIDsByFeedLinkID(ctx, *feedItem.FeedLinkID)
	if err != nil {
		return fmt.Errorf("list subscribed users for feed_link_id %s: %w", *feedItem.FeedLinkID, err)
	}
	if len(userIDs) == 0 {
		logger.Logger.InfoContext(ctx, "Skipping auto fulltext fetch because no subscribers were found",
			"feed_link_id", *feedItem.FeedLinkID, "article_url", normalizedURL)
		return nil
	}

	for _, userID := range userIDs {
		u.processForUser(ctx, feedItem, feedID, normalizedURL, userID)
	}

	return nil
}

func (u *AutoFulltextFetchUsecase) processForUser(ctx context.Context, feedItem *domain.FeedItem, feedID, articleURL, userID string) {
	exists, articleID, err := u.repo.CheckArticleExistsByURLForUser(ctx, articleURL, userID)
	if err != nil {
		logger.Logger.ErrorContext(ctx, "Failed to check existing article before auto fulltext fetch",
			"article_url", articleURL, "user_id", userID, "error", err)
		return
	}
	if exists {
		logger.Logger.InfoContext(ctx, "Skipping auto fulltext fetch because article already exists",
			"article_url", articleURL, "user_id", userID, "article_id", articleID)
		return
	}

	parsedURL, err := url.Parse(articleURL)
	if err != nil || parsedURL.Hostname() == "" {
		logger.Logger.WarnContext(ctx, "Skipping auto fulltext fetch because article URL is invalid",
			"article_url", articleURL, "user_id", userID, "error", err)
		return
	}

	domainStr := parsedURL.Hostname()
	isDeclined, err := u.repo.IsDomainDeclined(ctx, userID, domainStr)
	if err != nil {
		logger.Logger.ErrorContext(ctx, "Failed to check declined domain before auto fulltext fetch",
			"domain", domainStr, "user_id", userID, "error", err)
		return
	}
	if isDeclined {
		logger.Logger.InfoContext(ctx, "Skipping auto fulltext fetch because domain is already declined",
			"domain", domainStr, "user_id", userID, "article_url", articleURL)
		return
	}

	allowed, err := u.policy.CanFetchArticle(ctx, articleURL)
	if err != nil {
		logger.Logger.WarnContext(ctx, "Scraping policy check failed during auto fulltext fetch; defaulting to allow",
			"article_url", articleURL, "user_id", userID, "error", err)
		allowed = true
	}
	if !allowed {
		if saveErr := u.repo.SaveDeclinedDomain(ctx, userID, domainStr); saveErr != nil {
			logger.Logger.ErrorContext(ctx, "Failed to persist declined domain from auto fulltext fetch",
				"domain", domainStr, "user_id", userID, "error", saveErr)
		}
		logger.Logger.InfoContext(ctx, "Auto fulltext fetch blocked by scraping policy",
			"article_url", articleURL, "user_id", userID, "status_code", http.StatusForbidden)
		return
	}

	fetchCtx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()

	rawHTML, err := u.articleFetcher.FetchArticleContents(fetchCtx, articleURL)
	if err != nil {
		logger.Logger.WarnContext(ctx, "Auto fulltext fetch failed; article will not be created",
			"article_url", articleURL, "user_id", userID, "error", err)
		return
	}
	if rawHTML == nil || strings.TrimSpace(*rawHTML) == "" {
		logger.Logger.WarnContext(ctx, "Auto fulltext fetch returned empty HTML; article will not be created",
			"article_url", articleURL, "user_id", userID)
		return
	}

	headHTML := html_parser.ExtractHead(*rawHTML)
	ogImageURL := html_parser.ExtractOgImageURL(*rawHTML)
	title := strings.TrimSpace(html_parser.ExtractTitle(*rawHTML))
	if title == "" {
		title = strings.TrimSpace(feedItem.Title)
	}
	if title == "" {
		title = articleURL
	}

	content := strings.TrimSpace(html_parser.ExtractArticleHTML(*rawHTML))
	if content == "" {
		content = strings.TrimSpace(html_parser.SanitizeHTML(*rawHTML))
	}
	if content == "" {
		logger.Logger.WarnContext(ctx, "Auto fulltext fetch extracted empty content; article will not be created",
			"article_url", articleURL, "user_id", userID)
		return
	}

	publishedAt := feedItem.PublishedParsed
	if publishedAt.IsZero() {
		publishedAt = time.Now().UTC()
	}

	newArticleID, _, err := u.articleCreator.CreateArticle(ctx, internal_article_port.CreateArticleParams{
		Title:       title,
		URL:         articleURL,
		Content:     content,
		FeedID:      feedID,
		UserID:      userID,
		PublishedAt: publishedAt,
	})
	if err != nil {
		logger.Logger.ErrorContext(ctx, "Failed to create article from auto fulltext fetch",
			"article_url", articleURL, "user_id", userID, "error", err)
		return
	}

	if headHTML != "" {
		if err := u.repo.SaveArticleHead(ctx, newArticleID, headHTML, ogImageURL); err != nil {
			logger.Logger.ErrorContext(ctx, "Failed to save article head from auto fulltext fetch",
				"article_id", newArticleID, "user_id", userID, "error", err)
		}
	}

	now := time.Now().UTC()
	go func(articleID, finalTitle, finalContent, finalURL, finalUserID string) {
		ragCtx, ragCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer ragCancel()
		if err := u.ragIntegration.UpsertArticle(ragCtx, rag_integration_port.UpsertArticleInput{
			ArticleID:   articleID,
			Title:       finalTitle,
			Body:        finalContent,
			URL:         finalURL,
			PublishedAt: &now,
			UserID:      finalUserID,
		}); err != nil {
			logger.Logger.Error("Failed to upsert article to RAG from auto fulltext fetch",
				"article_id", articleID, "user_id", finalUserID, "error", err)
		}
	}(newArticleID, title, content, articleURL, userID)

	logger.Logger.InfoContext(ctx, "Auto fulltext fetch created article",
		"article_url", articleURL, "user_id", userID, "article_id", newArticleID)
}
