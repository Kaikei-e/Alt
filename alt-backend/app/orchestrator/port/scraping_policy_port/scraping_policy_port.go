package scraping_policy_port

import (
	"context"
)

//go:generate go run go.uber.org/mock/mockgen -source=scraping_policy_port.go -destination=../../mocks/mock_scraping_policy_port.go

// ScrapingPolicyPort defines the interface for scraping policy checks
type ScrapingPolicyPort interface {
	// CanFetchArticle checks if an article URL can be fetched based on domain policy and robots.txt
	CanFetchArticle(ctx context.Context, articleURL string) (bool, error)
}
