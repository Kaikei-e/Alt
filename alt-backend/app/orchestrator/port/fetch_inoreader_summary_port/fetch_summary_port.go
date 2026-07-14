package fetch_inoreader_summary_port

import (
	"alt/domain"
	"context"
)

// FetchInoreaderSummaryPort defines the interface for fetching inoreader article summaries
type FetchInoreaderSummaryPort interface {
	FetchSummariesByURLs(ctx context.Context, urls []string) ([]*domain.InoreaderSummary, error)
}
