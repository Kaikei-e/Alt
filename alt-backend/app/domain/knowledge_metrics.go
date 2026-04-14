package domain

import "time"

// SystemMetrics holds aggregated system metric snapshots for the admin dashboard.
type SystemMetrics struct {
	Projector     ProjectorMetrics
	Handler       HandlerMetrics
	Tracking      TrackingMetrics
	Stream        StreamMetrics
	Correctness   CorrectnessMetrics
	Sovereign     SovereignMetrics
	Recall        RecallMetrics
	ServiceHealth []ServiceHealthStatus
}

// ProjectorMetrics captures event processing pipeline health.
type ProjectorMetrics struct {
	EventsProcessed    int64
	LagSeconds         float64
	BatchDurationMsP50 float64
	BatchDurationMsP95 float64
	BatchDurationMsP99 float64
	Errors             int64
}

// HandlerMetrics captures Knowledge Home page serving health.
type HandlerMetrics struct {
	PagesServed     int64
	PagesDegraded   int64
	DegradedRatePct float64
}

// TrackingMetrics captures user interaction tracking health.
type TrackingMetrics struct {
	ItemsExposed   int64
	ItemsOpened    int64
	ItemsDismissed int64
	OpenRatePct    float64
	DismissRatePct float64
}

// StreamMetrics captures SSE stream connection health.
type StreamMetrics struct {
	ConnectionsTotal  int64
	DisconnectsTotal  int64
	ReconnectsTotal   int64
	DeliveriesTotal   int64
	DisconnectRatePct float64
}

// CorrectnessMetrics captures data quality signals.
type CorrectnessMetrics struct {
	EmptyResponses      int64
	MalformedWhy        int64
	OrphanItems         int64
	SupersedeMismatch   int64
	RequestsTotal       int64
	CorrectnessScorePct float64
}

// SovereignMetrics captures knowledge-sovereign mutation health.
type SovereignMetrics struct {
	MutationsApplied      int64
	MutationsErrors       int64
	MutationDurationMsP50 float64
	MutationDurationMsP95 float64
	ErrorRatePct          float64
}

// RecallMetrics captures recall pipeline health.
type RecallMetrics struct {
	SignalsAppended        int64
	SignalErrors           int64
	CandidatesGenerated    int64
	CandidatesEmpty        int64
	UsersProcessed         int64
	ProjectorDurationMsP50 float64
	ProjectorDurationMsP95 float64
}

// ServiceHealthStatus represents the health of a downstream service.
type ServiceHealthStatus struct {
	ServiceName  string
	Endpoint     string
	Status       string // "healthy", "unhealthy", "unknown"
	LatencyMs    int64
	CheckedAt    time.Time
	ErrorMessage string
}

// Service health status constants.
const (
	ServiceHealthy   = "healthy"
	ServiceUnhealthy = "unhealthy"
	ServiceUnknown   = "unknown"
)
