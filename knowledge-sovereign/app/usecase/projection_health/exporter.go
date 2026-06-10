// Package projection_health publishes DB-truth producer-liveness gauges sampled
// on a timer: the per-event last-occurrence age. A producer (recap topic
// snapshot, summary/tag versioning) dying while the rest of the pipeline stays
// fresh shows up as a climbing age, distinguishing "dead producer" from
// "no usage".
package projection_health

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// neverSeenAgeSeconds is the sentinel age published for a watched event type
// that has never been emitted. 10 years — far above any liveness threshold, so
// a producer that has never wired up (the recap.topic_snapshotted.v1 bug
// ADR-000939 fixed) reads as extremely stale rather than as an absent series.
const neverSeenAgeSeconds = 10 * 365 * 24 * 3600.0

// WatchedEventTypes are the producers whose liveness the gate tracks, plus
// SummaryVersionCreated as the "pipeline alive" reference the recap-dead alert
// joins against.
var WatchedEventTypes = []string{
	"recap.topic_snapshotted.v1",
	"SummarySuperseded",
	"TagSetVersionCreated",
	"SummaryVersionCreated",
}

var (
	eventLastOccurrenceAge = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "knowledge_event",
		Subsystem: "",
		Name:      "last_occurrence_age_seconds",
		Help:      "Age in seconds of the most recent event of this type (producer liveness). A large value for recap.topic_snapshotted.v1 / augur.conversation_linked.v1 while SummaryVersionCreated stays fresh signals a dead producer, not no usage.",
	}, []string{"event_type"})
)

// Repository is the narrow read surface the exporter needs.
type Repository interface {
	GetKnowledgeEventLastOccurrenceAges(ctx context.Context, eventTypes []string) (map[string]float64, error)
}

// Exporter samples the projection-health queries and publishes the gauges.
type Exporter struct {
	repo   Repository
	logger *slog.Logger
}

func New(repo Repository, logger *slog.Logger) *Exporter {
	if logger == nil {
		logger = slog.Default()
	}
	return &Exporter{repo: repo, logger: logger}
}

// RunOnce samples both signals and updates the gauges. Returns an error if a
// query fails (the caller logs it); a transient DB error just means the gauges
// hold their last value until the next tick.
func (e *Exporter) RunOnce(ctx context.Context) error {
	ages, err := e.repo.GetKnowledgeEventLastOccurrenceAges(ctx, WatchedEventTypes)
	if err != nil {
		return fmt.Errorf("projection_health: event ages: %w", err)
	}
	for _, et := range WatchedEventTypes {
		age, ok := ages[et]
		if !ok {
			age = neverSeenAgeSeconds
		}
		eventLastOccurrenceAge.WithLabelValues(et).Set(age)
	}
	return nil
}
