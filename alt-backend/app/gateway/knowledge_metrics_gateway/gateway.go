package knowledge_metrics_gateway

import (
	"alt/domain"
	altotel "alt/utils/otel"
	"context"
)

// Gateway reads system metrics from the in-memory OTel snapshot.
type Gateway struct {
	snapshot *altotel.MetricsSnapshot
}

// NewGateway creates a new metrics gateway backed by a metrics snapshot.
func NewGateway(snapshot *altotel.MetricsSnapshot) *Gateway {
	return &Gateway{snapshot: snapshot}
}

// GetSystemMetrics reads current metric values from the atomic snapshot.
func (g *Gateway) GetSystemMetrics(_ context.Context) (*domain.SystemMetrics, error) {
	if g.snapshot == nil {
		return &domain.SystemMetrics{}, nil
	}

	return &domain.SystemMetrics{
		Projector: domain.ProjectorMetrics{
			EventsProcessed:    g.snapshot.ProjectorEventsProcessed(),
			LagSeconds:         g.snapshot.ProjectorLagSeconds(),
			BatchDurationMsP50: g.snapshot.ProjectorBatchP50(),
			BatchDurationMsP95: g.snapshot.ProjectorBatchP95(),
			BatchDurationMsP99: g.snapshot.ProjectorBatchP99(),
			Errors:             g.snapshot.ProjectorErrors(),
		},
		Handler: domain.HandlerMetrics{
			PagesServed:   g.snapshot.PagesServed(),
			PagesDegraded: g.snapshot.PagesDegraded(),
		},
		Tracking: domain.TrackingMetrics{
			ItemsExposed:   g.snapshot.ItemsExposed(),
			ItemsOpened:    g.snapshot.ItemsOpened(),
			ItemsDismissed: g.snapshot.ItemsDismissed(),
		},
		Stream: domain.StreamMetrics{
			ConnectionsTotal: g.snapshot.StreamConnections(),
			DisconnectsTotal: g.snapshot.StreamDisconnects(),
			ReconnectsTotal:  g.snapshot.StreamReconnects(),
			DeliveriesTotal:  g.snapshot.StreamDeliveries(),
		},
		Correctness: domain.CorrectnessMetrics{
			EmptyResponses:    g.snapshot.EmptyResponses(),
			MalformedWhy:      g.snapshot.MalformedWhy(),
			OrphanItems:       g.snapshot.OrphanItems(),
			SupersedeMismatch: g.snapshot.SupersedeMismatch(),
			RequestsTotal:     g.snapshot.RequestsTotal(),
		},
		Sovereign: domain.SovereignMetrics{
			MutationsApplied:      g.snapshot.SovereignApplied(),
			MutationsErrors:       g.snapshot.SovereignErrors(),
			MutationDurationMsP50: g.snapshot.SovereignDurationP50(),
			MutationDurationMsP95: g.snapshot.SovereignDurationP95(),
		},
		Recall: domain.RecallMetrics{
			SignalsAppended:        g.snapshot.RecallSignals(),
			SignalErrors:           g.snapshot.RecallSignalErrors(),
			CandidatesGenerated:   g.snapshot.RecallCandidates(),
			CandidatesEmpty:       g.snapshot.RecallCandidatesEmpty(),
			UsersProcessed:        g.snapshot.RecallUsersProcessed(),
			ProjectorDurationMsP50: g.snapshot.RecallProjectorDurationP50(),
			ProjectorDurationMsP95: g.snapshot.RecallProjectorDurationP95(),
		},
	}, nil
}
