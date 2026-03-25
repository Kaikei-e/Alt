package get_knowledge_home_usecase

import (
	"alt/domain"
	"alt/port/knowledge_home_port"
	"alt/port/knowledge_lens_port"
	"alt/port/today_digest_port"
	"alt/utils/logger"
	"context"
	"errors"
	"fmt"
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

	// Fetch items
	items, nextCursor, hasMore, itemsErr := u.homeItemsPort.GetKnowledgeHomeItems(ctx, userID, cursor, limit, lensFilter)
	if itemsErr != nil {
		logger.Logger.ErrorContext(ctx, "failed to fetch knowledge home items", "error", itemsErr)
		result.Degraded = true
	} else {
		result.Items = items
		result.NextCursor = nextCursor
		result.HasMore = hasMore
	}

	// Fetch today digest
	digest, digestErr := u.todayDigestPort.GetTodayDigest(ctx, userID, date)
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
	u.enrichTagHotspots(ctx, result, userID)

	// Enrich digest with backend-authoritative values
	u.enrichDigest(ctx, result, userID, date)

	// Compute 3-tier service quality
	result.ServiceQuality = computeServiceQuality(itemsErr, result)

	return result, nil
}

// enrichTagHotspots adds tag_hotspot WhyReasons to items whose tags match currently trending tags.
func (u *GetKnowledgeHomeUsecase) enrichTagHotspots(ctx context.Context, result *Result, userID uuid.UUID) {
	if u.tagHotspotPort == nil || len(result.Items) == 0 {
		return
	}

	trendingTags, err := u.tagHotspotPort.GetTrendingTags(ctx, userID)
	if err != nil {
		logger.Logger.WarnContext(ctx, "failed to get trending tags for enrichment", "error", err)
		return
	}
	if len(trendingTags) == 0 {
		return
	}

	trendingSet := make(map[string]bool, len(trendingTags))
	for _, t := range trendingTags {
		trendingSet[t.TagName] = true
	}

	for i := range result.Items {
		item := &result.Items[i]
		if hasWhyCode(item.WhyReasons, domain.WhyTagHotspot) {
			continue
		}
		for _, tag := range item.Tags {
			if trendingSet[tag] {
				item.WhyReasons = append(item.WhyReasons, domain.WhyReason{
					Code: domain.WhyTagHotspot,
					Tag:  tag,
				})
				break
			}
		}
	}
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
func computeServiceQuality(itemsErr error, result *Result) string {
	if itemsErr != nil {
		return ServiceQualityFallback
	}
	// Projection staleness downgrades quality, but is not itself a fallback response.
	if result.Digest.LastProjectedAt != nil {
		age := time.Since(*result.Digest.LastProjectedAt)
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

// enrichDigest populates freshness and accurate count on the digest.
// Failures are logged but do not hard-fail the request.
func (u *GetKnowledgeHomeUsecase) enrichDigest(ctx context.Context, result *Result, userID uuid.UUID, date time.Time) {
	// Enrich needToKnowCount from backend query
	if u.needToKnowCountPort != nil {
		count, err := u.needToKnowCountPort.CountNeedToKnowItems(ctx, userID, date)
		if err != nil {
			logger.Logger.ErrorContext(ctx, "failed to count need-to-know items", "error", err)
			// Keep original value (which may be 0 from the digest)
			result.Digest.NeedToKnowCount = 0
		} else {
			result.Digest.NeedToKnowCount = count
		}
	}

	// Enrich freshness from projector checkpoint
	if u.freshnessPort != nil {
		updatedAt, err := u.freshnessPort.GetProjectionFreshness(ctx, projectorName)
		if err != nil {
			logger.Logger.ErrorContext(ctx, "failed to get projection freshness", "error", err)
			result.Digest.DigestFreshness = domain.FreshnessUnknown
		} else {
			result.Digest.LastProjectedAt = updatedAt
			result.Digest.DigestFreshness = result.Digest.ComputeFreshness(time.Now())
		}
	}
}
