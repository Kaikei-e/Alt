package service

import (
	"log/slog"

	"pre-processor-sidecar/domain"
	"pre-processor-sidecar/models"
)

// Tier1FilterResult holds the outcome of filtering a batch of articles.
type Tier1FilterResult struct {
	Tier1    []*models.Article
	Filtered int
}

// FilterTier1Articles separates Tier1 articles from non-Tier1 ones.
// Non-Tier1 articles are logged with rejection reasons.
func FilterTier1Articles(articles []*models.Article, logger *slog.Logger) Tier1FilterResult {
	var tier1 []*models.Article
	var filtered int

	for _, article := range articles {
		result := domain.ClassifyTier1(article.Content, article.ArticleURL)
		if result.IsTier1 {
			tier1 = append(tier1, article)
		} else {
			filtered++
			logger.Info("non-tier1 article filtered",
				"url", article.ArticleURL,
				"title", article.Title,
				"reason", result.Reason)
		}
	}

	if len(articles) > 0 {
		logger.Info("tier1 filter applied",
			"total", len(articles),
			"tier1", len(tier1),
			"filtered", filtered)
	}

	return Tier1FilterResult{Tier1: tier1, Filtered: filtered}
}
