package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
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

	// mu guards refreshTicker/fetchTicker/stopChan/isRunning against
	// concurrent Start/Stop calls. stopChan is (re)created per Start instead
	// of once in NewScheduler, since a closed channel can never be reopened
	// — reusing it would make runLoop return immediately on every restart
	// after the first Stop.
	mu            sync.Mutex
	refreshTicker *time.Ticker
	fetchTicker   *time.Ticker
	stopChan      chan struct{}
	isRunning     bool
	wg            sync.WaitGroup

	// fetchRunMu/refreshRunMu make the ticker-driven loop and an
	// Admin-API-triggered manual run (TriggerFetchNow/TriggerRefreshNow)
	// mutually exclusive — without this, an admin trigger racing the ticker
	// could double-consume the 100 req/day Inoreader quota concurrently.
	fetchRunMu   sync.Mutex
	refreshRunMu sync.Mutex
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
	}
}

// Start starts the scheduling loops
func (s *Scheduler) Start(cfg Config) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.isRunning {
		s.logger.Warn("Scheduler is already running")
		return
	}

	s.logger.Info("Starting Inoreader Scheduler",
		"fetch_interval", cfg.FetchInterval,
		"refresh_interval", cfg.RefreshInterval)

	stopChan := make(chan struct{})
	refreshTicker := time.NewTicker(cfg.RefreshInterval)
	fetchTicker := time.NewTicker(cfg.FetchInterval)

	s.stopChan = stopChan
	s.refreshTicker = refreshTicker
	s.fetchTicker = fetchTicker
	s.isRunning = true

	s.wg.Add(1)
	go s.runLoop(stopChan, refreshTicker, fetchTicker)
}

// Stop stops the scheduler and waits for the run loop to fully exit, so a
// subsequent Start never races with the previous loop still shutting down.
func (s *Scheduler) Stop() {
	s.mu.Lock()
	if !s.isRunning {
		s.mu.Unlock()
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
	s.mu.Unlock()

	s.wg.Wait()
}

func (s *Scheduler) runLoop(stopChan chan struct{}, refreshTicker, fetchTicker *time.Ticker) {
	defer s.wg.Done()
	for {
		select {
		case <-stopChan:
			return
		case <-refreshTicker.C:
			s.runRefresh()
		case <-fetchTicker.C:
			s.runFetch()
		}
	}
}

// runFetch runs the ticker-driven article fetch, skipping this tick if a
// fetch (ticker-driven or via TriggerFetchNow) is already in progress.
func (s *Scheduler) runFetch() {
	if !s.fetchRunMu.TryLock() {
		s.logger.Warn("Skipping scheduled article fetch: a fetch is already in progress")
		return
	}
	defer s.fetchRunMu.Unlock()
	s.fetchNextStream()
}

// runRefresh runs the ticker-driven subscription refresh, skipping this
// tick if a refresh is already in progress.
func (s *Scheduler) runRefresh() {
	if !s.refreshRunMu.TryLock() {
		s.logger.Warn("Skipping scheduled subscription refresh: a refresh is already in progress")
		return
	}
	defer s.refreshRunMu.Unlock()
	s.refreshSubscriptions()
}

// TriggerFetchNow runs an out-of-band article fetch (e.g. from the Admin
// API's manual trigger endpoint). Mutually exclusive with the ticker-driven
// fetch via fetchRunMu, so the two paths can never run concurrently and
// double-consume the Inoreader daily quota. Runs asynchronously, matching
// the fire-and-forget semantics HTTP callers expect.
func (s *Scheduler) TriggerFetchNow() error {
	if !s.fetchRunMu.TryLock() {
		return fmt.Errorf("article fetch already in progress")
	}
	go func() {
		defer s.fetchRunMu.Unlock()
		s.fetchNextStream()
	}()
	return nil
}

// TriggerRefreshNow runs an out-of-band subscription refresh (e.g. from the
// Admin API's manual trigger endpoint), mutually exclusive with the
// ticker-driven refresh via refreshRunMu.
func (s *Scheduler) TriggerRefreshNow() error {
	if !s.refreshRunMu.TryLock() {
		return fmt.Errorf("subscription refresh already in progress")
	}
	go func() {
		defer s.refreshRunMu.Unlock()
		s.refreshSubscriptions()
	}()
	return nil
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
