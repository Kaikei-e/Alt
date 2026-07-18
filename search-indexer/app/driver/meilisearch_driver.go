package driver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/meilisearch/meilisearch-go"
	"golang.org/x/sync/singleflight"

	"search-indexer/config"
	"search-indexer/logger"
	appotel "search-indexer/utils/otel"
)

// meilisearchTaskPollInterval and meilisearchTaskWaitTimeout bound every
// WaitForTaskWithContext call made through the waitForTask helper below.
//
// meilisearch-go's WaitForTask(WithContext)'s second argument is a POLLING
// INTERVAL, not a timeout -- confirmed by reading the vendored v0.36.2
// source (meilisearch.go's waitForTask loops on a ticker set to that
// interval and only returns early via ctx.Done(); it never times out on
// its own). The old code called WaitForTask(taskUID, 15*time.Second) with
// a "// 15 seconds timeout" comment that was simply wrong: it polled every
// 15s and, with context.Background() baked in by the non-context variant,
// waited forever if a task never completed. waitForTask fixes this by
// wrapping every call in an explicit context.WithTimeout and using a much
// shorter poll interval so completion is detected promptly.
const (
	meilisearchTaskPollInterval = 500 * time.Millisecond
	meilisearchTaskWaitTimeout  = 15 * time.Second
)

// MeilisearchDriver isolates admin operations (IndexDocuments, Delete,
// EnsureIndex, RegisterSynonyms) from search-only operations. If the operator
// provisions a dedicated Search API key, L-001 lets us use it only for the
// read path while admin writes keep the higher-privilege key.
type MeilisearchDriver struct {
	client      meilisearch.ServiceManager
	index       meilisearch.IndexManager
	searchIndex meilisearch.IndexManager
	indexName   string
	hybrid      *HybridConfig
	cache       *searchCache
	sf          singleflight.Group

	// taskWaitTimeout/taskPollInterval back waitForTask. Exposed as fields
	// (rather than the package constants directly) so tests can shrink the
	// timeout instead of waiting out the full 15s default.
	taskWaitTimeout  time.Duration
	taskPollInterval time.Duration
}

// NewMeilisearchDriver constructs a driver where the same client handles both
// reads and writes. Used when only a single master/admin key is configured.
func NewMeilisearchDriver(client meilisearch.ServiceManager, indexName string) *MeilisearchDriver {
	idx := client.Index(indexName)
	return &MeilisearchDriver{
		client:           client,
		index:            idx,
		searchIndex:      idx,
		indexName:        indexName,
		taskWaitTimeout:  meilisearchTaskWaitTimeout,
		taskPollInterval: meilisearchTaskPollInterval,
	}
}

// NewMeilisearchDriverWithClients splits the read and write paths across two
// clients. Pass nil for searchClient to fall back to the admin client.
func NewMeilisearchDriverWithClients(adminClient meilisearch.ServiceManager, searchClient meilisearch.ServiceManager, indexName string) *MeilisearchDriver {
	d := &MeilisearchDriver{
		client:           adminClient,
		index:            adminClient.Index(indexName),
		searchIndex:      adminClient.Index(indexName),
		indexName:        indexName,
		taskWaitTimeout:  meilisearchTaskWaitTimeout,
		taskPollInterval: meilisearchTaskPollInterval,
	}
	if searchClient != nil {
		d.searchIndex = searchClient.Index(indexName)
	}
	return d
}

// waitForTask polls until a Meilisearch task completes or a bounded timeout
// elapses. See the package-level comment on meilisearchTaskWaitTimeout for
// why this can't just call WaitForTask(taskUID, timeout) directly.
func (d *MeilisearchDriver) waitForTask(ctx context.Context, taskUID int64) (*meilisearch.Task, error) {
	waitCtx, cancel := context.WithTimeout(ctx, d.taskWaitTimeout)
	defer cancel()
	return d.index.WaitForTaskWithContext(waitCtx, taskUID, d.taskPollInterval)
}

// WithHybrid installs a hybrid-search configuration on the driver. Pass nil
// or a HybridConfig with an empty Embedder to disable hybrid mode.
func (d *MeilisearchDriver) WithHybrid(cfg *HybridConfig) *MeilisearchDriver {
	d.hybrid = cfg
	return d
}

// WithCache installs an in-memory LRU cache in front of the search path.
// size=0 disables the cache (leaves it nil). The driver itself remains
// safe to use without a cache; the search methods fall back to direct
// Meilisearch calls when the cache is nil.
func (d *MeilisearchDriver) WithCache(size int, ttl time.Duration) *MeilisearchDriver {
	if size <= 0 {
		d.cache = nil
		return d
	}
	c, err := newSearchCache(size, ttl)
	if err != nil {
		logger.Logger.Warn("failed to construct search cache, continuing without it", "err", err)
		return d
	}
	d.cache = c
	return d
}

// hybridSnapshot returns the embedder name and ratio currently configured on
// the driver. Used as part of the cache key so an env-driven config change
// invalidates cached results.
func (d *MeilisearchDriver) hybridSnapshot() (string, float64) {
	if d.hybrid == nil {
		return "", 0
	}
	return d.hybrid.Embedder, d.hybrid.SemanticRatio
}

// newBaseSearchRequest centralises SearchRequest construction so hybrid
// plumbing stays consistent across Search, SearchWithFilters, and the
// user-scoped search variants.
//
// AttributesToRetrieve excludes the full content field — the driver no
// longer needs the raw body once Meilisearch is asked to crop it.
// AttributesToCrop + CropLength make Meilisearch produce a bounded snippet
// in hit["_formatted"]["content"]; the driver reads that via getCropped and
// places it back into SearchDocumentDriver.Content so downstream layers see
// no shape change. Net effect: ~7.5 KB → ~few hundred bytes per hit on the
// wire and in the LRU cache.
func (d *MeilisearchDriver) newBaseSearchRequest(query string, limit int) *meilisearch.SearchRequest {
	return &meilisearch.SearchRequest{
		Query:                query,
		Limit:                int64(limit),
		ShowRankingScore:     true,
		Hybrid:               d.hybrid.toSDK(),
		AttributesToRetrieve: []string{"id", "title", "tags", "user_id", "language", "published_at"},
		AttributesToCrop:     []string{"content"},
		CropLength:           120,
	}
}

// recordProcessing surfaces Meilisearch's self-reported processingTimeMs as a
// span attribute + debug log line. Hybrid search runs the embedder inside
// Meilisearch, so this value is the cleanest signal for separating embedder
// cold-start latency from BM25 search cost without invasive driver signature
// changes.
func (d *MeilisearchDriver) recordProcessing(ctx context.Context, op string, result *meilisearch.SearchResponse) {
	if result == nil {
		return
	}
	appotel.RecordMeilisearchProcessing(ctx, op, result.ProcessingTimeMs)
	logger.Logger.DebugContext(ctx, "meilisearch search",
		"op", op,
		"processing_ms", result.ProcessingTimeMs,
		"hits", len(result.Hits),
		"estimated_total", result.EstimatedTotalHits,
	)
}

func (d *MeilisearchDriver) IndexDocuments(ctx context.Context, docs []SearchDocumentDriver) error {
	if len(docs) == 0 {
		return nil
	}

	task, err := d.index.AddDocumentsWithContext(ctx, docs, nil)
	if err != nil {
		return &DriverError{
			Op:  "IndexDocuments",
			Err: err,
		}
	}

	// Wait for the indexing task to complete
	if _, err := d.waitForTask(ctx, task.TaskUID); err != nil {
		return &DriverError{
			Op:  "IndexDocuments",
			Err: fmt.Errorf("failed to wait for indexing task: %w", err),
		}
	}

	return nil
}

func (d *MeilisearchDriver) DeleteDocuments(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}

	task, err := d.index.DeleteDocumentsWithContext(ctx, ids, nil)
	if err != nil {
		return &DriverError{
			Op:  "DeleteDocuments",
			Err: err,
		}
	}

	// Wait for the deletion task to complete
	if _, err := d.waitForTask(ctx, task.TaskUID); err != nil {
		return &DriverError{
			Op:  "DeleteDocuments",
			Err: fmt.Errorf("failed to wait for deletion task: %w", err),
		}
	}

	return nil
}

func (d *MeilisearchDriver) Search(ctx context.Context, query string, limit int) ([]SearchDocumentDriver, error) {
	emb, ratio := d.hybridSnapshot()
	key := cacheKey{
		Query:         normalizeCacheKeyQuery(query),
		Limit:         int64(limit),
		Embedder:      emb,
		SemanticRatio: ratio,
	}
	if e, ok := d.cache.get(key); ok {
		appotel.RecordMeilisearchProcessing(ctx, "Search.cacheHit", e.ProcessingMs)
		return e.Docs, nil
	}

	entry, err := d.singleflightSearch(ctx, key.String(), func() (cacheEntry, error) {
		searchRequest := d.newBaseSearchRequest(query, limit)
		// Locales intentionally omitted: let Meilisearch match across all configured
		// locales (jpn + eng). Previously CJK queries were restricted to jpn-only,
		// which prevented Japanese queries from matching English article content
		// (e.g., "ヴァンス副大統領" could not find "JD Vance" articles).
		result, err := d.searchIndex.SearchWithContext(ctx, query, searchRequest)
		if err != nil {
			return cacheEntry{}, err
		}
		d.recordProcessing(ctx, "Search", result)
		docs := d.hitsToDocs(result.Hits)
		e := cacheEntry{Docs: docs, ProcessingMs: result.ProcessingTimeMs}
		d.cache.put(key, e)
		return e, nil
	})
	if err != nil {
		return nil, &DriverError{Op: "Search", Err: err}
	}
	return entry.Docs, nil
}

func (d *MeilisearchDriver) SearchWithFilters(ctx context.Context, query string, filters []string, limit int) ([]SearchDocumentDriver, error) {
	filter := d.buildSecureFilter(filters)

	emb, ratio := d.hybridSnapshot()
	key := cacheKey{
		Query:         normalizeCacheKeyQuery(query),
		Filter:        filter,
		Limit:         int64(limit),
		Embedder:      emb,
		SemanticRatio: ratio,
	}
	if e, ok := d.cache.get(key); ok {
		appotel.RecordMeilisearchProcessing(ctx, "SearchWithFilters.cacheHit", e.ProcessingMs)
		return e.Docs, nil
	}

	entry, err := d.singleflightSearch(ctx, key.String(), func() (cacheEntry, error) {
		searchRequest := d.newBaseSearchRequest(query, limit)
		if filter != "" {
			searchRequest.Filter = filter
		}
		result, err := d.searchIndex.SearchWithContext(ctx, query, searchRequest)
		if err != nil {
			return cacheEntry{}, err
		}
		d.recordProcessing(ctx, "SearchWithFilters", result)
		docs := d.hitsToDocs(result.Hits)
		e := cacheEntry{Docs: docs, ProcessingMs: result.ProcessingTimeMs}
		d.cache.put(key, e)
		return e, nil
	})
	if err != nil {
		return nil, &DriverError{Op: "SearchWithFilters", Err: err}
	}
	return entry.Docs, nil
}

// SearchWithDateFilter restricts results to documents whose “published_at“
// (Unix seconds) is inside the requested window. Either bound may be nil.
// When both are nil this degrades to a plain Search.
func (d *MeilisearchDriver) SearchWithDateFilter(ctx context.Context, query string, publishedAfter, publishedBefore *time.Time, limit int) ([]SearchDocumentDriver, error) {
	if publishedAfter == nil && publishedBefore == nil {
		return d.Search(ctx, query, limit)
	}

	filterClauses := make([]string, 0, 2)
	if publishedAfter != nil {
		filterClauses = append(filterClauses, "published_at >= "+strconv.FormatInt(publishedAfter.Unix(), 10))
	}
	if publishedBefore != nil {
		filterClauses = append(filterClauses, "published_at <= "+strconv.FormatInt(publishedBefore.Unix(), 10))
	}
	filter := strings.Join(filterClauses, " AND ")

	emb, ratio := d.hybridSnapshot()
	key := cacheKey{
		Query:         normalizeCacheKeyQuery(query),
		Filter:        filter,
		Limit:         int64(limit),
		Embedder:      emb,
		SemanticRatio: ratio,
	}
	if e, ok := d.cache.get(key); ok {
		appotel.RecordMeilisearchProcessing(ctx, "SearchWithDateFilter.cacheHit", e.ProcessingMs)
		return e.Docs, nil
	}

	entry, err := d.singleflightSearch(ctx, key.String(), func() (cacheEntry, error) {
		searchRequest := d.newBaseSearchRequest(query, limit)
		searchRequest.Filter = filter
		result, err := d.searchIndex.SearchWithContext(ctx, query, searchRequest)
		if err != nil {
			return cacheEntry{}, err
		}
		d.recordProcessing(ctx, "SearchWithDateFilter", result)
		docs := d.hitsToDocs(result.Hits)
		e := cacheEntry{Docs: docs, ProcessingMs: result.ProcessingTimeMs}
		d.cache.put(key, e)
		return e, nil
	})
	if err != nil {
		return nil, &DriverError{Op: "SearchWithDateFilter", Err: err}
	}
	return entry.Docs, nil
}

// hitsToDocs flattens a Meilisearch result slice into SearchDocumentDriver
// values, preserving the language and published_at attributes that used to
// be dropped silently at this boundary. Content is sourced from the cropped
// variant (Meilisearch _formatted.content) so the driver never carries the
// full body.
func (d *MeilisearchDriver) hitsToDocs(hits []meilisearch.Hit) []SearchDocumentDriver {
	docs := make([]SearchDocumentDriver, 0, len(hits))
	for _, hit := range hits {
		docs = append(docs, SearchDocumentDriver{
			ID:          d.getString(hit, "id"),
			Title:       d.getString(hit, "title"),
			Content:     d.getCropped(hit, "content"),
			Tags:        d.getStringSlice(hit, "tags"),
			UserID:      d.getString(hit, "user_id"),
			Language:    d.getString(hit, "language"),
			Score:       d.getFloat64(hit, "_rankingScore"),
			PublishedAt: d.getInt64(hit, "published_at"),
		})
	}
	return docs
}

func (d *MeilisearchDriver) EnsureIndex(ctx context.Context) error {
	_, err := d.index.FetchInfoWithContext(ctx)
	if err != nil {
		if !isIndexNotFoundErr(err) {
			return &DriverError{
				Op:  "EnsureIndex",
				Err: fmt.Errorf("fetch index info: %w", err),
			}
		}
		task, createErr := d.client.CreateIndexWithContext(ctx, &meilisearch.IndexConfig{
			Uid:        d.indexName,
			PrimaryKey: "id",
		})
		if createErr != nil {
			return &DriverError{
				Op:  "EnsureIndex",
				Err: fmt.Errorf("failed to create index: %w", createErr),
			}
		}
		if _, waitErr := d.waitForTask(ctx, task.TaskUID); waitErr != nil {
			return &DriverError{
				Op:  "EnsureIndex",
				Err: fmt.Errorf("failed to wait for index creation: %w", waitErr),
			}
		}
	}

	// Configure index settings (best practice: set before indexing) and wait
	// for each async task so subsequent indexing sees the applied settings.

	searchableAttrs := []string{"title", "content", "tags"}
	searchableTask, err := d.index.UpdateSearchableAttributesWithContext(ctx, &searchableAttrs)
	if err != nil {
		return &DriverError{
			Op:  "EnsureIndex",
			Err: fmt.Errorf("failed to set searchable attributes: %w", err),
		}
	}
	if _, err := d.waitForTask(ctx, searchableTask.TaskUID); err != nil {
		return &DriverError{
			Op:  "EnsureIndex",
			Err: fmt.Errorf("failed to wait for searchable attributes update: %w", err),
		}
	}

	// Set filterable attributes for tags / user_id plus the new date and
	// language filters. ``published_at`` is stored as Unix seconds so it
	// supports ``published_at >= X AND published_at <= Y`` windows;
	// ``language`` pairs with the acolyte language_quota rebalancing so
	// cross-lingual recall can be scoped when the caller opts in.
	filterableAttrs := []interface{}{"tags", "user_id", "published_at", "language"}
	filterableTask, err := d.index.UpdateFilterableAttributesWithContext(ctx, &filterableAttrs)
	if err != nil {
		return &DriverError{
			Op:  "EnsureIndex",
			Err: fmt.Errorf("failed to set filterable attributes: %w", err),
		}
	}
	if _, err := d.waitForTask(ctx, filterableTask.TaskUID); err != nil {
		return &DriverError{
			Op:  "EnsureIndex",
			Err: fmt.Errorf("failed to wait for filterable attributes update: %w", err),
		}
	}

	rankingRules := []string{
		"words",
		"typo",
		"proximity",
		"attribute",
		"sort",
		"exactness",
	}
	rankingTask, err := d.index.UpdateRankingRulesWithContext(ctx, &rankingRules)
	if err != nil {
		return &DriverError{
			Op:  "EnsureIndex",
			Err: fmt.Errorf("failed to set ranking rules: %w", err),
		}
	}
	if _, err := d.waitForTask(ctx, rankingTask.TaskUID); err != nil {
		return &DriverError{
			Op:  "EnsureIndex",
			Err: fmt.Errorf("failed to wait for ranking rules update: %w", err),
		}
	}

	// Set localized attributes for Japanese content.
	// Without this, Meilisearch uses its default tokenizer which splits CJK text
	// incorrectly, causing 0 results for Japanese BM25 queries.
	// Multiple locale rules allow Meilisearch to auto-detect the content language
	// and apply the correct tokenizer. Articles contain both Japanese and English text.
	localizedAttrs := []*meilisearch.LocalizedAttributes{
		{
			Locales:           []string{"jpn"},
			AttributePatterns: []string{"title", "content", "tags"},
		},
		{
			Locales:           []string{"eng"},
			AttributePatterns: []string{"title", "content", "tags"},
		},
	}
	task, localErr := d.index.UpdateLocalizedAttributesWithContext(ctx, localizedAttrs)
	if localErr != nil {
		return &DriverError{
			Op:  "EnsureIndex",
			Err: fmt.Errorf("failed to set localized attributes: %w", localErr),
		}
	}
	if _, err := d.waitForTask(ctx, task.TaskUID); err != nil {
		return &DriverError{
			Op:  "EnsureIndex",
			Err: fmt.Errorf("failed to wait for localized attributes update: %w", err),
		}
	}

	// Apply searchCutoffMs so Meilisearch returns partial results within a
	// bounded time budget instead of letting a slow hybrid query consume the
	// full Connect-RPC section timeout. Setting 0 means "no cap" — skip the
	// update in that case to leave the engine default in place.
	if config.MeiliSearchCutoffMs > 0 {
		cutoffTask, cutoffErr := d.index.UpdateSearchCutoffMsWithContext(ctx, int64(config.MeiliSearchCutoffMs))
		if cutoffErr != nil {
			return &DriverError{
				Op:  "EnsureIndex",
				Err: fmt.Errorf("failed to set search cutoff ms: %w", cutoffErr),
			}
		}
		if _, err := d.waitForTask(ctx, cutoffTask.TaskUID); err != nil {
			return &DriverError{
				Op:  "EnsureIndex",
				Err: fmt.Errorf("failed to wait for search cutoff update: %w", err),
			}
		}
	}

	return nil
}

// isIndexNotFoundErr reports whether err means the Meilisearch index is absent.
// Network / auth / other API failures must NOT be treated as "missing index".
func isIndexNotFoundErr(err error) bool {
	var merr *meilisearch.Error
	if !errors.As(err, &merr) {
		return false
	}
	if merr.StatusCode == 404 {
		return true
	}
	return merr.MeilisearchApiError.Code == "index_not_found"
}

func (d *MeilisearchDriver) getString(m meilisearch.Hit, key string) string {
	if v, ok := m[key]; ok {
		var s string
		if err := json.Unmarshal(v, &s); err == nil {
			return s
		}
	}
	return ""
}

// getCropped returns the Meilisearch-cropped value of a field, falling back
// to the raw value when no cropped variant is present. Meilisearch puts crop
// output under hit["_formatted"][key] when attributesToCrop is configured.
// Reading the cropped version means the driver never has to ship the full
// content body across the wire, dropping per-hit payload from ~7.5 KB to a
// few hundred bytes.
func (d *MeilisearchDriver) getCropped(m meilisearch.Hit, key string) string {
	if raw, ok := m["_formatted"]; ok {
		var formatted map[string]string
		if err := json.Unmarshal(raw, &formatted); err == nil {
			if v, ok := formatted[key]; ok && v != "" {
				return v
			}
		}
	}
	return d.getString(m, key)
}

func (d *MeilisearchDriver) getStringSlice(m meilisearch.Hit, key string) []string {
	if v, ok := m[key]; ok {
		var slice []string
		if err := json.Unmarshal(v, &slice); err == nil {
			return slice
		}
	}
	return []string{}
}

func (d *MeilisearchDriver) getFloat64(m meilisearch.Hit, key string) float64 {
	if v, ok := m[key]; ok {
		var f float64
		if err := json.Unmarshal(v, &f); err == nil {
			return f
		}
	}
	return 0.0
}

func (d *MeilisearchDriver) getInt64(m meilisearch.Hit, key string) int64 {
	if v, ok := m[key]; ok {
		var n int64
		if err := json.Unmarshal(v, &n); err == nil {
			return n
		}
	}
	return 0
}

func (d *MeilisearchDriver) SearchByUserID(ctx context.Context, query string, userID string, limit int) ([]SearchDocumentDriver, error) {
	filter := BuildUserFilter(userID)

	emb, ratio := d.hybridSnapshot()
	key := cacheKey{
		Query:         normalizeCacheKeyQuery(query),
		UserID:        userID,
		Filter:        filter,
		Limit:         int64(limit),
		Embedder:      emb,
		SemanticRatio: ratio,
	}
	if e, ok := d.cache.get(key); ok {
		appotel.RecordMeilisearchProcessing(ctx, "SearchByUserID.cacheHit", e.ProcessingMs)
		return e.Docs, nil
	}

	entry, err := d.singleflightSearch(ctx, key.String(), func() (cacheEntry, error) {
		req := d.newBaseSearchRequest(query, limit)
		req.Filter = filter
		if containsCJK(query) {
			req.Locales = []string{"jpn"}
		}
		result, err := d.searchIndex.SearchWithContext(ctx, query, req)
		if err != nil {
			return cacheEntry{}, err
		}
		d.recordProcessing(ctx, "SearchByUserID", result)
		docs := d.hitsToDocs(result.Hits)
		e := cacheEntry{Docs: docs, ProcessingMs: result.ProcessingTimeMs}
		d.cache.put(key, e)
		return e, nil
	})
	if err != nil {
		return nil, &DriverError{Op: "SearchByUserID", Err: err}
	}
	return entry.Docs, nil
}

func (d *MeilisearchDriver) SearchByUserIDWithPagination(ctx context.Context, query string, userID string, offset, limit int64) ([]SearchDocumentDriver, int64, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	filter := BuildUserFilter(userID)

	emb, ratio := d.hybridSnapshot()
	key := cacheKey{
		Query:         normalizeCacheKeyQuery(query),
		UserID:        userID,
		Filter:        filter,
		Offset:        offset,
		Limit:         limit,
		Embedder:      emb,
		SemanticRatio: ratio,
	}
	if e, ok := d.cache.get(key); ok {
		appotel.RecordMeilisearchProcessing(ctx, "SearchByUserIDWithPagination.cacheHit", e.ProcessingMs)
		return e.Docs, e.EstimatedTotal, nil
	}

	entry, err := d.singleflightSearch(ctx, key.String(), func() (cacheEntry, error) {
		paginReq := d.newBaseSearchRequest(query, int(limit))
		paginReq.Offset = offset
		paginReq.Filter = filter
		if containsCJK(query) {
			paginReq.Locales = []string{"jpn"}
		}
		result, err := d.searchIndex.SearchWithContext(ctx, query, paginReq)
		if err != nil {
			return cacheEntry{}, err
		}
		d.recordProcessing(ctx, "SearchByUserIDWithPagination", result)
		docs := d.hitsToDocs(result.Hits)
		e := cacheEntry{
			Docs:           docs,
			EstimatedTotal: result.EstimatedTotalHits,
			ProcessingMs:   result.ProcessingTimeMs,
		}
		d.cache.put(key, e)
		return e, nil
	})
	if err != nil {
		return nil, 0, &DriverError{Op: "SearchByUserIDWithPagination", Err: err}
	}
	return entry.Docs, entry.EstimatedTotal, nil
}

func (d *MeilisearchDriver) RegisterSynonyms(ctx context.Context, synonyms map[string][]string) error {
	task, err := d.index.UpdateSynonymsWithContext(ctx, &synonyms)
	if err != nil {
		return &DriverError{
			Op:  "RegisterSynonyms",
			Err: fmt.Errorf("failed to register synonyms: %w", err),
		}
	}

	if _, err := d.waitForTask(ctx, task.TaskUID); err != nil {
		return &DriverError{
			Op:  "RegisterSynonyms",
			Err: fmt.Errorf("failed to wait for synonyms update: %w", err),
		}
	}

	return nil
}

// buildSecureFilter creates a secure filter from tag filters
func (d *MeilisearchDriver) buildSecureFilter(filters []string) string {
	return makeSecureSearchFilter(filters)
}

// containsCJK checks if text contains CJK characters (Hiragana, Katakana, Han/Kanji).
func containsCJK(text string) bool {
	for _, r := range text {
		if unicode.In(r, unicode.Hiragana, unicode.Katakana, unicode.Han) {
			return true
		}
	}
	return false
}
