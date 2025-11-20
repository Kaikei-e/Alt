package morning_letter_port

import (
	"context"
	"time"

	"alt/domain"
)

// MorningRepository defines the interface for accessing morning letter data.
type MorningRepository interface {
	// GetMorningArticleGroups returns the article groups for the morning update within the specified time window.
	GetMorningArticleGroups(ctx context.Context, since time.Time) ([]*domain.MorningArticleGroup, error)
}

// MorningUsecase defines the interface for the morning letter business logic.
type MorningUsecase interface {
	// GetOvernightUpdates returns the overnight updates for a user.
	GetOvernightUpdates(ctx context.Context, userID string) ([]*domain.MorningUpdate, error)
}
