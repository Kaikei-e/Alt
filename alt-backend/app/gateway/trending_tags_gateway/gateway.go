package trending_tags_gateway

import (
	"alt/port/knowledge_home_port"
	"context"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
)

const (
	minRecentCount  = 3
	minSurgeRatio   = 1.5
	maxTrendingTags = 20
	baselineWeeks   = 4.0 // 30 days ≈ 4 weeks
)

// TrendingTagsGateway computes trending tags by comparing recent (7d) vs baseline (30d) article counts.
// All trending logic lives here in Go — SQL only fetches raw counts.
type TrendingTagsGateway struct {
	fetchPort knowledge_home_port.FetchTagArticleCountsPort
	cacheTTL  time.Duration

	mu    sync.RWMutex
	cache map[uuid.UUID]cacheEntry
}

type cacheEntry struct {
	tags      []knowledge_home_port.TrendingTag
	fetchedAt time.Time
}

func NewTrendingTagsGateway(fetchPort knowledge_home_port.FetchTagArticleCountsPort, cacheTTL time.Duration) *TrendingTagsGateway {
	return &TrendingTagsGateway{
		fetchPort: fetchPort,
		cacheTTL:  cacheTTL,
		cache:     make(map[uuid.UUID]cacheEntry),
	}
}

func (g *TrendingTagsGateway) GetTrendingTags(ctx context.Context, userID uuid.UUID) ([]knowledge_home_port.TrendingTag, error) {
	if cached := g.getCached(userID); cached != nil {
		return cached, nil
	}

	tags, err := g.compute(ctx, userID)
	if err != nil {
		return nil, err
	}

	g.setCache(userID, tags)
	return tags, nil
}

func (g *TrendingTagsGateway) compute(ctx context.Context, userID uuid.UUID) ([]knowledge_home_port.TrendingTag, error) {
	now := time.Now()

	recentCounts, err := g.fetchPort.FetchTagArticleCounts(ctx, userID, now.Add(-7*24*time.Hour))
	if err != nil {
		return nil, err
	}

	baselineCounts, err := g.fetchPort.FetchTagArticleCounts(ctx, userID, now.Add(-30*24*time.Hour))
	if err != nil {
		return nil, err
	}

	baselineMap := make(map[string]int, len(baselineCounts))
	for _, b := range baselineCounts {
		baselineMap[b.TagName] = b.ArticleCount
	}

	var trending []knowledge_home_port.TrendingTag
	for _, r := range recentCounts {
		if r.ArticleCount < minRecentCount {
			continue
		}

		baselineTotal := baselineMap[r.TagName]
		weeklyAvg := float64(baselineTotal) / baselineWeeks

		var surgeRatio float64
		if weeklyAvg > 0 {
			surgeRatio = float64(r.ArticleCount) / weeklyAvg
		} else {
			surgeRatio = float64(r.ArticleCount)
		}

		if surgeRatio >= minSurgeRatio {
			trending = append(trending, knowledge_home_port.TrendingTag{
				TagName:     r.TagName,
				RecentCount: r.ArticleCount,
				SurgeRatio:  surgeRatio,
			})
		}
	}

	sort.Slice(trending, func(i, j int) bool {
		return trending[i].SurgeRatio > trending[j].SurgeRatio
	})

	if len(trending) > maxTrendingTags {
		trending = trending[:maxTrendingTags]
	}

	return trending, nil
}

func (g *TrendingTagsGateway) getCached(userID uuid.UUID) []knowledge_home_port.TrendingTag {
	g.mu.RLock()
	defer g.mu.RUnlock()

	entry, ok := g.cache[userID]
	if !ok || time.Since(entry.fetchedAt) > g.cacheTTL {
		return nil
	}
	result := make([]knowledge_home_port.TrendingTag, len(entry.tags))
	copy(result, entry.tags)
	return result
}

func (g *TrendingTagsGateway) setCache(userID uuid.UUID, tags []knowledge_home_port.TrendingTag) {
	g.mu.Lock()
	defer g.mu.Unlock()

	stored := make([]knowledge_home_port.TrendingTag, len(tags))
	copy(stored, tags)
	g.cache[userID] = cacheEntry{tags: stored, fetchedAt: time.Now()}
}
