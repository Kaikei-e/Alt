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

	// Create a map for quick lookup
	feedIDMap := make(map[uuid.UUID]bool)
	for _, feedID := range feedIDs {
		feedIDMap[feedID] = true
	}

	// Define "overnight" as past 24 hours for now
	since := time.Now().Add(-24 * time.Hour)

	// Get all groups from recap-worker
	groups, err := u.repo.GetMorningArticleGroups(ctx, since)
	if err != nil {
		return nil, err
	}

	// Filter groups by user's subscribed feeds
	var filteredGroups []*domain.MorningArticleGroup
	for _, g := range groups {
		if g.Article != nil && feedIDMap[g.Article.FeedID] {
			filteredGroups = append(filteredGroups, g)
		}
	}

	// Group by GroupID
	groupedMap := make(map[string]*domain.MorningUpdate)
	for _, g := range filteredGroups {
		groupIDStr := g.GroupID.String()
		if _, exists := groupedMap[groupIDStr]; !exists {
			groupedMap[groupIDStr] = &domain.MorningUpdate{
				GroupID:    g.GroupID,
				Duplicates: []*domain.Article{},
			}
		}

		update := groupedMap[groupIDStr]
		if g.IsPrimary {
			update.PrimaryArticle = g.Article
		} else {
			update.Duplicates = append(update.Duplicates, g.Article)
		}
	}

	// Convert map to slice
	var updates []*domain.MorningUpdate
	for _, update := range groupedMap {
		if update.PrimaryArticle != nil {
			updates = append(updates, update)
		}
	}

	return updates, nil
}
