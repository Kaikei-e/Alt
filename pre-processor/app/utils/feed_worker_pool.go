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
	workers   int
	jobQueue  chan FeedJob
	logger    *slog.Logger
}

// Workers returns the number of workers in the pool
func (p *FeedWorkerPool) Workers() int {
	return p.workers
}

// NewFeedWorkerPool creates a new feed worker pool
func NewFeedWorkerPool(workers int, queueSize int, logger *slog.Logger) *FeedWorkerPool {
	return &FeedWorkerPool{
		workers:  workers,
		jobQueue: make(chan FeedJob, queueSize),
		logger:   logger,
	}
}

// ProcessFeeds processes feeds concurrently using worker pool
func (p *FeedWorkerPool) ProcessFeeds(ctx context.Context, feeds []FeedJob, fetcher ArticleFetcher) []FeedResult {
	results := make([]FeedResult, 0, len(feeds))
	resultsChan := make(chan FeedResult, len(feeds))
	
	var wg sync.WaitGroup
	
	// Start workers
	for i := 0; i < p.workers; i++ {
		wg.Add(1)
		go p.worker(ctx, &wg, resultsChan, fetcher)
	}
	
	// Send jobs
	go func() {
		defer close(p.jobQueue)
		for _, job := range feeds {
			select {
			case p.jobQueue <- job:
			case <-ctx.Done():
				return
			}
		}
	}()
	
	// Wait for workers to complete
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
func (p *FeedWorkerPool) worker(ctx context.Context, wg *sync.WaitGroup, results chan<- FeedResult, fetcher ArticleFetcher) {
	defer wg.Done()
	
	for {
		select {
		case job, ok := <-p.jobQueue:
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