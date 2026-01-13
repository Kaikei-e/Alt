package fetch_recent_articles_usecase

import (
	"alt/domain"
	"alt/port/fetch_recent_articles_port"
	"alt/utils/logger"
	"context"
	"errors"
	"time"
)

// FetchRecentArticlesInput defines the input for fetching recent articles
type FetchRecentArticlesInput struct {
	WithinHours int // Time window in hours (default: 24)
	Limit       int // Maximum number of articles to return (default: 100)
}

// FetchRecentArticlesOutput defines the output containing recent articles
type FetchRecentArticlesOutput struct {
	Articles []*domain.Article `json:"articles"`
	Since    time.Time         `json:"since"`
	Until    time.Time         `json:"until"`
	Count    int               `json:"count"`
}

// FetchRecentArticlesUsecase handles fetching recent articles
type FetchRecentArticlesUsecase struct {
	gateway fetch_recent_articles_port.FetchRecentArticlesPort
}

// NewFetchRecentArticlesUsecase creates a new usecase instance
func NewFetchRecentArticlesUsecase(gateway fetch_recent_articles_port.FetchRecentArticlesPort) *FetchRecentArticlesUsecase {
	return &FetchRecentArticlesUsecase{gateway: gateway}
}

// Execute fetches recent articles within the specified time window
func (u *FetchRecentArticlesUsecase) Execute(ctx context.Context, input FetchRecentArticlesInput) (*FetchRecentArticlesOutput, error) {
	// Validate and set defaults
	withinHours := input.WithinHours
	if withinHours <= 0 {
		withinHours = 24
	}
	if withinHours > 168 { // Max 7 days
		withinHours = 168
	}

	// limit=0 means no limit (time constraint only for RAG use case)
	// negative limit defaults to 100
	limit := input.Limit
	if limit < 0 {
		limit = 100
	}
	if limit > 500 && limit != 0 {
		limit = 500
	}

	now := time.Now()
	since := now.Add(-time.Duration(withinHours) * time.Hour)

	logger.Logger.Info("fetching recent articles",
		"within_hours", withinHours,
		"limit", limit,
		"since", since.Format(time.RFC3339))

	articles, err := u.gateway.FetchRecentArticles(ctx, since, limit)
	if err != nil {
		logger.Logger.Error("failed to fetch recent articles", "error", err)
		return nil, errors.New("failed to fetch recent articles")
	}

	logger.Logger.Info("successfully fetched recent articles", "count", len(articles))

	return &FetchRecentArticlesOutput{
		Articles: articles,
		Since:    since,
		Until:    now,
		Count:    len(articles),
	}, nil
}
