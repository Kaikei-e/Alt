package scheduler

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"pre-processor-sidecar/repository"
	"pre-processor-sidecar/service"
)

// SyncMode says whether this process is allowed to talk to Inoreader at all.
type SyncMode string

const (
	SyncEnabled  SyncMode = "enabled"
	SyncDisabled SyncMode = "disabled"
)

// ErrSyncDisabled is returned by the manual triggers while Inoreader sync is
// switched off, so an operator hitting the Admin API gets a clear refusal
// instead of a silent no-op.
var ErrSyncDisabled = errors.New("inoreader sync is disabled by configuration")

// ParseSyncMode maps the INOREADER_SYNC environment value onto a SyncMode. An
// unset variable keeps the historical behaviour (enabled); switching sync off
// has to be spelled out, so a missing variable can never be mistaken for a
// deliberate shutdown. Anything else is an error rather than a guess.
func ParseSyncMode(raw string) (SyncMode, error) {
	switch SyncMode(raw) {
	case "", SyncEnabled:
		return SyncEnabled, nil
	case SyncDisabled:
		return SyncDisabled, nil
	default:
		return "", fmt.Errorf("INOREADER_SYNC must be %q or %q, got %q", SyncEnabled, SyncDisabled, raw)
	}
}

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

	// syncMode is captured in Start and read by the manual triggers, so a
	// disabled deployment refuses out-of-band runs too.
	syncMode SyncMode
}

// Config holds scheduler configuration
type Config struct {
	FetchInterval   time.Duration
	RefreshInterval time.Duration

	// SyncMode gates every Inoreader call: both ticker loops and the Admin
	// API's manual triggers. It has no usable zero value on purpose.
	SyncMode SyncMode
}

// DefaultConfig returns the default configuration for the scheduler
// 90 requests/day = 1 request every 16 minutes
// 1 refresh/day = 24 hours
func DefaultConfig() Config {
	return Config{
		FetchInterval:   16 * time.Minute,
		RefreshInterval: 24 * time.Hour,
		SyncMode:        SyncEnabled,
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

	switch cfg.SyncMode {
	case SyncEnabled, SyncDisabled:
	default:
		panic(fmt.Sprintf("scheduler: SyncMode is %q; the caller must pass %q or %q so an unwired config cannot pass for a deliberate shutdown", cfg.SyncMode, SyncEnabled, SyncDisabled))
	}

	s.syncMode = cfg.SyncMode
	if cfg.SyncMode == SyncDisabled {
		s.logger.Warn("inoreader_sync_disabled",
			"detail", "no article fetch or subscription refresh will run and the Admin API triggers will refuse",
			"how_to_enable", "set INOREADER_SYNC=enabled")
		return
	}

	s.logger.Info("inoreader_sync_enabled",
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
	if s.disabled() {
		return ErrSyncDisabled
	}
	if !s.fetchRunMu.TryLock() {
		return fmt.Errorf("article fetch already in progress")
	}
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		defer s.fetchRunMu.Unlock()
		s.fetchNextStream()
	}()
	return nil
}

// TriggerRefreshNow runs an out-of-band subscription refresh (e.g. from the
// Admin API's manual trigger endpoint), mutually exclusive with the
// ticker-driven refresh via refreshRunMu.
func (s *Scheduler) TriggerRefreshNow() error {
	if s.disabled() {
		return ErrSyncDisabled
	}
	if !s.refreshRunMu.TryLock() {
		return fmt.Errorf("subscription refresh already in progress")
	}
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		defer s.refreshRunMu.Unlock()
		s.refreshSubscriptions()
	}()
	return nil
}

func (s *Scheduler) disabled() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.syncMode == SyncDisabled
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
