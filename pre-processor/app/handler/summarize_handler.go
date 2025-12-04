package handler

import (
	"log/slog"
	"net/http"
	"strings"

	"pre-processor/models"
	"pre-processor/repository"
	"pre-processor/utils/html_parser"

	"github.com/labstack/echo/v4"
)

// SummarizeRequest represents the request body for article summarization
type SummarizeRequest struct {
	Content   string `json:"content"`
	ArticleID string `json:"article_id" validate:"required"`
	Title     string `json:"title"`
}

// SummarizeResponse represents the response for article summarization
type SummarizeResponse struct {
	Success   bool   `json:"success"`
	Summary   string `json:"summary"`
	ArticleID string `json:"article_id"`
}

// SummarizeHandler handles on-demand article summarization requests
type SummarizeHandler struct {
	apiRepo     repository.ExternalAPIRepository
	summaryRepo repository.SummaryRepository
	articleRepo repository.ArticleRepository
	logger      *slog.Logger
}

// NewSummarizeHandler creates a new summarize handler
func NewSummarizeHandler(apiRepo repository.ExternalAPIRepository, summaryRepo repository.SummaryRepository, articleRepo repository.ArticleRepository, logger *slog.Logger) *SummarizeHandler {
	return &SummarizeHandler{
		apiRepo:     apiRepo,
		summaryRepo: summaryRepo,
		articleRepo: articleRepo,
		logger:      logger,
	}
}

// HandleSummarize handles POST /api/v1/summarize requests
func (h *SummarizeHandler) HandleSummarize(c echo.Context) error {
	ctx := c.Request().Context()

	// Parse request body
	var req SummarizeRequest
	if err := c.Bind(&req); err != nil {
		h.logger.Error("failed to bind request", "error", err)
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid request format")
	}

	// Validate required fields
	if req.ArticleID == "" {
		h.logger.Warn("empty article_id provided")
		return echo.NewHTTPError(http.StatusBadRequest, "Article ID cannot be empty")
	}

	// If content is empty, try to fetch from DB
	if req.Content == "" {
		h.logger.Info("content is empty, fetching from DB", "article_id", req.ArticleID)
		fetchedArticle, err := h.articleRepo.FindByID(ctx, req.ArticleID)
		if err != nil {
			h.logger.Error("failed to fetch article from DB", "error", err, "article_id", req.ArticleID)
			return echo.NewHTTPError(http.StatusInternalServerError, "Failed to fetch article content")
		}
		if fetchedArticle == nil {
			h.logger.Warn("article not found in DB", "article_id", req.ArticleID)
			return echo.NewHTTPError(http.StatusNotFound, "Article not found")
		}
		if fetchedArticle.Content == "" {
			h.logger.Warn("article found but content is empty", "article_id", req.ArticleID)
			return echo.NewHTTPError(http.StatusBadRequest, "Article content is empty in database")
		}
		// Check if content is HTML and extract text if needed
		content := fetchedArticle.Content
		if strings.Contains(content, "<") && strings.Contains(content, ">") {
			// Content appears to be HTML, extract text
			h.logger.Info("detected HTML content, extracting text", "article_id", req.ArticleID)
			extractedText := html_parser.ExtractArticleText(content)
			if extractedText != "" {
				content = extractedText
				h.logger.Info("HTML content extracted successfully", "article_id", req.ArticleID, "original_length", len(fetchedArticle.Content), "extracted_length", len(extractedText))
			} else {
				h.logger.Warn("HTML extraction returned empty, using original content", "article_id", req.ArticleID)
			}
		}
		req.Content = content
		// Also update title if missing
		if req.Title == "" {
			req.Title = fetchedArticle.Title
		}
		h.logger.Info("content fetched from DB successfully", "article_id", req.ArticleID, "content_length", len(req.Content))
	}

	if req.Content == "" {
		h.logger.Warn("empty content provided and not found in DB", "article_id", req.ArticleID)
		return echo.NewHTTPError(http.StatusBadRequest, "Content cannot be empty")
	}

	h.logger.Info("processing summarization request", "article_id", req.ArticleID)

	// Create article model for summarization
	article := &models.Article{
		ID:      req.ArticleID,
		Content: req.Content,
	}

	// Call summarization service
	summarized, err := h.apiRepo.SummarizeArticle(ctx, article)
	if err != nil {
		h.logger.Error("failed to summarize article", "error", err, "article_id", req.ArticleID)
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to generate summary")
	}

	h.logger.Info("article summarized successfully", "article_id", req.ArticleID)

	// Save summary to database
	articleTitle := req.Title
	if articleTitle == "" {
		articleTitle = "Untitled" // Fallback if no title provided
	}

	articleSummary := &models.ArticleSummary{
		ArticleID:       req.ArticleID,
		ArticleTitle:    articleTitle,
		SummaryJapanese: summarized.SummaryJapanese,
	}

	if err := h.summaryRepo.Create(ctx, articleSummary); err != nil {
		h.logger.Error("failed to save summary to database", "error", err, "article_id", req.ArticleID)
		// Don't fail the request if DB save fails - still return the summary
		// This ensures the user gets the summary even if DB has issues
		h.logger.Warn("continuing despite DB save failure", "article_id", req.ArticleID)
	} else {
		h.logger.Info("summary saved to database successfully", "article_id", req.ArticleID)
	}

	// Return response
	response := SummarizeResponse{
		Success:   true,
		Summary:   summarized.SummaryJapanese,
		ArticleID: req.ArticleID,
	}

	return c.JSON(http.StatusOK, response)
}
