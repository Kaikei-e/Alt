package job

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"alt/di"
	"alt/orchestrator/usecase/push_dispatch_usecase"
)

const (
	// dispatchInterval is how often the dispatcher looks for work. Someone who
	// just watched a job finish is still looking at the screen, so seconds
	// matter here in a way they do not for the hourly jobs.
	dispatchInterval = 2 * time.Second
	dispatchTimeout  = 30 * time.Second
	// dispatchBatchSize bounds one pass. Sends run sequentially inside the
	// pass, so this also bounds how long the claim lease has to cover.
	dispatchBatchSize = 50
)

// These instruments exist because the failure this feature has to survive is
// silence, not errors. Web Push has no delivery receipt — a 201 means a push
// service accepted the message and nothing more — so every alert in
// observability/prometheus/rules/push-delivery-alerts.yml watches for the
// absence of success rather than the presence of failure, and these are what it
// watches.
var (
	dispatchMeterOnce   sync.Once
	pushSendCounter     metric.Int64Counter
	pushDeadCounter     metric.Int64Counter
	pushClaimFailures   metric.Int64Counter
	pushLastTickGauge   metric.Float64Gauge
	pushBacklogAgeGauge metric.Float64Gauge
	pushPendingGauge    metric.Float64Gauge
)

func initDispatchMetrics() {
	dispatchMeterOnce.Do(func() {
		meter := otel.Meter("alt-notifier.push-dispatcher")
		pushSendCounter, _ = meter.Int64Counter("push_send_total",
			metric.WithDescription("Web Push delivery attempts, labeled by SLI bucket and HTTP status. The 401/403 series is the one that pages: a VAPID JWT signed once at startup works for a day and then rejects every send with no crash and no error rate"))
		pushDeadCounter, _ = meter.Int64Counter("push_delivery_dead_total",
			metric.WithDescription("Deliveries that reached the dead-letter state"))
		pushClaimFailures, _ = meter.Int64Counter("push_dispatcher_claim_failures_total",
			metric.WithDescription("Passes where claiming from the data plane failed. Without this, an unreachable data plane is indistinguishable from an empty queue"))
		pushLastTickGauge, _ = meter.Float64Gauge("push_dispatcher_last_tick_timestamp_seconds",
			metric.WithDescription("Wall-clock time of the last dispatcher pass, set on every pass including ones that claim nothing"))
		pushBacklogAgeGauge, _ = meter.Float64Gauge("push_delivery_oldest_pending_age_seconds",
			metric.WithDescription("Age of the oldest non-terminal push delivery, counting rows in 'sending' as well as 'pending' so a row orphaned by a crashed dispatcher is not invisible"))
		pushPendingGauge, _ = meter.Float64Gauge("push_delivery_pending_count",
			metric.WithDescription("Non-terminal push deliveries. Read alongside the age gauge: a rising count with a flat age means the producers sped up, a flat count with a rising age means nothing is draining"))
	})
}

// RegisterNotifierJobs wires cmd/notifier's single job.
//
// It lives here rather than in di because orchestrator/job already imports di,
// and registering from the composition root would close an import cycle. main
// calls this, exactly as cmd/harvester does.
func RegisterNotifierJobs(scheduler *JobScheduler, container *di.NotifierComponents) {
	if container == nil || container.Dispatch == nil {
		// Rule 8. A notifier with no dispatcher would pass its health check and
		// deliver nothing, which reads identically to "nobody has enabled
		// notifications yet".
		panic("notifier: dispatch usecase is nil — refusing to start a process whose only job is unwired (see .claude/rules/di-wiring.md)")
	}

	initDispatchMetrics()

	scheduler.Add(Job{
		Name:     "push-dispatcher",
		Interval: dispatchInterval,
		Timeout:  dispatchTimeout,
		Fn: func(ctx context.Context) error {
			// Published before the batch, and regardless of what the batch does.
			// The queue-age gauge has to be written on a pass that claims
			// nothing — that is exactly the shape of a backlog nobody is
			// draining — and a gauge that stops being written keeps serving its
			// last value for as long as the target is up.
			publishBacklogAge(ctx, container.Dispatch)

			stats, err := container.Dispatch.DispatchBatch(ctx, dispatchBatchSize)
			recordDispatchPass(ctx, stats, err)
			return err
		},
	})
}

// recordDispatchPass publishes one pass's outcome.
//
// The tick gauge is set unconditionally, before anything else and including on
// the failure path. A gauge that is only written when there is something to
// report keeps serving its last value for as long as the target is up, so a
// wedged dispatcher would read healthy forever — which is the exact detector
// failure the alert file warns about.
func recordDispatchPass(ctx context.Context, stats push_dispatch_usecase.Stats, err error) {
	pushLastTickGauge.Record(ctx, float64(time.Now().Unix()))

	if err != nil {
		pushClaimFailures.Add(ctx, 1)
		slog.ErrorContext(ctx, "push_dispatch_claim_failed", "error", err)
		return
	}

	for _, attempt := range stats.Attempts {
		pushSendCounter.Add(ctx, 1, metric.WithAttributes(
			attribute.String("result", attempt.Result),
			attribute.Int("status_code", attempt.StatusCode),
		))
	}
	if stats.Dead > 0 {
		pushDeadCounter.Add(ctx, int64(stats.Dead))
	}

	if stats.Claimed > 0 {
		slog.InfoContext(ctx, "push_dispatch_pass",
			"claimed", stats.Claimed,
			"sent", stats.Sent,
			"released", stats.Released,
			"dead", stats.Dead,
			"gone", stats.Gone,
		)
	}
}

// publishBacklogAge asks the data plane how stale the queue is and records it.
//
// A read failure is logged and leaves the gauge unwritten rather than recording
// a zero: a fabricated zero would say "the queue is fresh" at the moment we
// stopped being able to tell, which is the one answer that must never be
// guessed. The staleness that follows is caught by the tick alert instead.
func publishBacklogAge(ctx context.Context, dispatch *push_dispatch_usecase.Usecase) {
	age, pending, err := dispatch.BacklogAge(ctx)
	if err != nil {
		slog.WarnContext(ctx, "push_backlog_age_unavailable", "error", err)
		return
	}
	pushBacklogAgeGauge.Record(ctx, age.Seconds())
	pushPendingGauge.Record(ctx, float64(pending))
}
