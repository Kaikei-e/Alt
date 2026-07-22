package bootstrap

import (
	"context"
	"time"

	"search-indexer/logger"
)

// synonymsFlusher narrows the indexing usecase to the single method the
// flush loop needs, mirroring taskPruner's and warmupSearcher's rationale:
// keep the unit test independent of the full usecase surface.
type synonymsFlusher interface {
	FlushSynonyms(ctx context.Context) error
}

// runSynonymsFlushLoop periodically turns an accumulated, dirty synonyms
// union into a single Meilisearch PUT. This is the durable fix for PM-2026-047
// action item #2: Meilisearch's synonyms setting has no incremental/patch
// update, only a full-replace PUT, and it retains every settingsUpdate
// task's full payload in its task history indefinitely. Calling
// RegisterSynonyms once per indexed batch (the pre-2026-07-22 behavior)
// generated one such task per batch with an ever-growing payload, filling the
// task database and locking out all writes. Decoupling the PUT from indexing
// throughput onto this fixed interval bounds how often that task is created
// regardless of how many batches run in between.
//
// Flushing runs once immediately (an operator restarting the service should
// not wait a full interval for pending synonyms to reach Meilisearch) and
// then on every tick until ctx is cancelled. Failures are Warn-logged and
// non-fatal: a transient Meilisearch outage must not crash the indexer, and
// the next tick retries with whatever is dirty by then.
func runSynonymsFlushLoop(ctx context.Context, flusher synonymsFlusher, interval time.Duration) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		if err := flusher.FlushSynonyms(ctx); err != nil {
			logger.Logger.WarnContext(ctx, "synonyms flush failed", "err", err)
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(interval):
		}
	}
}
