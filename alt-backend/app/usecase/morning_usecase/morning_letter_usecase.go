package morning_usecase

import (
	"context"
	"fmt"
	"sync"
	"time"

	"alt/domain"
	"alt/port/morning_letter_port"
	"alt/port/user_feed_port"

	"github.com/google/uuid"
)

// regenerateCooldown enforces a per-user minimum interval between manual
// Morning Letter regeneration requests. Process-local; acceptable as a
// best-effort client-side guard in addition to recap-worker's own checks.
const regenerateCooldown = time.Hour

type morningLetterUsecase struct {
	repo         morning_letter_port.MorningLetterRepository
	userFeedPort user_feed_port.UserFeedPort

	// Optional enrichment ports. When any is nil the enrichment RPC
	// degrades gracefully (articles still link, tags/related/feed title
	// may be empty). This lets the constructor stay backward-compatible
	// for existing unit tests.
	articleBatch   morning_letter_port.ArticleMetadataBatchPort
	feedTitleBatch morning_letter_port.FeedTitleBatchPort
	searchRelated  morning_letter_port.SearchRelatedArticlesPort

	regenMu   sync.Mutex
	regenLast map[string]time.Time
}

func NewMorningLetterUsecase(
	repo morning_letter_port.MorningLetterRepository,
	userFeedPort user_feed_port.UserFeedPort,
) morning_letter_port.MorningLetterUsecase {
	return &morningLetterUsecase{
		repo:         repo,
		userFeedPort: userFeedPort,
		regenLast:    make(map[string]time.Time),
	}
}

// NewMorningLetterUsecaseWithEnrichment wires in the extra ports needed
// by GetLetterEnrichment. Kept as a separate constructor so tests that
// don't exercise enrichment can keep using NewMorningLetterUsecase.
func NewMorningLetterUsecaseWithEnrichment(
	repo morning_letter_port.MorningLetterRepository,
	userFeedPort user_feed_port.UserFeedPort,
	articleBatch morning_letter_port.ArticleMetadataBatchPort,
	feedTitleBatch morning_letter_port.FeedTitleBatchPort,
	searchRelated morning_letter_port.SearchRelatedArticlesPort,
) morning_letter_port.MorningLetterUsecase {
	return &morningLetterUsecase{
		repo:           repo,
		userFeedPort:   userFeedPort,
		articleBatch:   articleBatch,
		feedTitleBatch: feedTitleBatch,
		searchRelated:  searchRelated,
		regenLast:      make(map[string]time.Time),
	}
}

// RegenerateLatest enforces a 1-hour per-user cooldown. When within the
// cooldown window, returns (latestCached, false, retryAfter, nil).
func (u *morningLetterUsecase) RegenerateLatest(
	ctx context.Context,
	userID, editionTimezone string,
) (*domain.MorningLetterDocument, bool, time.Duration, error) {
	if userID == "" {
		return nil, false, 0, fmt.Errorf("user_id required for regenerate")
	}

	u.regenMu.Lock()
	last, ok := u.regenLast[userID]
	now := time.Now()
	if ok && now.Sub(last) < regenerateCooldown {
		u.regenMu.Unlock()
		doc, err := u.repo.GetLatestLetter(ctx)
		if err != nil {
			return nil, false, 0, fmt.Errorf("rate-limited and failed to load cached letter: %w", err)
		}
		return doc, false, regenerateCooldown - now.Sub(last), nil
	}
	u.regenLast[userID] = now
	u.regenMu.Unlock()

	doc, err := u.repo.RegenerateLatest(ctx, editionTimezone)
	if err != nil {
		// Roll back the timestamp so the user isn't penalised for a failed run.
		u.regenMu.Lock()
		delete(u.regenLast, userID)
		u.regenMu.Unlock()
		return nil, false, 0, fmt.Errorf("regenerate failed: %w", err)
	}
	return doc, true, 0, nil
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
