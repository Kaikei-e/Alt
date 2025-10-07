package handler

import (
	"log/slog"
	"net/http"

	"pre-processor/models"
	"pre-processor/repository"

	"github.com/labstack/echo/v4"
)

// SummarizeRequest represents the request body for article summarization
type SummarizeRequest struct {
	Content   string `json:"content" validate:"required"`
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
	logger      *slog.Logger
}

// NewSummarizeHandler creates a new summarize handler
func NewSummarizeHandler(apiRepo repository.ExternalAPIRepository, summaryRepo repository.SummaryRepository, logger *slog.Logger) *SummarizeHandler {
	return &SummarizeHandler{
		apiRepo:     apiRepo,
		summaryRepo: summaryRepo,
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
	if req.Content == "" {
		h.logger.Warn("empty content provided")
		return echo.NewHTTPError(http.StatusBadRequest, "Content cannot be empty")
	}

	if req.ArticleID == "" {
		h.logger.Warn("empty article_id provided")
		return echo.NewHTTPError(http.StatusBadRequest, "Article ID cannot be empty")
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
