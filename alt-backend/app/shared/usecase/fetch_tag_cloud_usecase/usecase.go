package fetch_tag_cloud_usecase

import (
	"alt/domain"
	"alt/shared/port/fetch_tag_cloud_port"
	"alt/utils/logger"
	"context"
	"errors"
	"strconv"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

const (
	defaultMaxStale          = 24 * time.Hour
	backgroundRefreshTimeout = 30 * time.Second
)

type cacheState string

const (
	cacheStateFresh cacheState = "fresh"
	cacheStateStale cacheState = "stale"
	cacheStateMiss  cacheState = "miss"
)

// FetchTagCloudUsecase orchestrates fetching tag cloud data with in-memory caching.
type FetchTagCloudUsecase struct {
	fetchTagCloudPort fetch_tag_cloud_port.FetchTagCloudPort

	mu          sync.RWMutex
	cachedItems []*domain.TagCloudItem
	cachedLimit int
	cachedAt    time.Time
	cacheTTL    time.Duration
	maxStale    time.Duration
	refreshing  bool
	now         func() time.Time

	sf singleflight.Group
}

// NewFetchTagCloudUsecase creates a new FetchTagCloudUsecase with the given cache TTL.
func NewFetchTagCloudUsecase(port fetch_tag_cloud_port.FetchTagCloudPort, cacheTTL time.Duration) *FetchTagCloudUsecase {
	return &FetchTagCloudUsecase{
		fetchTagCloudPort: port,
		cacheTTL:          cacheTTL,
		maxStale:          defaultMaxStale,
		now:               time.Now,
	}
}

// Execute fetches tag cloud data with validation and caching.
func (u *FetchTagCloudUsecase) Execute(ctx context.Context, limit int) ([]*domain.TagCloudItem, error) {
	return u.execute(ctx, limit, false)
}

// Refresh always recomputes the tag cloud (bypasses cache).
// Used by the cache warmer to guarantee fresh data and reset TTL.
func (u *FetchTagCloudUsecase) Refresh(ctx context.Context, limit int) ([]*domain.TagCloudItem, error) {
	return u.execute(ctx, limit, true)
}

// ExecuteCountsOnly fetches raw tag cloud counts from the database without
// fetching tag cooccurrences or computing the 3D force-directed layout.
// Used by the data-plane RPC handler where only tag names and counts are returned.
func (u *FetchTagCloudUsecase) ExecuteCountsOnly(ctx context.Context, limit int) ([]*domain.TagCloudItem, error) {
	if limit <= 0 {
		limit = 300
	}
	if limit > 500 {
		logger.Logger.ErrorContext(ctx, "invalid limit: cannot exceed 500", "limit", limit)
		return nil, errors.New("limit cannot exceed 500")
	}

	items, err := u.fetchTagCloudPort.FetchTagCloud(ctx, limit)
	if err != nil {
		logger.Logger.ErrorContext(ctx, "failed to fetch tag cloud", "error", err)
		return nil, err
	}

	return items, nil
}

func (u *FetchTagCloudUsecase) execute(ctx context.Context, limit int, forceRefresh bool) ([]*domain.TagCloudItem, error) {
	if limit <= 0 {
		limit = 300
	}
	if limit > 500 {
		logger.Logger.ErrorContext(ctx, "invalid limit: cannot exceed 500", "limit", limit)
		return nil, errors.New("limit cannot exceed 500")
	}

	// Check cache (skip when force-refreshing)
	if !forceRefresh {
		if cached, state, startRefresh := u.getCached(limit); cached != nil {
			logger.Logger.InfoContext(ctx, "tag cloud cache hit", "limit", limit, "cache_state", string(state))
			if startRefresh {
				go u.refreshInBackground(ctx, limit)
			}
			return cached, nil
		}
	}

	return u.computeCoalesced(ctx, limit, forceRefresh)
}

func (u *FetchTagCloudUsecase) refreshInBackground(reqCtx context.Context, limit int) {
	// Detach from caller cancellation so client disconnects do not abort revalidation.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(reqCtx), backgroundRefreshTimeout)
	defer cancel()

	if _, err := u.computeCoalesced(ctx, limit, false); err != nil {
		u.mu.Lock()
		u.refreshing = false
		u.mu.Unlock()
		logger.Logger.WarnContext(ctx, "failed to refresh tag cloud in background", "limit", limit, "error", err)
	}
}

func (u *FetchTagCloudUsecase) computeCoalesced(ctx context.Context, limit int, forceRefresh bool) ([]*domain.TagCloudItem, error) {
	key := strconv.Itoa(limit)
	val, err, _ := u.sf.Do(key, func() (any, error) {
		return u.compute(ctx, limit, forceRefresh)
	})
	if err != nil {
		return nil, err
	}
	return deepCopyItems(val.([]*domain.TagCloudItem)), nil
}

func (u *FetchTagCloudUsecase) compute(ctx context.Context, limit int, forceRefresh bool) ([]*domain.TagCloudItem, error) {
	logger.Logger.InfoContext(ctx, "fetching tag cloud", "limit", limit, "forceRefresh", forceRefresh, "cache_state", string(cacheStateMiss))

	items, err := u.fetchTagCloudPort.FetchTagCloud(ctx, limit)
	if err != nil {
		logger.Logger.ErrorContext(ctx, "failed to fetch tag cloud", "error", err)
		return nil, err
	}

	logger.Logger.InfoContext(ctx, "successfully fetched tag cloud", "count", len(items))

	// Compute 3D layout using force-directed graph
	if len(items) > 0 {
		tagNames := make([]string, len(items))
		for i, item := range items {
			tagNames[i] = item.TagName
		}

		cooccStart := time.Now()
		cooccurrences, err := u.fetchTagCloudPort.FetchTagCooccurrences(ctx, tagNames)
		if err != nil {
			logger.Logger.WarnContext(ctx, "failed to fetch cooccurrences, using layout without edges", "error", err)
			cooccurrences = nil
		}
		cooccMs := time.Since(cooccStart).Milliseconds()

		layoutStart := time.Now()
		ComputeLayout(items, cooccurrences)
		layoutMs := time.Since(layoutStart).Milliseconds()

		logger.Logger.InfoContext(ctx, "tag cloud computation complete",
			"cooccurrence_ms", cooccMs,
			"layout_ms", layoutMs,
			"edge_count", len(cooccurrences),
		)
	}

	// Store in cache
	u.setCache(limit, items)

	return items, nil
}

// getCached returns a deep copy of cached items if valid, nil otherwise.
func (u *FetchTagCloudUsecase) getCached(limit int) ([]*domain.TagCloudItem, cacheState, bool) {
	u.mu.Lock()
	defer u.mu.Unlock()

	now := u.now()
	if u.cachedItems == nil || u.cachedLimit != limit {
		return nil, cacheStateMiss, false
	}

	age := now.Sub(u.cachedAt)
	if age <= u.cacheTTL {
		return deepCopyItems(u.cachedItems), cacheStateFresh, false
	}
	if age <= u.maxStale {
		startRefresh := !u.refreshing
		if startRefresh {
			u.refreshing = true
		}
		return deepCopyItems(u.cachedItems), cacheStateStale, startRefresh
	}
	return nil, cacheStateMiss, false
}

// setCache stores the items in the cache.
func (u *FetchTagCloudUsecase) setCache(limit int, items []*domain.TagCloudItem) {
	u.mu.Lock()
	defer u.mu.Unlock()

	u.cachedItems = items
	u.cachedLimit = limit
	u.refreshing = false
	u.cachedAt = u.now()
}

// deepCopyItems creates a deep copy of TagCloudItem slice.
func deepCopyItems(items []*domain.TagCloudItem) []*domain.TagCloudItem {
	if items == nil {
		return nil
	}
	copies := make([]*domain.TagCloudItem, len(items))
	for i, item := range items {
		cp := *item
		copies[i] = &cp
	}
	return copies
}
