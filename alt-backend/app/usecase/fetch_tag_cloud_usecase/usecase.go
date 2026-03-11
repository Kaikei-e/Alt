package fetch_tag_cloud_usecase

import (
	"alt/domain"
	"alt/port/fetch_tag_cloud_port"
	"alt/utils/logger"
	"context"
	"errors"
	"sync"
	"time"
)

// FetchTagCloudUsecase orchestrates fetching tag cloud data with in-memory caching.
type FetchTagCloudUsecase struct {
	fetchTagCloudPort fetch_tag_cloud_port.FetchTagCloudPort

	mu          sync.RWMutex
	cachedItems []*domain.TagCloudItem
	cachedLimit int
	cachedAt    time.Time
	cacheTTL    time.Duration
}

// NewFetchTagCloudUsecase creates a new FetchTagCloudUsecase with the given cache TTL.
func NewFetchTagCloudUsecase(port fetch_tag_cloud_port.FetchTagCloudPort, cacheTTL time.Duration) *FetchTagCloudUsecase {
	return &FetchTagCloudUsecase{
		fetchTagCloudPort: port,
		cacheTTL:          cacheTTL,
	}
}

// Execute fetches tag cloud data with validation and caching.
func (u *FetchTagCloudUsecase) Execute(ctx context.Context, limit int) ([]*domain.TagCloudItem, error) {
	if limit <= 0 {
		limit = 300
	}
	if limit > 500 {
		logger.Logger.ErrorContext(ctx, "invalid limit: cannot exceed 500", "limit", limit)
		return nil, errors.New("limit cannot exceed 500")
	}

	// Check cache
	if cached := u.getCached(limit); cached != nil {
		logger.Logger.InfoContext(ctx, "tag cloud cache hit", "limit", limit)
		return cached, nil
	}

	logger.Logger.InfoContext(ctx, "fetching tag cloud", "limit", limit)

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

		cooccurrences, err := u.fetchTagCloudPort.FetchTagCooccurrences(ctx, tagNames)
		if err != nil {
			logger.Logger.WarnContext(ctx, "failed to fetch cooccurrences, using layout without edges", "error", err)
			cooccurrences = nil
		}

		ComputeLayout(items, cooccurrences)
	}

	// Store in cache
	u.setCache(limit, items)

	// Return deep copy
	return deepCopyItems(items), nil
}

// getCached returns a deep copy of cached items if cache is valid, nil otherwise.
func (u *FetchTagCloudUsecase) getCached(limit int) []*domain.TagCloudItem {
	u.mu.RLock()
	defer u.mu.RUnlock()

	if u.cachedItems == nil || u.cachedLimit != limit || time.Since(u.cachedAt) > u.cacheTTL {
		return nil
	}
	return deepCopyItems(u.cachedItems)
}

// setCache stores the items in the cache.
func (u *FetchTagCloudUsecase) setCache(limit int, items []*domain.TagCloudItem) {
	u.mu.Lock()
	defer u.mu.Unlock()

	u.cachedItems = items
	u.cachedLimit = limit
	u.cachedAt = time.Now()
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
