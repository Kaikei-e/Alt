package bootstrap

import (
	"context"
	"time"

	"search-indexer/logger"
)

// taskPruner narrows SearchEngine to the single method the prune loop needs,
// mirroring warmupSearcher's rationale: keep the unit test independent of the
// full port.SearchEngine surface.
type taskPruner interface {
	PruneTaskHistory(ctx context.Context, olderThan time.Duration) error
}

// runTaskPruneLoop periodically deletes finished Meilisearch tasks older than
// retention. This is the durable fix for the 2026-07-22 incident:
// registerBatchSynonyms's process-wide synonyms union grows without bound as
// new tags arrive, and every full-replace PUT is retained forever as a
// settingsUpdate task's "details" payload. Meilisearch's own automatic
// cleanup only triggers once total stored tasks reach 1M -- it never fires
// here because a few thousand large-payload tasks exhaust the ~10GiB task
// database's byte budget long before the count threshold is reached. Without
// an explicit prune, the task database eventually fills, Meilisearch rejects
// all writes (no_space_left_on_device), and indexing silently stops while the
// service stays healthy.
//
// Pruning runs once immediately (an operator restarting the service to
// recover from exactly this incident should not wait a full interval for
// relief) and then on every tick until ctx is cancelled. Failures are
// Warn-logged and non-fatal: a transient Meilisearch outage must not crash
// the indexer, and the next tick will retry.
func runTaskPruneLoop(ctx context.Context, engine taskPruner, interval, retention time.Duration) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		start := time.Now()
		if err := engine.PruneTaskHistory(ctx, retention); err != nil {
			logger.Logger.WarnContext(ctx, "task history prune failed",
				"err", err,
				"retention", retention,
				"elapsed_ms", time.Since(start).Milliseconds(),
			)
		} else {
			logger.Logger.InfoContext(ctx, "task history prune ok",
				"retention", retention,
				"elapsed_ms", time.Since(start).Milliseconds(),
			)
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(interval):
		}
	}
}
