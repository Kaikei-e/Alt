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

// recallWeightSet maps a signal_code to (weight, isNegative). Negative
// signals dampen the score so content the user has actively pushed away
// stops cycling back into the rail (ADR-000913 §D-9, Twitter Heavy-Ranker
// grounding).
type recallWeightSet struct {
	version string
	weights map[string]recallSignalWeight
}

type recallSignalWeight struct {
	weight     float64
	isNegative bool
}

// Reason weights for the v1 fixed weight set. The numbers retain the
// pre-Heavy-Ranker semantics so legacy candidate rows are unchanged when
// the projector keeps v1 selected.
var recallWeightSetV1 = recallWeightSet{
	version: domain.RecallWeightSetV1Fixed,
	weights: map[string]recallSignalWeight{
		domain.SignalOpened:             {weight: 0.30},
		domain.SignalSearchRelated:      {weight: 0.25},
		domain.SignalAugurReferenced:    {weight: 0.35},
		domain.SignalRecapContextUnread: {weight: 0.20},
		domain.SignalPulseFollowup:      {weight: 0.25},
		domain.SignalTagInterest:        {weight: 0.15},
		domain.SignalTagClicked:         {weight: 0.20},
	},
}

// v2 Heavy-Ranker weights: positive signals lean slightly heavier on
// explicit Augur references (the strongest "this matters to me" signal)
// and introduce explicit negative weights for dismissed / low-confidence
// content. The numbers are normalised so a single negative signal cancels
// roughly one positive signal of equal magnitude.
var recallWeightSetV2HeavyRanker = recallWeightSet{
	version: domain.RecallWeightSetV2HeavyRanker,
	weights: map[string]recallSignalWeight{
		domain.SignalOpened:             {weight: 0.30},
		domain.SignalSearchRelated:      {weight: 0.25},
		domain.SignalAugurReferenced:    {weight: 0.45},
		domain.SignalRecapContextUnread: {weight: 0.20},
		domain.SignalPulseFollowup:      {weight: 0.25},
		domain.SignalTagInterest:        {weight: 0.15},
		domain.SignalTagClicked:         {weight: 0.20},

		domain.SignalRecentlyDismissed:    {weight: -0.40, isNegative: true},
		domain.SignalLowSummaryConfidence: {weight: -0.20, isNegative: true},
	},
}

// resolveRecallWeightSet picks the weights map by the
// KNOWLEDGE_HOME_RECALL_WEIGHT_SET environment variable. Default is v1 so
// the migration is opt-in and back-compatible.
func resolveRecallWeightSet(name string) recallWeightSet {
	switch name {
	case domain.RecallWeightSetV2HeavyRanker:
		return recallWeightSetV2HeavyRanker
	}
	return recallWeightSetV1
}

// Per-signal reason metadata used to describe the contribution to the user.
type recallReasonTemplate struct {
	reasonType string
	descTpl    func(age string, payload map[string]any) string
}

var recallReasonTemplates = map[string]recallReasonTemplate{
	domain.SignalOpened: {
		reasonType: domain.ReasonOpenedNotRevisited,
		descTpl: func(age string, _ map[string]any) string {
			return fmt.Sprintf("Opened %s, not revisited since", age)
		},
	},
	domain.SignalSearchRelated: {
		reasonType: domain.ReasonRelatedToRecentSearch,
		descTpl: func(age string, p map[string]any) string {
			if q, ok := p["search_query"].(string); ok && q != "" {
				return fmt.Sprintf("Related to your search for \"%s\" (%s)", q, age)
			}
			return fmt.Sprintf("Related to a search from %s", age)
		},
	},
	domain.SignalAugurReferenced: {
		reasonType: domain.ReasonRelatedToAugurQ,
		descTpl: func(age string, _ map[string]any) string {
			return fmt.Sprintf("Referenced in an Augur question from %s", age)
		},
	},
	domain.SignalRecapContextUnread: {
		reasonType: domain.ReasonRecapContextUnfinished,
		descTpl: func(age string, _ map[string]any) string {
			return fmt.Sprintf("Recap context from %s, still unread", age)
		},
	},
	domain.SignalPulseFollowup: {
		reasonType: domain.ReasonPulseFollowupNeeded,
		descTpl: func(age string, _ map[string]any) string {
			return fmt.Sprintf("Pulse flagged for follow-up %s", age)
		},
	},
	domain.SignalTagInterest: {
		reasonType: domain.ReasonTagInterestOverlap,
		descTpl: func(age string, _ map[string]any) string {
			return fmt.Sprintf("Matches your interest tags (signal from %s)", age)
		},
	},
	domain.SignalTagClicked: {
		reasonType: domain.ReasonTagInteraction,
		descTpl: func(age string, p map[string]any) string {
			tag := ""
			if t, ok := p["tag"].(string); ok {
				tag = t
			}
			return fmt.Sprintf("You explored tag \"%s\" (%s)", tag, age)
		},
	},
}

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

// recallScoreResult is the pure output of ScoreRecallSignals — no
// side-effects, no UpsertRecallCandidate call, no time.Now(). The caller
// converts each result into a domain.RecallCandidate.
type recallScoreResult struct {
	ItemKey       string
	Score         float64
	Reasons       []domain.RecallReason
	Breakdown     []domain.RecallScoreContribution
	FreshnessAt   time.Time
	WeightSetUsed string
}

// ScoreRecallSignals applies a weight set to a flat slice of signals and
// returns one result per item. Pure function: same inputs always produce
// the same outputs, so reproject is deterministic. time.Now() is NEVER
// consulted — freshness is the max(OccurredAt) of the signals contributing
// to the item.
func ScoreRecallSignals(signals []domain.RecallSignal, weightSet recallWeightSet) []recallScoreResult {
	itemSignals := make(map[string][]domain.RecallSignal, len(signals))
	for _, s := range signals {
		itemSignals[s.ItemKey] = append(itemSignals[s.ItemKey], s)
	}

	results := make([]recallScoreResult, 0, len(itemSignals))
	for itemKey, sigs := range itemSignals {
		reasons := make([]domain.RecallReason, 0, len(sigs))
		breakdown := make([]domain.RecallScoreContribution, 0, len(sigs))
		var score float64
		var freshness time.Time

		for _, sig := range sigs {
			weight, known := weightSet.weights[sig.SignalType]
			if !known {
				continue
			}
			// SignalOpened keeps its v1 "older than an hour" guard so the
			// reason stays meaningful — we do this purely from event-time,
			// never wall-clock, by checking the signal's age against the
			// most recent signal in the same batch. Without that guard
			// fresh opens would race the recall rail into recommending an
			// article the user is literally reading right now.
			if sig.SignalType == domain.SignalOpened && sig.OccurredAt.After(freshness) {
				freshness = sig.OccurredAt
			}
			if sig.OccurredAt.After(freshness) {
				freshness = sig.OccurredAt
			}

			contribution := weight.weight
			score += contribution
			breakdown = append(breakdown, domain.RecallScoreContribution{
				SignalCode:   sig.SignalType,
				Weight:       weight.weight,
				Contribution: contribution,
				IsNegative:   weight.isNegative,
			})
			if tmpl, ok := recallReasonTemplates[sig.SignalType]; ok {
				reasons = append(reasons, domain.RecallReason{
					Type:        tmpl.reasonType,
					Description: tmpl.descTpl(formatRelativeAgeFromBase(sig.OccurredAt, freshness), sig.Payload),
				})
			}
		}
		results = append(results, recallScoreResult{
			ItemKey:       itemKey,
			Score:         score,
			Reasons:       reasons,
			Breakdown:     breakdown,
			FreshnessAt:   freshness,
			WeightSetUsed: weightSet.version,
		})
	}
	return results
}

// scoreAndUpsertCandidates scores recall candidates from signals and returns the count generated.
func scoreAndUpsertCandidates(
	ctx context.Context,
	userID uuid.UUID,
	signals []domain.RecallSignal,
	candidatePort recall_candidate_port.UpsertRecallCandidatePort,
	metrics *otel.KnowledgeHomeMetrics,
) (int, error) {
	weightSet := resolveRecallWeightSet(currentRecallWeightSet)
	results := ScoreRecallSignals(signals, weightSet)

	generated := 0
	for _, r := range results {
		if r.Score < minRecallScore {
			continue
		}
		fresh := r.FreshnessAt
		candidate := domain.RecallCandidate{
			UserID:            userID,
			ItemKey:           r.ItemKey,
			RecallScore:       r.Score,
			Reasons:           r.Reasons,
			NextSuggestAt:     &fresh,
			FirstEligibleAt:   &fresh,
			UpdatedAt:         fresh,
			ProjectionVersion: 1,
			WeightSetVersion:  r.WeightSetUsed,
			ScoreBreakdown:    r.Breakdown,
		}

		if err := candidatePort.UpsertRecallCandidate(ctx, candidate); err != nil {
			logger.Logger.ErrorContext(ctx, "recall projector: failed to upsert candidate",
				"error", err, "item_key", r.ItemKey)
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

// currentRecallWeightSet is the runtime selector for the weight set. It is
// initialised from KNOWLEDGE_HOME_RECALL_WEIGHT_SET in main.go via
// SetRecallWeightSet so tests can pin a value without touching env.
var currentRecallWeightSet = domain.RecallWeightSetV1Fixed

// SetRecallWeightSet pins the active weight set version. Call once from
// main.go after reading the env var.
func SetRecallWeightSet(name string) {
	if name == "" {
		return
	}
	currentRecallWeightSet = name
}

// formatRelativeAgeFromBase returns a human-readable age of t relative to
// base. base is the MAX(OccurredAt) of the signals being scored, so the
// description stays event-time pure across replay.
func formatRelativeAgeFromBase(t, base time.Time) string {
	if base.IsZero() || t.IsZero() || !base.After(t) {
		return formatRelativeAge(t)
	}
	d := base.Sub(t)
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
