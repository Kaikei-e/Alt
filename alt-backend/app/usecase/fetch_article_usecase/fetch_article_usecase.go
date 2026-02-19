package fetch_article_usecase

import (
	"alt/domain"
	"alt/port/fetch_article_port"
	"alt/port/rag_integration_port"
	"alt/port/robots_txt_port"
	"alt/port/scraping_policy_port"
	"alt/utils/html_parser"
	"alt/utils/logger"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ArticleUsecase defines the business logic for fetching articles
type ArticleUsecase interface {
	Execute(ctx context.Context, articleURL string) (*string, error)
	FetchCompliantArticle(ctx context.Context, articleURL *url.URL, userContext domain.UserContext) (content string, articleID string, err error)
}

// ArticleRepository defines the data access interface needed by this usecase
type ArticleRepository interface {
	FetchArticleByURL(ctx context.Context, articleURL string) (*domain.ArticleContent, error)
	IsDomainDeclined(ctx context.Context, userID, domain string) (bool, error)
	SaveDeclinedDomain(ctx context.Context, userID, domain string) error
	SaveArticle(ctx context.Context, url, title, content string) (string, error)
}

type ArticleUsecaseImpl struct {
	articleFetcher     fetch_article_port.FetchArticlePort
	robotsTxt          robots_txt_port.RobotsTxtPort
	repo               ArticleRepository
	ragIntegration     rag_integration_port.RagIntegrationPort
	scrapingPolicyPort scraping_policy_port.ScrapingPolicyPort // optional, nil = fallback to robotsTxt
}

func NewArticleUsecase(
	articleFetcher fetch_article_port.FetchArticlePort,
	robotsTxt robots_txt_port.RobotsTxtPort,
	repo ArticleRepository,
	ragIntegration rag_integration_port.RagIntegrationPort,
) ArticleUsecase {
	return &ArticleUsecaseImpl{
		articleFetcher: articleFetcher,
		robotsTxt:      robotsTxt,
		repo:           repo,
		ragIntegration: ragIntegration,
	}
}

// NewArticleUsecaseWithScrapingPolicy creates an ArticleUsecase with ScrapingPolicyPort integration.
// When scrapingPolicyPort is set, it is used instead of direct robots.txt HTTP fetching,
// providing cached robots.txt checks and crawl-delay enforcement.
func NewArticleUsecaseWithScrapingPolicy(
	articleFetcher fetch_article_port.FetchArticlePort,
	robotsTxt robots_txt_port.RobotsTxtPort,
	repo ArticleRepository,
	ragIntegration rag_integration_port.RagIntegrationPort,
	scrapingPolicyPort scraping_policy_port.ScrapingPolicyPort,
) ArticleUsecase {
	return &ArticleUsecaseImpl{
		articleFetcher:     articleFetcher,
		robotsTxt:          robotsTxt,
		repo:               repo,
		ragIntegration:     ragIntegration,
		scrapingPolicyPort: scrapingPolicyPort,
	}
}

// Execute is the legacy method (keeping for backward compatibility if used elsewhere)
func (u *ArticleUsecaseImpl) Execute(ctx context.Context, articleURL string) (*string, error) {
	content, err := u.articleFetcher.FetchArticleContents(ctx, articleURL)
	if err != nil {
		return nil, err
	}
	if content == nil || strings.TrimSpace(*content) == "" {
		return nil, errors.New("fetched article content is empty")
	}
	textOnly := html_parser.ExtractArticleText(*content)
	if strings.TrimSpace(textOnly) == "" {
		return nil, errors.New("extracted article text is empty")
	}
	return &textOnly, nil
}

func (u *ArticleUsecaseImpl) FetchCompliantArticle(ctx context.Context, targetURL *url.URL, userContext domain.UserContext) (content string, articleID string, err error) {
	urlStr := targetURL.String()
	domainStr := targetURL.Hostname()

	// 1. Check if article already exists in DB
	existingArticle, err := u.repo.FetchArticleByURL(ctx, urlStr)
	if err != nil {
		// Log error but generally fail if DB is down.
		// Note via driver: returns nil, nil if Not Found.
		return "", "", fmt.Errorf("failed to check existing article: %w", err)
	}

	if existingArticle != nil {
		logger.Logger.InfoContext(ctx, "Article found in database", "url", urlStr, "id", existingArticle.ID)
		return existingArticle.Content, existingArticle.ID, nil
	}

	// 2. Check if the domain is in the declined_domains table for this user
	isDeclined, err := u.repo.IsDomainDeclined(ctx, userContext.UserID.String(), domainStr)
	if err != nil {
		logger.Logger.ErrorContext(ctx, "Failed to check if domain is declined", "error", err, "domain", domainStr, "user_id", userContext.UserID)
		return "", "", fmt.Errorf("failed to check declined status: %w", err)
	}

	if isDeclined {
		logger.Logger.InfoContext(ctx, "Domain is in declined list for user", "domain", domainStr, "user_id", userContext.UserID)
		return "", "", &domain.ComplianceError{Code: http.StatusForbidden, Message: "The request was declined. Please visit the site."}
	}

	// 3. Robots.txt Compliance Check
	// Use ScrapingPolicyPort (cached) if available, otherwise fall back to direct robots.txt fetch
	var isAllowed bool
	if u.scrapingPolicyPort != nil {
		allowed, policyErr := u.scrapingPolicyPort.CanFetchArticle(ctx, urlStr)
		if policyErr != nil {
			logger.Logger.WarnContext(ctx, "ScrapingPolicy check failed, defaulting to ALLOWED", "error", policyErr, "url", urlStr)
			isAllowed = true
		} else {
			isAllowed = allowed
		}
	} else {
		// Fallback: direct robots.txt HTTP fetch (no cache)
		userAgent := "Alt-RSS-Reader/1.0 (+https://alt.example.com)"
		allowed, robotsErr := u.robotsTxt.IsPathAllowed(ctx, targetURL, userAgent)
		if robotsErr != nil {
			logger.Logger.WarnContext(ctx, "Failed to check robots.txt, defaulting to ALLOWED", "error", robotsErr, "url", urlStr)
			isAllowed = true
		} else {
			isAllowed = allowed
		}
	}

	if !isAllowed {
		logger.Logger.InfoContext(ctx, "Access denied by scraping policy", "url", urlStr)
		if err := u.repo.SaveDeclinedDomain(ctx, userContext.UserID.String(), domainStr); err != nil {
			logger.Logger.ErrorContext(ctx, "Failed to save declined domain", "error", err, "domain", domainStr)
		}
		return "", "", &domain.ComplianceError{Code: http.StatusForbidden, Message: "The request was declined. Please visit the site."}
	}

	// 4. Fetch from Web
	logger.Logger.InfoContext(ctx, "Fetching article from Web", "url", urlStr)
	contentPtr, err := u.articleFetcher.FetchArticleContents(ctx, urlStr)
	if err != nil {
		logger.Logger.ErrorContext(ctx, "Failed to fetch article content", "error", err, "url", urlStr)
		return "", "", fmt.Errorf("fetch failed: %w", err)
	}
	if contentPtr == nil || *contentPtr == "" {
		return "", "", fmt.Errorf("fetched content is empty")
	}
	htmlContent := *contentPtr

	// 5. Extract Title and Rich HTML Content
	fetchedTitle := html_parser.ExtractTitle(htmlContent)
	contentStr := html_parser.ExtractArticleHTML(htmlContent)

	if contentStr == "" {
		logger.Logger.WarnContext(ctx, "failed to extract article HTML from content, falling back to sanitized HTML",
			"url", urlStr, "html_size_bytes", len(htmlContent))
		contentStr = html_parser.SanitizeHTML(htmlContent)
	}

	// 6. Save to Database
	newID, saveErr := u.repo.SaveArticle(ctx, urlStr, fetchedTitle, contentStr)
	if saveErr != nil {
		logger.Logger.ErrorContext(ctx, "Failed to save article to database", "error", saveErr, "url", urlStr)
	} else {
		logger.Logger.InfoContext(ctx, "Article content saved", "url", urlStr, "new_id", newID)

		// 7. Upsert to RAG (Step A: Direct Call)
		// Using time.Now() for PublishedAt as a temporary measure until HTML parser supports date extraction

		t := time.Now()
		upsertInput := rag_integration_port.UpsertArticleInput{
			ArticleID:   newID,
			Title:       fetchedTitle,
			Body:        contentStr,
			URL:         urlStr,
			PublishedAt: &t,
			UserID:      userContext.UserID.String(),
		}
		if err := u.ragIntegration.UpsertArticle(ctx, upsertInput); err != nil {
			// Log error but do not fail the request, as Article is already saved
			logger.Logger.ErrorContext(ctx, "Failed to upsert article to RAG", "error", err, "article_id", newID)
		} else {
			logger.Logger.InfoContext(ctx, "Article upserted to RAG", "article_id", newID)
		}
	}

	return contentStr, newID, nil
}
