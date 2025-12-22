package scheduler

import (
	"context"
	"log/slog"
	"time"

	"pre-processor-sidecar/repository"
	"pre-processor-sidecar/service"
)

// Scheduler manages the scheduling of Inoreader API requests
type Scheduler struct {
	syncRepo            repository.SyncStateRepository
	subService          *service.SubscriptionSyncService // Updated type name
	articleFetchService *service.ArticleFetchService     // Use ArticleFetchService to ensure persistence
	logger              *slog.Logger
	refreshTicker       *time.Ticker
	fetchTicker         *time.Ticker
	stopChan            chan struct{}
	isRunning           bool
}

// Config holds scheduler configuration
type Config struct {
	FetchInterval   time.Duration
	RefreshInterval time.Duration
}

// DefaultConfig returns the default configuration for the scheduler
// 90 requests/day = 1 request every 16 minutes
// 1 refresh/day = 24 hours
func DefaultConfig() Config {
	return Config{
		FetchInterval:   16 * time.Minute,
		RefreshInterval: 24 * time.Hour,
	}
}

// NewScheduler creates a new Inoreader scheduler
func NewScheduler(
	syncRepo repository.SyncStateRepository,
	subService *service.SubscriptionSyncService,
	articleFetchService *service.ArticleFetchService,
	logger *slog.Logger,
) *Scheduler {
	return &Scheduler{
		syncRepo:            syncRepo,
		subService:          subService,
		articleFetchService: articleFetchService,
		logger:              logger,
		stopChan:            make(chan struct{}),
	}
}

// Start starts the scheduling loops
func (s *Scheduler) Start(cfg Config) {
	if s.isRunning {
		s.logger.Warn("Scheduler is already running")
		return
	}

	s.logger.Info("Starting Inoreader Scheduler",
		"fetch_interval", cfg.FetchInterval,
		"refresh_interval", cfg.RefreshInterval)

	s.refreshTicker = time.NewTicker(cfg.RefreshInterval)
	s.fetchTicker = time.NewTicker(cfg.FetchInterval)
	s.isRunning = true

	go s.runLoop()
}

// Stop stops the scheduler
func (s *Scheduler) Stop() {
	if !s.isRunning {
		return
	}

	s.logger.Info("Stopping Inoreader Scheduler")
	close(s.stopChan)
	if s.refreshTicker != nil {
		s.refreshTicker.Stop()
	}
	if s.fetchTicker != nil {
		s.fetchTicker.Stop()
	}
	s.isRunning = false
}

func (s *Scheduler) runLoop() {
	for {
		select {
		case <-s.stopChan:
			return
		case <-s.refreshTicker.C:
			s.refreshSubscriptions()
		case <-s.fetchTicker.C:
			s.fetchNextStream()
		}
	}
}

func (s *Scheduler) refreshSubscriptions() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	s.logger.Info("Starting daily subscription refresh")

	if err := s.subService.SyncSubscriptionsNew(ctx); err != nil {
		s.logger.Error("Failed to refresh subscriptions", "error", err)
		return
	}

	s.logger.Info("Successfully refreshed subscriptions")
}

func (s *Scheduler) fetchNextStream() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// 1. Get the oldest synced stream
	syncState, err := s.syncRepo.GetOldestOne(ctx)
	if err != nil {
		s.logger.Error("Failed to get oldest sync state", "error", err)
		return
	}

	if syncState == nil {
		s.logger.Info("No streams found to sync")
		return
	}

	s.logger.Info("Fetching content for stream",
		"stream_id", syncState.StreamID,
		"last_sync", syncState.LastSync)

	// 2. Fetch AND Save content using ArticleFetchService
	// This ensures articles are persisted to the database
	result, err := s.articleFetchService.FetchArticles(ctx, syncState.StreamID, 100)
	if err != nil {
		s.logger.Error("Failed to fetch and save articles",
			"stream_id", syncState.StreamID,
			"error", err)
		return
	}

	// FetchArticles checks errors itself, result will contain details
	if len(result.Errors) > 0 {
		s.logger.Warn("Fetch completed with errors", "errors", result.Errors)
	}

	s.logger.Info("Successfully processed stream",
		"stream_id", syncState.StreamID,
		"articles_found", result.TotalProcessed,
		"new_articles_saved", result.NewArticles,
		"has_continuation", result.ContinuationToken != "")
}
