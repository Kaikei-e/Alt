// Package act_outcome_cron emits the 7-day no_engagement fallback outcome
// for KnowledgeLoopActed events that did not receive an explicit outcome
// from the immediate emitter path (alt-backend view trackers). It is the
// missing half of the ADR-000908 §Δ1 closure loop: without it, an acted
// event that the user simply ignored never feeds a signal back to Surface
// Planner v2, so the planner cannot down-weight content the user is
// actively skipping.
//
// Reproject-safety: the cron uses wall-clock only to decide which acted
// events have aged past the 7-day cutoff. The emitted outcome event's
// occurred_at is always derived from acted.occurred_at + 7d, so replaying
// the same event log against a fresh projection produces the same outcome
// rows regardless of when the cron actually fired.
package act_outcome_cron

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"knowledge-sovereign/driver/sovereign_db"
	"knowledge-sovereign/usecase/knowledge_loop_projector"
)

// PlannerName names the cron in checkpoint / log / metric labels. We keep
// it distinct from surface_planner_v2 so operators can read the two
// crons' progress independently.
const PlannerName = "act_outcome_cron"

const (
	defaultLensModeID = "default"
	defaultWindow     = 7 * 24 * time.Hour
	defaultBatchSize  = 256
	noEngagementLabel = "no_engagement"
)

// Repository captures the narrow surface the cron needs from sovereign_db.
// The NOT-EXISTS check belongs at the SQL boundary (the driver) so the
// cron does not have to walk the entire event log to detect which acted
// events still need a fallback outcome.
type Repository interface {
	ListActedEventsWithoutOutcome(ctx context.Context, cutoff time.Time, limit int) ([]sovereign_db.KnowledgeEvent, error)
	AppendKnowledgeEvent(ctx context.Context, ev sovereign_db.KnowledgeEvent) (int64, error)
}

// Config tunes the cron loop. Zero values fall back to defaults.
type Config struct {
	// BatchSize caps how many acted events one tick will process. Defaults to
	// 256 — large enough for steady-state coverage but small enough to keep
	// per-tick latency bounded.
	BatchSize int

	// Window is the silence period after which a missing outcome is filled
	// in with no_engagement. Defaults to 7 * 24h per ADR-000908.
	Window time.Duration

	// Clock is the source of wall-clock now used to compute the cutoff.
	// Tests pin a deterministic clock; production wires time.Now. Treated
	// strictly as a scan-cutoff identifier — it MUST NOT leak into emitted
	// event payloads. The emitted event's occurred_at is always derived
	// from acted.OccurredAt + Window.
	Clock func() time.Time

	// BackfillCutoff, when non-nil, replaces Clock() for the scan cutoff so
	// operators can run a backfill against the full historical event log
	// without the cron's wall-clock leaking into the batch boundary. The
	// event payloads are still pure (occurred_at = acted.OccurredAt + Window),
	// so reproject under the same event log yields identical output regardless
	// of which BackfillCutoff was used.
	BackfillCutoff *time.Time
}

// Cron is the no_engagement fallback producer.
type Cron struct {
	repo   Repository
	logger *slog.Logger
	cfg    Config
}

// New constructs a cron with sensible defaults applied.
func New(repo Repository, logger *slog.Logger, cfg Config) *Cron {
	if cfg.BatchSize == 0 {
		cfg.BatchSize = defaultBatchSize
	}
	if cfg.Window == 0 {
		cfg.Window = defaultWindow
	}
	if cfg.Clock == nil {
		cfg.Clock = time.Now
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Cron{repo: repo, logger: logger, cfg: cfg}
}

// RunBatch fetches acted events past the cutoff without a matching outcome
// and emits one no_engagement outcome per event. The emitted event's
// occurred_at is bound to acted.occurred_at + window so reproject is
// deterministic regardless of cron wall-clock.
//
// scanAt is the scan-boundary identifier: it picks which acted events have
// aged past the cutoff. It MUST NOT appear in any emitted event payload —
// that is the event-time-purity invariant tested by invariants_test.go.
// BackfillCutoff, when set, replaces Clock() so operators can run the cron
// against a historical window without leaking the current wall-clock.
func (c *Cron) RunBatch(ctx context.Context) error {
	var scanAt time.Time
	if c.cfg.BackfillCutoff != nil {
		scanAt = *c.cfg.BackfillCutoff
	} else {
		scanAt = c.cfg.Clock()
	}
	cutoff := scanAt.Add(-c.cfg.Window)

	events, err := c.repo.ListActedEventsWithoutOutcome(ctx, cutoff, c.cfg.BatchSize)
	if err != nil {
		return fmt.Errorf("%s: list acted-without-outcome: %w", PlannerName, err)
	}

	emitted := 0
	for _, acted := range events {
		if acted.UserID == nil {
			// System-emitted acted events would not enter this query path,
			// but guard against malformed upstream rather than crash the
			// batch.
			continue
		}
		ev, err := buildNoEngagementOutcome(acted, c.cfg.Window)
		if err != nil {
			c.logger.ErrorContext(ctx, "act_outcome_cron: build event failed",
				slog.String("err", err.Error()),
				slog.String("acted_event_id", acted.EventID.String()),
			)
			continue
		}
		if _, err := c.repo.AppendKnowledgeEvent(ctx, ev); err != nil {
			c.logger.ErrorContext(ctx, "act_outcome_cron: append failed",
				slog.String("err", err.Error()),
				slog.String("acted_event_id", acted.EventID.String()),
			)
			continue
		}
		knowledge_loop_projector.ObserveOutcomeMissingFill()
		emitted++
	}

	c.logger.InfoContext(ctx, "act_outcome_cron.batch_complete",
		slog.String("planner", PlannerName),
		slog.Time("cutoff", cutoff),
		slog.Int("scanned", len(events)),
		slog.Int("emitted", emitted),
	)
	return nil
}

// buildNoEngagementOutcome assembles the system-emitted closure event for
// a single acted event. The dedupe_key keys on (event_type, acted_event_id,
// outcome) so re-running the cron is a no-op at the AppendKnowledgeEvent
// layer.
func buildNoEngagementOutcome(acted sovereign_db.KnowledgeEvent, window time.Duration) (sovereign_db.KnowledgeEvent, error) {
	entryKey := readEntryKey(acted.Payload)
	if entryKey == "" {
		entryKey = acted.AggregateID
	}
	observedAt := acted.OccurredAt.Add(window)

	body := map[string]any{
		"acted_event_id": acted.EventID.String(),
		"entry_key":      entryKey,
		"lens_mode_id":   defaultLensModeID,
		"outcome":        noEngagementLabel,
		"observed_at":    observedAt.UTC().Format(time.RFC3339Nano),
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return sovereign_db.KnowledgeEvent{}, err
	}

	dedupeKey := fmt.Sprintf("%s:%s:%s",
		knowledge_loop_projector.EventKnowledgeLoopActOutcome,
		acted.EventID.String(),
		noEngagementLabel,
	)

	uid := *acted.UserID
	return sovereign_db.KnowledgeEvent{
		EventID:       uuid.New(),
		OccurredAt:    observedAt,
		TenantID:      acted.TenantID,
		UserID:        &uid,
		ActorType:     "system",
		ActorID:       PlannerName,
		EventType:     knowledge_loop_projector.EventKnowledgeLoopActOutcome,
		AggregateType: "knowledge_loop_entry",
		AggregateID:   entryKey,
		DedupeKey:     dedupeKey,
		Payload:       payload,
	}, nil
}

// readEntryKey pulls entry_key out of a JSON payload using the same
// alternative key set the projector accepts.
func readEntryKey(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return ""
	}
	for _, k := range []string{"entry_key", "item_key", "entryKey", "itemKey"} {
		if v, ok := m[k]; ok {
			if s, ok := v.(string); ok && s != "" {
				return s
			}
		}
	}
	return ""
}
