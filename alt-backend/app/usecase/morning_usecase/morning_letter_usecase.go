package morning_usecase

import (
	"context"
	"fmt"
	"time"

	"alt/domain"
	"alt/port/morning_letter_port"
	"alt/port/user_feed_port"

	"github.com/google/uuid"
)

type morningLetterUsecase struct {
	repo         morning_letter_port.MorningLetterRepository
	userFeedPort user_feed_port.UserFeedPort
}

func NewMorningLetterUsecase(
	repo morning_letter_port.MorningLetterRepository,
	userFeedPort user_feed_port.UserFeedPort,
) morning_letter_port.MorningLetterUsecase {
	return &morningLetterUsecase{repo: repo, userFeedPort: userFeedPort}
}

func (u *morningLetterUsecase) GetLatestLetter(ctx context.Context) (*domain.MorningLetterDocument, error) {
	return u.repo.GetLatestLetter(ctx)
}

func (u *morningLetterUsecase) GetLetterByDate(ctx context.Context, targetDate string) (*domain.MorningLetterDocument, error) {
	if _, err := time.Parse("2006-01-02", targetDate); err != nil {
		return nil, fmt.Errorf("invalid date format: %q (expected YYYY-MM-DD): %w", targetDate, err)
	}
	return u.repo.GetLetterByDate(ctx, targetDate)
}

func (u *morningLetterUsecase) GetLetterSources(ctx context.Context, letterID string) ([]*domain.MorningLetterSourceEntry, error) {
	feedIDs, err := u.userFeedPort.GetUserFeedIDs(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get user feed IDs: %w", err)
	}
	feedIDSet := make(map[uuid.UUID]bool, len(feedIDs))
	for _, id := range feedIDs {
		feedIDSet[id] = true
	}

	sources, err := u.repo.GetLetterSources(ctx, letterID)
	if err != nil {
		return nil, fmt.Errorf("failed to get letter sources: %w", err)
	}

	filtered := make([]*domain.MorningLetterSourceEntry, 0, len(sources))
	for _, s := range sources {
		if feedIDSet[s.FeedID] {
			filtered = append(filtered, s)
		}
	}
	return filtered, nil
}
