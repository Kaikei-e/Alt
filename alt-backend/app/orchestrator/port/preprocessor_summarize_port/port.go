// Package preprocessor_summarize_port defines the contract for calling the
// pre-processor's summarization API. It is implemented by
// gateway/preprocessor_summarize_gateway and consumed by
// usecase/summarize_article_usecase.
package preprocessor_summarize_port

import (
	"context"
	"io"
)

// SummarizeStatus represents the status of an asynchronous summarization job.
type SummarizeStatus struct {
	JobID        string
	Status       string
	Summary      string
	ErrorMessage string
	ArticleID    string
}

// PreProcessorSummarizePort is the capability usecases need from the
// pre-processor summarization API.
type PreProcessorSummarizePort interface {
	// Summarize performs synchronous summarization. content may be empty
	// when using the pull model (pre-processor reads article content from
	// its own database by articleID).
	Summarize(ctx context.Context, content, articleID, title string) (string, error)
	// StreamSummarize performs streaming summarization. The caller must
	// close the returned ReadCloser.
	StreamSummarize(ctx context.Context, content, articleID, title string) (io.ReadCloser, error)
	// QueueSummarize submits an article for asynchronous summarization and
	// returns a job ID.
	QueueSummarize(ctx context.Context, articleID, title string) (string, error)
	// GetSummarizeStatus checks the status of a previously queued job.
	// Returns (nil, nil) when the job is not found.
	GetSummarizeStatus(ctx context.Context, jobID string) (*SummarizeStatus, error)
}
