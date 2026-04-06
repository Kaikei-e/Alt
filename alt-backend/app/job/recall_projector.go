package job

import (
	"alt/domain"
	"alt/port/knowledge_home_port"
	"alt/port/recall_candidate_port"
	"alt/port/recall_signal_port"
	"alt/utils/logger"
	"alt/utils/otel"
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
)

const (
	signalWindowDays = 7
	minRecallScore   = 0.2
)

// Recall reason weights.
const (
	weightOpenedNotRevisited    = 0.3
	weightRelatedToSearch       = 0.25
	weightRelatedToAugur        = 0.35
	weightRecapContextUnfinished = 0.2
	weightPulseFollowup         = 0.25
	weightTagInterest           = 0.15
	weightTagClicked            = 0.20
)

// RecallProjectorJob returns a function that scores recall candidates from signals.
// Users are dynamically discovered via ListDistinctUserIDsPort (from knowledge_home_items).
func RecallProjectorJob(
	listUsersPort knowledge_home_port.ListDistinctUserIDsPort,
	signalPort recall_signal_port.ListRecallSignalsByUserPort,
	candidatePort recall_candidate_port.UpsertRecallCandidatePort,
	metrics *otel.KnowledgeHomeMetrics,
) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		return processRecallSignals(ctx, listUsersPort, signalPort, candidatePort, metrics)
	}
}

func processRecallSignals(
	ctx context.Context,
	listUsersPort knowledge_home_port.ListDistinctUserIDsPort,
	signalPort recall_signal_port.ListRecallSignalsByUserPort,
	candidatePort recall_candidate_port.UpsertRecallCandidatePort,
	metrics *otel.KnowledgeHomeMetrics,
) error {
	start := time.Now()

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
	var emptyUsers int
	for _, userID := range userIDs {
		signals, err := signalPort.ListRecallSignalsByUser(ctx, userID, signalWindowDays)
		if err != nil {
			logger.Logger.ErrorContext(ctx, "recall projector: list signals failed",
				"error", err, "user_id", userID)
			continue
		}
		if len(signals) == 0 {
			emptyUsers++
			continue
		}

		candidates, err := scoreAndUpsertCandidates(ctx, userID, signals, candidatePort, metrics)
		if err != nil {
			logger.Logger.ErrorContext(ctx, "recall projector: scoring failed",
				"error", err, "user_id", userID)
			continue
		}
		if candidates == 0 {
			emptyUsers++
		}
		totalCandidates += candidates
	}

	if metrics != nil {
		elapsed := float64(time.Since(start).Milliseconds())
		metrics.RecallProjectorDurationMs.Record(ctx, elapsed)
		metrics.RecallProjectorUsersProcessed.Add(ctx, int64(len(userIDs)))
		metrics.RecallCandidateEmptyTotal.Add(ctx, int64(emptyUsers))
		if metrics.Snapshot != nil {
			metrics.Snapshot.RecordRecallProjectorDuration(elapsed)
			for range len(userIDs) {
				metrics.Snapshot.RecordRecallUserProcessed()
			}
			for range emptyUsers {
				metrics.Snapshot.RecordRecallCandidateEmpty()
			}
		}
	}

	logger.Logger.InfoContext(ctx, "recall projector: completed",
		slog.Int("users_processed", len(userIDs)),
		slog.Int("candidates_generated", totalCandidates))

	return nil
}

// scoreAndUpsertCandidates scores recall candidates from signals and returns the count generated.
func scoreAndUpsertCandidates(
	ctx context.Context,
	userID uuid.UUID,
	signals []domain.RecallSignal,
	candidatePort recall_candidate_port.UpsertRecallCandidatePort,
	metrics *otel.KnowledgeHomeMetrics,
) (int, error) {
	// Group signals by item_key
	itemSignals := make(map[string][]domain.RecallSignal)
	for _, s := range signals {
		itemSignals[s.ItemKey] = append(itemSignals[s.ItemKey], s)
	}

	now := time.Now()
	generated := 0
	for itemKey, sigs := range itemSignals {
		reasons := make([]domain.RecallReason, 0, len(sigs))
		var score float64

		for _, sig := range sigs {
			age := formatRelativeAge(sig.OccurredAt)
			switch sig.SignalType {
			case domain.SignalOpened:
				if time.Since(sig.OccurredAt) > 1*time.Hour {
					reasons = append(reasons, domain.RecallReason{
						Type:        domain.ReasonOpenedNotRevisited,
						Description: fmt.Sprintf("Opened %s, not revisited since", age),
					})
					score += weightOpenedNotRevisited
				}
			case domain.SignalSearchRelated:
				desc := fmt.Sprintf("Related to a search from %s", age)
				if q, ok := sig.Payload["search_query"].(string); ok && q != "" {
					desc = fmt.Sprintf("Related to your search for \"%s\" (%s)", q, age)
				}
				reasons = append(reasons, domain.RecallReason{
					Type:        domain.ReasonRelatedToRecentSearch,
					Description: desc,
				})
				score += weightRelatedToSearch
			case domain.SignalAugurReferenced:
				reasons = append(reasons, domain.RecallReason{
					Type:        domain.ReasonRelatedToAugurQ,
					Description: fmt.Sprintf("Referenced in an Augur question from %s", age),
				})
				score += weightRelatedToAugur
			case domain.SignalRecapContextUnread:
				reasons = append(reasons, domain.RecallReason{
					Type:        domain.ReasonRecapContextUnfinished,
					Description: fmt.Sprintf("Recap context from %s, still unread", age),
				})
				score += weightRecapContextUnfinished
			case domain.SignalPulseFollowup:
				reasons = append(reasons, domain.RecallReason{
					Type:        domain.ReasonPulseFollowupNeeded,
					Description: fmt.Sprintf("Pulse flagged for follow-up %s", age),
				})
				score += weightPulseFollowup
			case domain.SignalTagInterest:
				reasons = append(reasons, domain.RecallReason{
					Type:        domain.ReasonTagInterestOverlap,
					Description: fmt.Sprintf("Matches your interest tags (signal from %s)", age),
				})
				score += weightTagInterest
			case domain.SignalTagClicked:
				tag := ""
				if t, ok := sig.Payload["tag"].(string); ok {
					tag = t
				}
				desc := fmt.Sprintf("You explored tag \"%s\" (%s)", tag, age)
				reasons = append(reasons, domain.RecallReason{
					Type:        domain.ReasonTagInteraction,
					Description: desc,
				})
				score += weightTagClicked
			}
		}

		if score < minRecallScore {
			continue
		}

		candidate := domain.RecallCandidate{
			UserID:            userID,
			ItemKey:           itemKey,
			RecallScore:       score,
			Reasons:           reasons,
			NextSuggestAt:     &now,
			FirstEligibleAt:   &now,
			UpdatedAt:         now,
			ProjectionVersion: 1,
		}

		if err := candidatePort.UpsertRecallCandidate(ctx, candidate); err != nil {
			logger.Logger.ErrorContext(ctx, "recall projector: failed to upsert candidate",
				"error", err, "item_key", itemKey)
			continue
		}

		generated++
		if metrics != nil {
			metrics.RecallCandidateGeneratedTotal.Add(ctx, 1)
			if metrics.Snapshot != nil {
				metrics.Snapshot.RecordRecallCandidate()
			}
		}
	}

	return generated, nil
}

// formatRelativeAge returns a human-readable relative time string (e.g. "3 days ago").
func formatRelativeAge(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Hour:
		m := int(d.Minutes())
		if m <= 1 {
			return "1 minute ago"
		}
		return fmt.Sprintf("%d minutes ago", m)
	case d < 24*time.Hour:
		h := int(d.Hours())
		if h == 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", h)
	default:
		days := int(d.Hours() / 24)
		if days == 1 {
			return "1 day ago"
		}
		return fmt.Sprintf("%d days ago", days)
	}
}

// ScoreRecallCandidates processes signals for a single user and upserts candidates.
// Exported for testing.
func ScoreRecallCandidates(
	ctx context.Context,
	userID uuid.UUID,
	signals []domain.RecallSignal,
	candidatePort recall_candidate_port.UpsertRecallCandidatePort,
) error {
	_, err := scoreAndUpsertCandidates(ctx, userID, signals, candidatePort, nil)
	return err
}
