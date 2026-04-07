package morning_letter_port

import (
	"context"
	"time"

	"alt/domain"
)

// MorningRepository defines the interface for accessing morning article groups.
type MorningRepository interface {
	GetMorningArticleGroups(ctx context.Context, since time.Time) ([]*domain.MorningArticleGroup, error)
}

// MorningUsecase defines the interface for the morning letter business logic (overnight updates).
type MorningUsecase interface {
	GetOvernightUpdates(ctx context.Context, userID string) ([]*domain.MorningUpdate, error)
}

// MorningLetterRepository defines data access for Morning Letter documents (via recap-worker REST).
type MorningLetterRepository interface {
	GetLatestLetter(ctx context.Context) (*domain.MorningLetterDocument, error)
	GetLetterByDate(ctx context.Context, targetDate string) (*domain.MorningLetterDocument, error)
	GetLetterSources(ctx context.Context, letterID string) ([]*domain.MorningLetterSourceEntry, error)
}

// MorningLetterUsecase defines business logic for reading Morning Letters with subscription filtering.
type MorningLetterUsecase interface {
	GetLatestLetter(ctx context.Context) (*domain.MorningLetterDocument, error)
	GetLetterByDate(ctx context.Context, targetDate string) (*domain.MorningLetterDocument, error)
	GetLetterSources(ctx context.Context, letterID string) ([]*domain.MorningLetterSourceEntry, error)
}
