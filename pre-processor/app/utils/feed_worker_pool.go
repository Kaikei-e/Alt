package utils

import (
	"context"
	"log/slog"
	"sync"

	"pre-processor/models"
)

// ArticleFetcher interface for dependency injection
type ArticleFetcher interface {
	FetchArticle(ctx context.Context, url string) (*models.Article, error)
	ValidateURL(url string) error
}

// FeedJob represents a job to process a feed
type FeedJob struct {
	URL string
}

// FeedResult represents the result of processing a feed
type FeedResult struct {
	Job     FeedJob
	Article *models.Article
	Error   error
}

// FeedWorkerPool manages a pool of workers for concurrent feed processing
type FeedWorkerPool struct {
	workers int
	logger  *slog.Logger
}

// Workers returns the number of workers in the pool
func (p *FeedWorkerPool) Workers() int {
	return p.workers
}

// NewFeedWorkerPool creates a new feed worker pool
func NewFeedWorkerPool(workers int, queueSize int, logger *slog.Logger) *FeedWorkerPool {
	return &FeedWorkerPool{
		workers: workers,
		logger:  logger,
	}
}

// ProcessFeeds processes feeds concurrently using worker pool
func (p *FeedWorkerPool) ProcessFeeds(ctx context.Context, feeds []FeedJob, fetcher ArticleFetcher) []FeedResult {
	if len(feeds) == 0 {
		return []FeedResult{}
	}

	results := make([]FeedResult, 0, len(feeds))
	resultsChan := make(chan FeedResult, len(feeds))
	jobQueue := make(chan FeedJob, len(feeds))

	var wg sync.WaitGroup
	var closeOnce sync.Once

	// Start workers
	for i := 0; i < p.workers; i++ {
		wg.Add(1)
		go p.worker(ctx, &wg, jobQueue, resultsChan, fetcher)
	}

	// Send jobs
	go func() {
		defer closeOnce.Do(func() { close(jobQueue) })
		for _, job := range feeds {
			select {
			case jobQueue <- job:
			case <-ctx.Done():
				return
			}
		}
	}()

	// Wait for workers to complete and close results channel
	go func() {
		wg.Wait()
		close(resultsChan)
	}()

	// Collect results
	for result := range resultsChan {
		results = append(results, result)
	}

	return results
}

// worker processes jobs from the job queue
func (p *FeedWorkerPool) worker(ctx context.Context, wg *sync.WaitGroup, jobQueue <-chan FeedJob, results chan<- FeedResult, fetcher ArticleFetcher) {
	defer wg.Done()

	for {
		select {
		case job, ok := <-jobQueue:
			if !ok {
				return
			}

			p.logger.Info("Processing feed", "url", job.URL)

			// Check context before making expensive call
			select {
			case <-ctx.Done():
				return
			default:
			}

			article, err := fetcher.FetchArticle(ctx, job.URL)
			if err != nil {
				p.logger.Error("Error fetching article", "url", job.URL, "error", err)
			}

			result := FeedResult{
				Job:     job,
				Article: article,
				Error:   err,
			}

			select {
			case results <- result:
			case <-ctx.Done():
				return
			}

		case <-ctx.Done():
			return
		}
	}
}

