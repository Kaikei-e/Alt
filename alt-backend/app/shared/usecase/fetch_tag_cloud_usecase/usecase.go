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
	defaultMaxStale = 24 * time.Hour
	computeTimeout  = 30 * time.Second
)

type cacheState string

const (
	cacheStateFresh   cacheState = "fresh"
	cacheStateStale   cacheState = "stale"
	cacheStateMiss    cacheState = "miss"
	cacheStateRefresh cacheState = "refresh"
)

// FetchTagCloudUsecase orchestrates fetching tag cloud data with in-memory caching.
type FetchTagCloudUsecase struct {
	fetchTagCloudPort fetch_tag_cloud_port.FetchTagCloudPort

	mu          sync.Mutex
	cachedItems []*domain.TagCloudItem
	cachedEdges []*domain.TagCloudEdge
	cachedLimit int
	cachedAt    time.Time
	cacheTTL    time.Duration
	maxStale    time.Duration
	refreshing  bool
	now         func() time.Time

	sf singleflight.Group
}

type computedTagCloud struct {
	items []*domain.TagCloudItem
	edges []*domain.TagCloudEdge
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
func (u *FetchTagCloudUsecase) Execute(ctx context.Context, limit int) ([]*domain.TagCloudItem, []*domain.TagCloudEdge, error) {
	return u.execute(ctx, limit, false)
}

// Refresh always recomputes the tag cloud (bypasses cache).
// Used by the cache warmer to guarantee fresh data and reset TTL.
func (u *FetchTagCloudUsecase) Refresh(ctx context.Context, limit int) ([]*domain.TagCloudItem, error) {
	items, _, err := u.execute(ctx, limit, true)
	return items, err
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

func (u *FetchTagCloudUsecase) execute(ctx context.Context, limit int, forceRefresh bool) ([]*domain.TagCloudItem, []*domain.TagCloudEdge, error) {
	if limit <= 0 {
		limit = 300
	}
	if limit > 500 {
		logger.Logger.ErrorContext(ctx, "invalid limit: cannot exceed 500", "limit", limit)
		return nil, nil, errors.New("limit cannot exceed 500")
	}

	// Check cache (skip when force-refreshing)
	if !forceRefresh {
		if cachedItems, cachedEdges, state, startRefresh := u.getCached(limit); cachedItems != nil {
			logger.Logger.InfoContext(ctx, "tag cloud cache hit", "limit", limit, "cache_state", string(state))
			if startRefresh {
				go u.refreshInBackground(ctx, limit)
			}
			return cachedItems, cachedEdges, nil
		}
	}

	state := cacheStateMiss
	if forceRefresh {
		state = cacheStateRefresh
	}
	items, edges, err := u.computeCoalesced(ctx, limit, forceRefresh, state)
	if err != nil {
		return nil, nil, err
	}
	return deepCopyItems(items), deepCopyEdges(edges), nil
}

func (u *FetchTagCloudUsecase) refreshInBackground(reqCtx context.Context, limit int) {
	defer func() {
		if r := recover(); r != nil {
			u.mu.Lock()
			u.refreshing = false
			u.mu.Unlock()
			logger.Logger.ErrorContext(reqCtx, "panic during background tag cloud refresh", "limit", limit, "panic", r)
		}
	}()

	if _, _, err := u.computeCoalesced(reqCtx, limit, false, cacheStateRefresh); err != nil {
		u.mu.Lock()
		u.refreshing = false
		u.mu.Unlock()
		logger.Logger.WarnContext(reqCtx, "failed to refresh tag cloud in background", "limit", limit, "error", err)
	}
}

func (u *FetchTagCloudUsecase) computeCoalesced(ctx context.Context, limit int, forceRefresh bool, state cacheState) ([]*domain.TagCloudItem, []*domain.TagCloudEdge, error) {
	key := strconv.Itoa(limit)
	val, err, _ := u.sf.Do(key, func() (any, error) {
		// Detach from the first caller's cancellation so one client disconnect
		// does not abort coalesced cold callers or background revalidation.
		cctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), computeTimeout)
		defer cancel()
		return u.compute(cctx, limit, forceRefresh, state)
	})
	if err != nil {
		return nil, nil, err
	}
	res := val.(computedTagCloud)
	return res.items, res.edges, nil
}

func (u *FetchTagCloudUsecase) compute(ctx context.Context, limit int, forceRefresh bool, state cacheState) (computedTagCloud, error) {
	logger.Logger.InfoContext(ctx, "fetching tag cloud", "limit", limit, "forceRefresh", forceRefresh, "cache_state", string(state))

	items, err := u.fetchTagCloudPort.FetchTagCloud(ctx, limit)
	if err != nil {
		logger.Logger.ErrorContext(ctx, "failed to fetch tag cloud", "error", err)
		return computedTagCloud{}, err
	}

	logger.Logger.InfoContext(ctx, "successfully fetched tag cloud", "count", len(items))

	var edges []*domain.TagCloudEdge
	// Compute 3D layout, communities, and edges using co-occurrences
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
		ComputeClusters(items, cooccurrences)
		edges = ComputeEdges(items, cooccurrences)
		layoutMs := time.Since(layoutStart).Milliseconds()

		logger.Logger.InfoContext(ctx, "tag cloud computation complete",
			"cooccurrence_ms", cooccMs,
			"layout_ms", layoutMs,
			"edge_count", len(edges),
		)
	}

	// Store in cache
	u.setCache(limit, items, edges)

	return computedTagCloud{items: items, edges: edges}, nil
}

// getCached returns deep copies of cached items and edges if within maxStale,
// the cache state, and whether the caller should start a background refresh.
func (u *FetchTagCloudUsecase) getCached(limit int) ([]*domain.TagCloudItem, []*domain.TagCloudEdge, cacheState, bool) {
	u.mu.Lock()
	defer u.mu.Unlock()

	now := u.now()
	if u.cachedItems == nil || u.cachedLimit != limit {
		return nil, nil, cacheStateMiss, false
	}

	age := now.Sub(u.cachedAt)
	if age <= u.cacheTTL {
		return deepCopyItems(u.cachedItems), deepCopyEdges(u.cachedEdges), cacheStateFresh, false
	}
	if age <= u.maxStale {
		startRefresh := !u.refreshing
		if startRefresh {
			u.refreshing = true
		}
		return deepCopyItems(u.cachedItems), deepCopyEdges(u.cachedEdges), cacheStateStale, startRefresh
	}
	return nil, nil, cacheStateMiss, false
}

// setCache stores the items and edges in the cache.
func (u *FetchTagCloudUsecase) setCache(limit int, items []*domain.TagCloudItem, edges []*domain.TagCloudEdge) {
	u.mu.Lock()
	defer u.mu.Unlock()

	u.cachedItems = items
	u.cachedEdges = edges
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

// deepCopyEdges creates a deep copy of TagCloudEdge slice.
func deepCopyEdges(edges []*domain.TagCloudEdge) []*domain.TagCloudEdge {
	if edges == nil {
		return nil
	}
	copies := make([]*domain.TagCloudEdge, len(edges))
	for i, edge := range edges {
		cp := *edge
		copies[i] = &cp
	}
	return copies
}
