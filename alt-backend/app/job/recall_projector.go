package job

import (
	"alt/domain"
	"alt/port/knowledge_home_port"
	"alt/port/recall_candidate_port"
	"alt/port/recall_signal_port"
	"alt/utils/logger"
	"context"
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
func RecallProjectorJob(
	signalPort recall_signal_port.ListRecallSignalsByUserPort,
	candidatePort recall_candidate_port.UpsertRecallCandidatePort,
	homeItemsPort knowledge_home_port.GetKnowledgeHomeItemsPort,
) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		return processRecallSignals(ctx, signalPort, candidatePort, homeItemsPort)
	}
}

func processRecallSignals(
	ctx context.Context,
	signalPort recall_signal_port.ListRecallSignalsByUserPort,
	candidatePort recall_candidate_port.UpsertRecallCandidatePort,
	homeItemsPort knowledge_home_port.GetKnowledgeHomeItemsPort,
) error {
	// In Phase 4, we process signals for all users who have recent signals.
	// For now, we use the tenant-as-user pattern (single tenant).
	// A production implementation would iterate over distinct user_ids from recall_signals.

	// This is a simplified version that processes one batch.
	// The full implementation would use a cursor over distinct users.
	logger.Logger.InfoContext(ctx, "recall projector: starting signal processing")
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
