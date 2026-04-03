package domain

import "context"

// SummarizerClient summarizes articles via pre-processor.
type SummarizerClient interface {
	Summarize(ctx context.Context, articleID string) (string, error)
}
