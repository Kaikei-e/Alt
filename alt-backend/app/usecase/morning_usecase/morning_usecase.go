package morning_usecase

import (
	"context"
	"time"

	"alt/domain"
	"alt/port/morning_letter_port"
)

type morningUsecase struct {
	repo morning_letter_port.MorningRepository
}

func NewMorningUsecase(repo morning_letter_port.MorningRepository) morning_letter_port.MorningUsecase {
	return &morningUsecase{
		repo: repo,
	}
}

func (u *morningUsecase) GetOvernightUpdates(ctx context.Context, userID string) ([]*domain.MorningUpdate, error) {
	// Define "overnight" as past 24 hours for now
	since := time.Now().Add(-24 * time.Hour)

	groups, err := u.repo.GetMorningArticleGroups(ctx, since)
	if err != nil {
		return nil, err
	}

	// Group by GroupID
	groupedMap := make(map[string]*domain.MorningUpdate)
	for _, g := range groups {
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
