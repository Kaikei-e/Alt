package get_knowledge_home_usecase

import (
	"alt/domain"
	"alt/port/knowledge_home_port"
	"alt/port/knowledge_lens_port"
	"alt/port/today_digest_port"
	"alt/utils/logger"
	"context"
	"errors"
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
}

// NewGetKnowledgeHomeUsecase creates a new GetKnowledgeHomeUsecase.
func NewGetKnowledgeHomeUsecase(
	homeItemsPort knowledge_home_port.GetKnowledgeHomeItemsPort,
	todayDigestPort today_digest_port.GetTodayDigestPort,
	resolveLensPort knowledge_lens_port.ResolveKnowledgeHomeLensPort,
	freshnessPort today_digest_port.GetProjectionFreshnessPort,
	needToKnowCountPort today_digest_port.CountNeedToKnowItemsPort,
) *GetKnowledgeHomeUsecase {
	return &GetKnowledgeHomeUsecase{
		homeItemsPort:       homeItemsPort,
		todayDigestPort:     todayDigestPort,
		resolveLensPort:     resolveLensPort,
		freshnessPort:       freshnessPort,
		needToKnowCountPort: needToKnowCountPort,
	}
}

// Result holds the output of GetKnowledgeHome.
type Result struct {
	Items       []domain.KnowledgeHomeItem
	Digest      domain.TodayDigest
	NextCursor  string
	HasMore     bool
	Degraded    bool
	GeneratedAt time.Time
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

	// Enrich digest with backend-authoritative values
	u.enrichDigest(ctx, result, userID, date)

	return result, nil
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
