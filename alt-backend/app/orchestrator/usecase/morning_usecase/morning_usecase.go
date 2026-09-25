package morning_usecase

import (
	"context"
	"fmt"
	"time"

	"alt/domain"
	"alt/orchestrator/port/morning_letter_port"
	"alt/orchestrator/port/user_feed_port"
)

type morningUsecase struct {
	repo         morning_letter_port.MorningRepository
	userFeedPort user_feed_port.UserFeedPort
}

func NewMorningUsecase(repo morning_letter_port.MorningRepository, userFeedPort user_feed_port.UserFeedPort) morning_letter_port.MorningUsecase {
	return &morningUsecase{
		repo:         repo,
		userFeedPort: userFeedPort,
	}
}

func (u *morningUsecase) GetOvernightUpdates(ctx context.Context, userID string) ([]*domain.MorningUpdate, error) {
	// Get user's subscribed feed IDs from context (same pattern as cursor-based endpoints)
	feedIDs, err := u.userFeedPort.GetUserFeedIDs(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get user feed IDs: %w", err)
	}

	// Define "overnight" as past 24 hours for now
	since := time.Now().Add(-24 * time.Hour)

	// Get all groups from recap-worker
	groups, err := u.repo.GetMorningArticleGroups(ctx, since)
	if err != nil {
		return nil, err
	}

	return groupMorningUpdates(groups, feedIDs), nil
}
