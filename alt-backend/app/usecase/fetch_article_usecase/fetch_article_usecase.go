package fetch_article_usecase

import (
	"alt/domain"
	"alt/port/fetch_article_port"
	"alt/port/robots_txt_port"
	"alt/utils/html_parser"
	"alt/utils/logger"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// ArticleUsecase defines the business logic for fetching articles
type ArticleUsecase interface {
	Execute(ctx context.Context, articleURL string) (*string, error)
	FetchCompliantArticle(ctx context.Context, articleURL *url.URL, userContext domain.UserContext) (string, error)
}

// ArticleRepository defines the data access interface needed by this usecase
//
type ArticleRepository interface {
	FetchArticleByURL(ctx context.Context, articleURL string) (*domain.ArticleContent, error)
	IsDomainDeclined(ctx context.Context, userID, domain string) (bool, error)
	SaveDeclinedDomain(ctx context.Context, userID, domain string) error
	SaveArticle(ctx context.Context, url, title, content string) (string, error)
}

type ArticleUsecaseImpl struct {
	articleFetcher fetch_article_port.FetchArticlePort
	robotsTxt      robots_txt_port.RobotsTxtPort
	repo           ArticleRepository
}

func NewArticleUsecase(
	articleFetcher fetch_article_port.FetchArticlePort,
	robotsTxt robots_txt_port.RobotsTxtPort,
	repo ArticleRepository,
) ArticleUsecase {
	return &ArticleUsecaseImpl{
		articleFetcher: articleFetcher,
		robotsTxt:      robotsTxt,
		repo:           repo,
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

func (u *ArticleUsecaseImpl) FetchCompliantArticle(ctx context.Context, targetURL *url.URL, userContext domain.UserContext) (string, error) {
	urlStr := targetURL.String()
	domainStr := targetURL.Hostname()

	// 1. Check if article already exists in DB
	existingArticle, err := u.repo.FetchArticleByURL(ctx, urlStr)
	if err != nil {
		// Log error but generally fail if DB is down.
		// Note via driver: returns nil, nil if Not Found.
		return "", fmt.Errorf("failed to check existing article: %w", err)
	}

	if existingArticle != nil {
		logger.Logger.Info("Article found in database", "url", urlStr, "id", existingArticle.ID)
		return existingArticle.Content, nil
	}

	// 2. Check if the domain is in the declined_domains table for this user
	isDeclined, err := u.repo.IsDomainDeclined(ctx, userContext.UserID.String(), domainStr)
	if err != nil {
		logger.Logger.Error("Failed to check if domain is declined", "error", err, "domain", domainStr, "user_id", userContext.UserID)
		return "", fmt.Errorf("failed to check declined status: %w", err)
	}

	if isDeclined {
		logger.Logger.Info("Domain is in declined list for user", "domain", domainStr, "user_id", userContext.UserID)
		return "", &domain.ComplianceError{Code: http.StatusForbidden, Message: "The request was declined. Please visit the site."}
	}

	// 3. Robots.txt Compliance Check
	userAgent := "Alt-RSS-Reader/1.0 (+https://alt.example.com)"
	isAllowed, err := u.robotsTxt.IsPathAllowed(ctx, targetURL, userAgent)
	if err != nil {
		logger.Logger.Warn("Failed to check robots.txt, defaulting to ALLOWED", "error", err, "url", urlStr)
		isAllowed = true
	}

	if !isAllowed {
		logger.Logger.Info("Access denied by robots.txt", "url", urlStr)
		if err := u.repo.SaveDeclinedDomain(ctx, userContext.UserID.String(), domainStr); err != nil {
			logger.Logger.Error("Failed to save declined domain", "error", err, "domain", domainStr)
		}
		return "", &domain.ComplianceError{Code: http.StatusForbidden, Message: "The request was declined. Please visit the site."}
	}

	// 4. Fetch from Web
	logger.Logger.Info("Fetching article from Web", "url", urlStr)
	contentPtr, err := u.articleFetcher.FetchArticleContents(ctx, urlStr)
	if err != nil {
		logger.Logger.Error("Failed to fetch article content", "error", err, "url", urlStr)
		return "", fmt.Errorf("fetch failed: %w", err)
	}
	if contentPtr == nil || *contentPtr == "" {
		return "", fmt.Errorf("fetched content is empty")
	}
	htmlContent := *contentPtr

	// 5. Extract Title and Text
	fetchedTitle := html_parser.ExtractTitle(htmlContent)
	contentStr := html_parser.ExtractArticleText(htmlContent)

	if contentStr == "" {
		logger.Logger.Warn("failed to extract article text from HTML, falling back to raw HTML",
			"url", urlStr, "html_size_bytes", len(htmlContent))
		contentStr = htmlContent
	}

	// 6. Save to Database
	newID, saveErr := u.repo.SaveArticle(ctx, urlStr, fetchedTitle, contentStr)
	if saveErr != nil {
		logger.Logger.Error("Failed to save article to database", "error", saveErr, "url", urlStr)
	} else {
		logger.Logger.Info("Article content saved", "url", urlStr, "new_id", newID)
	}

	return contentStr, nil
}
