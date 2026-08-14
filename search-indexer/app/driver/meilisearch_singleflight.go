// Package driver: meilisearch_singleflight.go coalesces concurrent identical
// cache-miss searches into a single underlying Meilisearch call. Production
// traces (2026-05-24 03:31) showed RAG sending 7 identical SearchArticles in
// under 300ms — without dedupe each pays the full hybrid-search cost.
//
// Composes with the LRU cache (meilisearch_cache.go): the cache absorbs
// repeats *across time*, singleflight absorbs repeats *across concurrent
// goroutines* before the first call has populated the cache.
package driver

import (
	"context"
	"errors"

	"github.com/meilisearch/meilisearch-go"

	"search-indexer/logger"
)

// maxSharedSearchFailovers bounds how many times one caller may re-enter the
// group after a shared call died on somebody else's cancellation. A failover
// only happens when another caller hung up, so the loop is already gated on
// external churn; the cap keeps a pathological burst of short-lived callers
// from making a patient caller retry forever.
const maxSharedSearchFailovers = 2

// singleflightSearch coalesces concurrent calls keyed by `key` into a single
// invocation of `fn`. Other callers wait on the in-flight channel and read
// the shared result.
//
// The shared invocation necessarily runs under whichever caller happened to
// lead the flight, because `fn` closes over that caller's context at the call
// site. Cancelling the leader therefore kills the search everybody else is
// waiting on — exactly in the fan-out that singleflight exists for. So a
// waiter that is still healthy treats a cancellation it cannot own as *not its
// error* and re-enters the group, which elects a new leader with a live
// context. Non-context failures are still returned to every waiter unchanged:
// re-running those would amplify a broken engine N-fold.
//
// The singleflight.Group lives on the driver struct (declared in
// meilisearch_driver.go) so its lifecycle matches the driver itself.
func (d *MeilisearchDriver) singleflightSearch(ctx context.Context, key string, fn func() (cacheEntry, error)) (cacheEntry, error) {
	for attempt := 0; ; attempt++ {
		ch := d.sf.DoChan(key, func() (any, error) {
			return fn()
		})
		select {
		case <-ctx.Done():
			return cacheEntry{}, ctx.Err()
		case res := <-ch:
			if res.Err != nil {
				if attempt < maxSharedSearchFailovers && ctx.Err() == nil && isContextFailure(res.Err) {
					// singleflight drops a finished key before it delivers
					// results (doCall deletes under the same lock), so the
					// survivors coalesce onto one fresh flight instead of
					// stampeding Meilisearch.
					logger.Logger.DebugContext(ctx, "meilisearch singleflight failover",
						"reason", "shared call cancelled by another caller",
						"attempt", attempt+1,
						"err", res.Err,
					)
					continue
				}
				return cacheEntry{}, res.Err
			}
			if e, ok := res.Val.(cacheEntry); ok {
				return e, nil
			}
			return cacheEntry{}, nil
		}
	}
}

// isContextFailure reports whether err ultimately came from a context
// cancellation or deadline. errors.Is alone is not enough here: meilisearch-go
// v0.36.3's *meilisearch.Error has no Unwrap — it parks the transport failure
// in OriginError instead — so a genuinely cancelled search would otherwise be
// indistinguishable from an engine error and get fanned out to every waiter.
func isContextFailure(err error) bool {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var apiErr *meilisearch.Error
	if errors.As(err, &apiErr) {
		return errors.Is(apiErr.OriginError, context.Canceled) ||
			errors.Is(apiErr.OriginError, context.DeadlineExceeded)
	}
	return false
}
