package get_knowledge_home_usecase

import (
	"alt/domain"
	"alt/orchestrator/port/knowledge_home_port"
	"alt/orchestrator/port/knowledge_lens_port"
	"alt/orchestrator/port/today_digest_port"
	"alt/utils/logger"
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

// projectorName is the canonical projector name used for freshness checks.
const projectorName = "knowledge-home-projector"

// GetKnowledgeHomeUsecase orchestrates fetching the Knowledge Home data.
type GetKnowledgeHomeUsecase struct {
	homeItemsPort       knowledge_home_port.GetKnowledgeHomeItemsPort
	todayDigestPort     today_digest_port.GetTodayDigestPort
	resolveLensPort     knowledge_lens_port.ResolveKnowledgeHomeLensPort
	freshnessPort       today_digest_port.GetProjectionFreshnessPort
	needToKnowCountPort today_digest_port.CountNeedToKnowItemsPort
	tagHotspotPort      knowledge_home_port.TagHotspotPort
}

// NewGetKnowledgeHomeUsecase creates a new GetKnowledgeHomeUsecase.
func NewGetKnowledgeHomeUsecase(
	homeItemsPort knowledge_home_port.GetKnowledgeHomeItemsPort,
	todayDigestPort today_digest_port.GetTodayDigestPort,
	resolveLensPort knowledge_lens_port.ResolveKnowledgeHomeLensPort,
	freshnessPort today_digest_port.GetProjectionFreshnessPort,
	needToKnowCountPort today_digest_port.CountNeedToKnowItemsPort,
	tagHotspotPort knowledge_home_port.TagHotspotPort,
) *GetKnowledgeHomeUsecase {
	return &GetKnowledgeHomeUsecase{
		homeItemsPort:       homeItemsPort,
		todayDigestPort:     todayDigestPort,
		resolveLensPort:     resolveLensPort,
		freshnessPort:       freshnessPort,
		needToKnowCountPort: needToKnowCountPort,
		tagHotspotPort:      tagHotspotPort,
	}
}

// Service quality constants for the 3-tier quality model.
const (
	ServiceQualityFull     = "full"
	ServiceQualityDegraded = "degraded"
	ServiceQualityFallback = "fallback"
)

// degradedStalenessThreshold is the projection age beyond which we consider
// the data stale enough to downgrade service quality.
const degradedStalenessThreshold = 15 * time.Minute

// Result holds the output of GetKnowledgeHome.
type Result struct {
	Items          []domain.KnowledgeHomeItem
	Digest         domain.TodayDigest
	NextCursor     string
	HasMore        bool
	Degraded       bool
	ServiceQuality string
	GeneratedAt    time.Time
}

// Execute fetches the Knowledge Home data for a user.
func (u *GetKnowledgeHomeUsecase) Execute(ctx context.Context, userID uuid.UUID, cursor string, limit int, date time.Time, lensID *uuid.UUID) (*Result, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	result := &Result{
		GeneratedAt: time.Now(),
	}

	var lensFilter *domain.KnowledgeHomeLensFilter
	if u.resolveLensPort != nil {
		resolved, err := u.resolveLensPort.ResolveKnowledgeHomeLens(ctx, userID, lensID)
		if err != nil {
			logger.Logger.ErrorContext(ctx, "failed to resolve knowledge home lens", "error", err)
			result.Degraded = true
		} else {
			lensFilter = resolved
		}
	}

	var (
		items           []domain.KnowledgeHomeItem
		nextCursor      string
		hasMore         bool
		itemsErr        error
		digest          domain.TodayDigest
		digestErr       error
		trendingTags    []knowledge_home_port.TrendingTag
		trendingErr     error
		needToKnowCount int
		needToKnowErr   error
		updatedAt       *time.Time
		freshnessErr    error
		wg              sync.WaitGroup
	)

	wg.Add(2)
	go func() {
		defer wg.Done()
		items, nextCursor, hasMore, itemsErr = u.homeItemsPort.GetKnowledgeHomeItems(ctx, userID, cursor, limit, lensFilter)
	}()
	go func() {
		defer wg.Done()
		digest, digestErr = u.todayDigestPort.GetTodayDigest(ctx, userID, date)
	}()
	if u.tagHotspotPort != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			trendingTags, trendingErr = u.tagHotspotPort.GetTrendingTags(ctx, userID)
		}()
	}
	if u.needToKnowCountPort != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			needToKnowCount, needToKnowErr = u.needToKnowCountPort.CountNeedToKnowItems(ctx, userID, date)
		}()
	}
	if u.freshnessPort != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			updatedAt, freshnessErr = u.freshnessPort.GetProjectionFreshness(ctx, projectorName)
		}()
	}
	wg.Wait()

	if itemsErr != nil {
		logger.Logger.ErrorContext(ctx, "failed to fetch knowledge home items", "error", itemsErr)
		result.Degraded = true
	} else {
		result.Items = items
		result.NextCursor = nextCursor
		result.HasMore = hasMore
	}

	if digestErr != nil {
		logger.Logger.ErrorContext(ctx, "failed to fetch today digest", "error", digestErr)
		result.Degraded = true
	} else {
		result.Digest = digest
	}

	if err := requestContextError(itemsErr, digestErr); err != nil {
		return nil, err
	}
	if itemsErr != nil && digestErr != nil {
		return nil, fmt.Errorf("get knowledge home: items unavailable: %w; digest unavailable: %v", itemsErr, digestErr)
	}

	// Enrich items with trending tag hotspots
	if u.tagHotspotPort != nil && len(result.Items) > 0 {
		if trendingErr != nil {
			logger.Logger.WarnContext(ctx, "failed to get trending tags for enrichment", "error", trendingErr)
		} else {
			result.Items = enrichItemsWithTagHotspots(result.Items, trendingTags)
		}
	}

	// Enrich digest with backend-authoritative values
	if u.needToKnowCountPort != nil {
		if needToKnowErr != nil {
			logger.Logger.ErrorContext(ctx, "failed to count need-to-know items", "error", needToKnowErr)
			result.Digest.NeedToKnowCount = 0
		} else {
			result.Digest.NeedToKnowCount = needToKnowCount
		}
	}

	if u.freshnessPort != nil {
		if freshnessErr != nil {
			logger.Logger.ErrorContext(ctx, "failed to get projection freshness", "error", freshnessErr)
			result.Digest.DigestFreshness = domain.FreshnessUnknown
		} else {
			result.Digest.LastProjectedAt = updatedAt
			result.Digest.DigestFreshness = result.Digest.ComputeFreshness(time.Now())
		}
	}

	// Compute 3-tier service quality
	result.ServiceQuality = computeServiceQuality(itemsErr, result, time.Now())

	return result, nil
}

// enrichItemsWithTagHotspots adds tag_hotspot WhyReasons to items matching trending tags.
func enrichItemsWithTagHotspots(items []domain.KnowledgeHomeItem, trendingTags []knowledge_home_port.TrendingTag) []domain.KnowledgeHomeItem {
	if len(trendingTags) == 0 || len(items) == 0 {
		return items
	}

	trendingSet := make(map[string]bool, len(trendingTags))
	for _, t := range trendingTags {
		trendingSet[t.TagName] = true
	}

	for i := range items {
		if hasWhyCode(items[i].WhyReasons, domain.WhyTagHotspot) {
			continue
		}
		for _, tag := range items[i].Tags {
			if trendingSet[tag] {
				items[i].WhyReasons = append([]domain.WhyReason{{
					Code: domain.WhyTagHotspot,
					Tag:  tag,
				}}, items[i].WhyReasons...)
				break
			}
		}
	}
	return items
}

func hasWhyCode(reasons []domain.WhyReason, code string) bool {
	for _, r := range reasons {
		if r.Code == code {
			return true
		}
	}
	return false
}

// computeServiceQuality determines the 3-tier service quality based on error state.
//   - full: all read sources succeeded
//   - degraded: partial failure or stale projection, but the page can still render normally
//   - fallback: one of the read sections had to be dropped, but we still have a usable partial response
func computeServiceQuality(itemsErr error, result *Result, now time.Time) string {
	if itemsErr != nil {
		return ServiceQualityFallback
	}
	// Projection staleness downgrades quality, but is not itself a fallback response.
	if result.Digest.LastProjectedAt != nil {
		age := now.Sub(*result.Digest.LastProjectedAt)
		if age > degradedStalenessThreshold {
			return ServiceQualityDegraded
		}
	}
	if result.Degraded {
		return ServiceQualityDegraded
	}
	return ServiceQualityFull
}

func requestContextError(errs ...error) error {
	for _, err := range errs {
		switch {
		case errors.Is(err, context.Canceled):
			return context.Canceled
		case errors.Is(err, context.DeadlineExceeded):
			return context.DeadlineExceeded
		}
	}
	return nil
}
