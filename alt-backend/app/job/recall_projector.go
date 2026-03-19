package job

import (
	"alt/domain"
	"alt/port/knowledge_home_port"
	"alt/port/recall_candidate_port"
	"alt/port/recall_signal_port"
	"alt/utils/logger"
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
)

const (
	recallProjectorName = "recall-rail-projector"
	signalWindowDays    = 7
	minRecallScore      = 0.2
)

// Recall reason weights.
const (
	weightOpenedNotRevisited    = 0.3
	weightRelatedToSearch       = 0.25
	weightRelatedToAugur        = 0.35
	weightRecapContextUnfinished = 0.2
	weightPulseFollowup         = 0.25
	weightTagInterest           = 0.15
)

// RecallProjectorJob returns a function that scores recall candidates from signals.
// Users are dynamically discovered via ListDistinctUserIDsPort (from knowledge_home_items).
func RecallProjectorJob(
	listUsersPort knowledge_home_port.ListDistinctUserIDsPort,
	signalPort recall_signal_port.ListRecallSignalsByUserPort,
	candidatePort recall_candidate_port.UpsertRecallCandidatePort,
) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		return processRecallSignals(ctx, listUsersPort, signalPort, candidatePort, nil)
	}
}

func processRecallSignals(
	ctx context.Context,
	listUsersPort knowledge_home_port.ListDistinctUserIDsPort,
	signalPort recall_signal_port.ListRecallSignalsByUserPort,
	candidatePort recall_candidate_port.UpsertRecallCandidatePort,
	_ interface{}, // reserved for homeItemsPort (future enrichment)
) error {
	userIDs, err := listUsersPort.ListDistinctUserIDs(ctx)
	if err != nil {
		return fmt.Errorf("recall projector: list distinct user IDs: %w", err)
	}

	logger.Logger.InfoContext(ctx, "recall projector: starting signal processing",
		slog.Int("user_count", len(userIDs)))

	if len(userIDs) == 0 {
		return nil
	}

	var totalCandidates int
	for _, userID := range userIDs {
		signals, err := signalPort.ListRecallSignalsByUser(ctx, userID, signalWindowDays)
		if err != nil {
			logger.Logger.ErrorContext(ctx, "recall projector: list signals failed",
				"error", err, "user_id", userID)
			continue
		}
		if len(signals) == 0 {
			continue
		}

		if err := ScoreRecallCandidates(ctx, userID, signals, candidatePort); err != nil {
			logger.Logger.ErrorContext(ctx, "recall projector: scoring failed",
				"error", err, "user_id", userID)
			continue
		}
		totalCandidates += len(signals)
	}

	logger.Logger.InfoContext(ctx, "recall projector: completed",
		slog.Int("users_processed", len(userIDs)),
		slog.Int("signals_processed", totalCandidates))

	return nil
}

// ScoreRecallCandidates processes signals for a single user and upserts candidates.
// Exported for testing.
func ScoreRecallCandidates(
	ctx context.Context,
	userID uuid.UUID,
	signals []domain.RecallSignal,
	candidatePort recall_candidate_port.UpsertRecallCandidatePort,
) error {
	// Group signals by item_key
	itemSignals := make(map[string][]domain.RecallSignal)
	for _, s := range signals {
		itemSignals[s.ItemKey] = append(itemSignals[s.ItemKey], s)
	}

	now := time.Now()
	for itemKey, sigs := range itemSignals {
		var reasons []domain.RecallReason
		var score float64

		for _, sig := range sigs {
			switch sig.SignalType {
			case domain.SignalOpened:
				// Check if opened but not revisited in 48h
				if time.Since(sig.OccurredAt) > 48*time.Hour {
					reasons = append(reasons, domain.RecallReason{
						Type:        domain.ReasonOpenedNotRevisited,
						Description: "You opened this but haven't revisited it",
					})
					score += weightOpenedNotRevisited
				}
			case domain.SignalSearchRelated:
				reasons = append(reasons, domain.RecallReason{
					Type:        domain.ReasonRelatedToRecentSearch,
					Description: "Related to your recent search",
				})
				score += weightRelatedToSearch
			case domain.SignalAugurReferenced:
				reasons = append(reasons, domain.RecallReason{
					Type:        domain.ReasonRelatedToAugurQ,
					Description: "Referenced in a recent Augur question",
				})
				score += weightRelatedToAugur
			case domain.SignalRecapContextUnread:
				reasons = append(reasons, domain.RecallReason{
					Type:        domain.ReasonRecapContextUnfinished,
					Description: "Recap context article left unread",
				})
				score += weightRecapContextUnfinished
			case domain.SignalPulseFollowup:
				reasons = append(reasons, domain.RecallReason{
					Type:        domain.ReasonPulseFollowupNeeded,
					Description: "Follow-up needed from pulse",
				})
				score += weightPulseFollowup
			case domain.SignalTagInterest:
				reasons = append(reasons, domain.RecallReason{
					Type:        domain.ReasonTagInterestOverlap,
					Description: "Matches your interest tags",
				})
				score += weightTagInterest
			}
		}

		if score < minRecallScore {
			continue
		}

		candidate := domain.RecallCandidate{
			UserID:          userID,
			ItemKey:         itemKey,
			RecallScore:     score,
			Reasons:         reasons,
			NextSuggestAt:   &now,
			FirstEligibleAt: &now,
			UpdatedAt:       now,
			ProjectionVersion: 1,
		}

		if err := candidatePort.UpsertRecallCandidate(ctx, candidate); err != nil {
			logger.Logger.ErrorContext(ctx, "recall projector: failed to upsert candidate",
				"error", err, "item_key", itemKey)
			continue
		}
	}

	return nil
}
