package get_knowledge_home_usecase

import (
	"alt/domain"
	"alt/port/knowledge_home_port"
	"alt/port/today_digest_port"
	"alt/utils/logger"
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

// GetKnowledgeHomeUsecase orchestrates fetching the Knowledge Home data.
type GetKnowledgeHomeUsecase struct {
	homeItemsPort   knowledge_home_port.GetKnowledgeHomeItemsPort
	todayDigestPort today_digest_port.GetTodayDigestPort
}

// NewGetKnowledgeHomeUsecase creates a new GetKnowledgeHomeUsecase.
func NewGetKnowledgeHomeUsecase(
	homeItemsPort knowledge_home_port.GetKnowledgeHomeItemsPort,
	todayDigestPort today_digest_port.GetTodayDigestPort,
) *GetKnowledgeHomeUsecase {
	return &GetKnowledgeHomeUsecase{
		homeItemsPort:   homeItemsPort,
		todayDigestPort: todayDigestPort,
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
func (u *GetKnowledgeHomeUsecase) Execute(ctx context.Context, userID uuid.UUID, cursor string, limit int, date time.Time) (*Result, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	result := &Result{
		GeneratedAt: time.Now(),
	}

	// Fetch items
	items, nextCursor, hasMore, err := u.homeItemsPort.GetKnowledgeHomeItems(ctx, userID, cursor, limit)
	if err != nil {
		logger.Logger.ErrorContext(ctx, "failed to fetch knowledge home items", "error", err)
		result.Degraded = true
	} else {
		result.Items = items
		result.NextCursor = nextCursor
		result.HasMore = hasMore
	}

	// Fetch today digest
	digest, err := u.todayDigestPort.GetTodayDigest(ctx, userID, date)
	if err != nil {
		logger.Logger.ErrorContext(ctx, "failed to fetch today digest", "error", err)
		result.Degraded = true
	} else {
		result.Digest = digest
	}

	// Both failed = return error
	if result.Items == nil && errors.Is(err, context.Canceled) {
		return nil, err
	}

	return result, nil
}
