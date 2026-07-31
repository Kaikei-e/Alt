package datahub

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"connectrpc.com/connect"
)

// legacyNamespaceCalledMsg is the per-request deprecation line. Named so tests
// and log queries agree on it.
const legacyNamespaceCalledMsg = "legacy_namespace.called"

// legacyNamespaceLogInterval is how long a procedure stays quiet after being
// reported once.
//
// Five minutes is chosen against the noisiest caller rather than as a round
// number: search-indexer's incremental loop and pre-processor's summarisation
// poll both call BackendInternalService continuously, so a per-request line
// would be the majority of everything alt-data-hub logs. An operator who
// responds by filtering the message out has also filtered out the signal that
// says which peers Wave 2-B still has to move — which is the only thing this
// line is for.
const legacyNamespaceLogInterval = 5 * time.Minute

// legacyNamespaceNotice reports calls that arrive on
// services.backend.v1.BackendInternalService while alt-data-hub serves both
// namespaces (ADR-000954 D7).
//
// It is a migration instrument, not a guard: the legacy namespace is fully
// supported until Wave 2-C removes it, and nothing here rejects, delays or
// alters a call. What it produces is the evidence Wave 2-C needs — which
// procedures still see traffic, and roughly how much — so that "no peer is
// left on the old path" is something an operator can read off the logs rather
// than infer from the PR list.
type legacyNamespaceNotice struct {
	logger   *slog.Logger
	interval time.Duration
	now      func() time.Time

	mu     sync.Mutex
	nextAt map[string]time.Time
	// since counts calls made after the last line was written, so the line
	// carries the volume it stands for instead of implying a single call.
	since map[string]int64
}

func newLegacyNamespaceNotice(logger *slog.Logger, interval time.Duration, now func() time.Time) *legacyNamespaceNotice {
	if logger == nil {
		logger = slog.Default()
	}
	if now == nil {
		now = time.Now
	}
	return &legacyNamespaceNotice{
		logger:   logger,
		interval: interval,
		now:      now,
		nextAt:   make(map[string]time.Time),
		since:    make(map[string]int64),
	}
}

// observe records one call and reports how many calls the returned line stands
// for, or 0 when this call is inside a quiet window.
func (n *legacyNamespaceNotice) observe(procedure string) int64 {
	n.mu.Lock()
	defer n.mu.Unlock()

	n.since[procedure]++

	now := n.now()
	if next, seen := n.nextAt[procedure]; seen && now.Before(next) {
		return 0
	}

	count := n.since[procedure]
	n.since[procedure] = 0
	n.nextAt[procedure] = now.Add(n.interval)
	return count
}

func (n *legacyNamespaceNotice) interceptor() connect.UnaryInterceptorFunc {
	return func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			procedure := req.Spec().Procedure
			if count := n.observe(procedure); count > 0 {
				n.logger.WarnContext(ctx, legacyNamespaceCalledMsg,
					"procedure", procedure,
					"deprecated_service", "services.backend.v1.BackendInternalService",
					"replacement", "alt.datahub.v1.DataHubService",
					"calls_since_last_log", count,
					"log_interval", n.interval.String(),
					"adr", "ADR-000954",
					"removed_in", "Wave 2-C",
				)
			}
			return next(ctx, req)
		}
	}
}
