// Package driver: meilisearch_singleflight.go coalesces concurrent identical
// cache-miss searches into a single underlying Meilisearch call. Production
// traces (2026-05-24 03:31) showed RAG sending 7 identical SearchArticles in
// under 300ms — without dedupe each pays the full hybrid-search cost.
//
// Composes with the LRU cache (meilisearch_cache.go): the cache absorbs
// repeats *across time*, singleflight absorbs repeats *across concurrent
// goroutines* before the first call has populated the cache.
package driver

import "context"

// singleflightSearch coalesces concurrent calls keyed by `key` into a single
// invocation of `fn`. Other callers wait on the in-flight channel and read
// the shared result. Per-caller context cancellation is honoured without
// aborting the underlying fn — that is what keeps later waiters honest when
// an early caller bails out.
//
// The singleflight.Group lives on the driver struct (declared in
// meilisearch_driver.go) so its lifecycle matches the driver itself.
func (d *MeilisearchDriver) singleflightSearch(ctx context.Context, key string, fn func() (cacheEntry, error)) (cacheEntry, error) {
	ch := d.sf.DoChan(key, func() (any, error) {
		return fn()
	})
	select {
	case <-ctx.Done():
		return cacheEntry{}, ctx.Err()
	case res := <-ch:
		if res.Err != nil {
			return cacheEntry{}, res.Err
		}
		if e, ok := res.Val.(cacheEntry); ok {
			return e, nil
		}
		return cacheEntry{}, nil
	}
}
