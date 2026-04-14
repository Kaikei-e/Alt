package driver

import (
	"context"
	"encoding/json"
	"time"
	"unicode"

	"github.com/meilisearch/meilisearch-go"
)

// MeilisearchDriver isolates admin operations (IndexDocuments, Delete,
// EnsureIndex, RegisterSynonyms) from search-only operations. If the operator
// provisions a dedicated Search API key, L-001 lets us use it only for the
// read path while admin writes keep the higher-privilege key.
type MeilisearchDriver struct {
	client      meilisearch.ServiceManager
	index       meilisearch.IndexManager
	searchIndex meilisearch.IndexManager
}

// NewMeilisearchDriver constructs a driver where the same client handles both
// reads and writes. Used when only a single master/admin key is configured.
func NewMeilisearchDriver(client meilisearch.ServiceManager, indexName string) *MeilisearchDriver {
	idx := client.Index(indexName)
	return &MeilisearchDriver{
		client:      client,
		index:       idx,
		searchIndex: idx,
	}
}

// NewMeilisearchDriverWithClients splits the read and write paths across two
// clients. Pass nil for searchClient to fall back to the admin client.
func NewMeilisearchDriverWithClients(adminClient meilisearch.ServiceManager, searchClient meilisearch.ServiceManager, indexName string) *MeilisearchDriver {
	d := &MeilisearchDriver{
		client:      adminClient,
		index:       adminClient.Index(indexName),
		searchIndex: adminClient.Index(indexName),
	}
	if searchClient != nil {
		d.searchIndex = searchClient.Index(indexName)
	}
	return d
}

func (d *MeilisearchDriver) IndexDocuments(ctx context.Context, docs []SearchDocumentDriver) error {
	if len(docs) == 0 {
		return nil
	}

	task, err := d.index.AddDocuments(docs, nil)
	if err != nil {
		return &DriverError{
			Op:  "IndexDocuments",
			Err: err.Error(),
		}
	}

	// Wait for the indexing task to complete
	_, err = d.index.WaitForTask(task.TaskUID, 15 * time.Second) // 15 seconds timeout
	if err != nil {
		return &DriverError{
			Op:  "IndexDocuments",
			Err: "failed to wait for indexing task: " + err.Error(),
		}
	}

	return nil
}

func (d *MeilisearchDriver) DeleteDocuments(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}

	task, err := d.index.DeleteDocuments(ids, nil)
	if err != nil {
		return &DriverError{
			Op:  "DeleteDocuments",
			Err: err.Error(),
		}
	}

	// Wait for the deletion task to complete
	_, err = d.index.WaitForTask(task.TaskUID, 15 * time.Second) // 15 seconds timeout
	if err != nil {
		return &DriverError{
			Op:  "DeleteDocuments",
			Err: "failed to wait for deletion task: " + err.Error(),
		}
	}

	return nil
}

func (d *MeilisearchDriver) Search(ctx context.Context, query string, limit int) ([]SearchDocumentDriver, error) {
	searchRequest := &meilisearch.SearchRequest{
		Query:            query,
		Limit:            int64(limit),
		ShowRankingScore: true,
	}
	// Locales intentionally omitted: let Meilisearch match across all configured
	// locales (jpn + eng). Previously CJK queries were restricted to jpn-only,
	// which prevented Japanese queries from matching English article content
	// (e.g., "ヴァンス副大統領" could not find "JD Vance" articles).

	result, err := d.searchIndex.Search(query, searchRequest)
	if err != nil {
		return nil, &DriverError{
			Op:  "Search",
			Err: err.Error(),
		}
	}

	docs := make([]SearchDocumentDriver, 0, len(result.Hits))
	for _, hit := range result.Hits {
		doc := SearchDocumentDriver{
			ID:      d.getString(hit, "id"),
			Title:   d.getString(hit, "title"),
			Content: d.getString(hit, "content"),
			Tags:    d.getStringSlice(hit, "tags"),
			Score:   d.getFloat64(hit, "_rankingScore"),
		}
		docs = append(docs, doc)
	}

	return docs, nil
}

func (d *MeilisearchDriver) SearchWithFilters(ctx context.Context, query string, filters []string, limit int) ([]SearchDocumentDriver, error) {
	filter := d.buildSecureFilter(filters)

	searchRequest := &meilisearch.SearchRequest{
		Query:            query,
		Limit:            int64(limit),
		ShowRankingScore: true,
	}

	// Only add filter if it's not empty
	if filter != "" {
		searchRequest.Filter = filter
	}

	result, err := d.searchIndex.Search(query, searchRequest)
	if err != nil {
		return nil, &DriverError{
			Op:  "SearchWithFilters",
			Err: err.Error(),
		}
	}

	docs := make([]SearchDocumentDriver, 0, len(result.Hits))
	for _, hit := range result.Hits {
		doc := SearchDocumentDriver{
			ID:      d.getString(hit, "id"),
			Title:   d.getString(hit, "title"),
			Content: d.getString(hit, "content"),
			Tags:    d.getStringSlice(hit, "tags"),
			Score:   d.getFloat64(hit, "_rankingScore"),
		}
		docs = append(docs, doc)
	}

	return docs, nil
}

func (d *MeilisearchDriver) EnsureIndex(ctx context.Context) error {
	// Check if index exists
	_, err := d.index.FetchInfo()
	if err != nil {
		// Index might not exist, try to create it by adding a dummy document
		dummyDoc := []map[string]interface{}{
			{
				"id":      "init",
				"title":   "Initialization document",
				"content": "This document is used to create the index",
				"tags":    []string{},
			},
		}

		task, err := d.index.AddDocuments(dummyDoc, nil)
		if err != nil {
			return &DriverError{
				Op:  "EnsureIndex",
				Err: "failed to create index: " + err.Error(),
			}
		}

		// Wait for index creation
		_, err = d.index.WaitForTask(task.TaskUID, 15 * time.Second)
		if err != nil {
			return &DriverError{
				Op:  "EnsureIndex",
				Err: "failed to wait for index creation: " + err.Error(),
			}
		}

		// Delete the dummy document
		deleteTask, err := d.index.DeleteDocument("init", nil)
		if err == nil {
			_, _ = d.index.WaitForTask(deleteTask.TaskUID, 15 * time.Second)
		}
	}

	// Configure index settings (best practice: set before indexing)

	// Set searchable attributes (prioritized order)
	searchableAttrs := []string{"title", "content", "tags"}
	if _, err := d.index.UpdateSearchableAttributes(&searchableAttrs); err != nil {
		return &DriverError{
			Op:  "EnsureIndex",
			Err: "failed to set searchable attributes: " + err.Error(),
		}
	}

	// Set filterable attributes for tags and user_id
	filterableAttrs := []interface{}{"tags", "user_id"}
	if _, err := d.index.UpdateFilterableAttributes(&filterableAttrs); err != nil {
		return &DriverError{
			Op:  "EnsureIndex",
			Err: "failed to set filterable attributes: " + err.Error(),
		}
	}

	// Set ranking rules (default + custom)
	rankingRules := []string{
		"words",     // Number of matching query terms
		"typo",      // Number of typos
		"proximity", // Proximity of query terms in document
		"attribute", // Attribute ranking order
		"sort",      // User-defined sort parameter
		"exactness", // Similarity of matched vs. query words
	}
	if _, err := d.index.UpdateRankingRules(&rankingRules); err != nil {
		return &DriverError{
			Op:  "EnsureIndex",
			Err: "failed to set ranking rules: " + err.Error(),
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
	task, localErr := d.index.UpdateLocalizedAttributes(localizedAttrs)
	if localErr != nil {
		return &DriverError{
			Op:  "EnsureIndex",
			Err: "failed to set localized attributes: " + localErr.Error(),
		}
	}
	if _, err := d.index.WaitForTask(task.TaskUID, 15*time.Second); err != nil {
		return &DriverError{
			Op:  "EnsureIndex",
			Err: "failed to wait for localized attributes update: " + err.Error(),
		}
	}

	return nil
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

func (d *MeilisearchDriver) SearchByUserID(ctx context.Context, query string, userID string, limit int) ([]SearchDocumentDriver, error) {
	filter := BuildUserFilter(userID)

	req := &meilisearch.SearchRequest{
		Limit:            int64(limit),
		Filter:           filter,
		ShowRankingScore: true,
	}
	if containsCJK(query) {
		req.Locales = []string{"jpn"}
	}
	result, err := d.searchIndex.Search(query, req)
	if err != nil {
		return nil, &DriverError{Op: "SearchByUserID", Err: err.Error()}
	}

	return d.extractDocs(result.Hits), nil
}

func (d *MeilisearchDriver) SearchByUserIDWithPagination(ctx context.Context, query string, userID string, offset, limit int64) ([]SearchDocumentDriver, int64, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	filter := BuildUserFilter(userID)

	paginReq := &meilisearch.SearchRequest{
		Offset:           offset,
		Limit:            limit,
		Filter:           filter,
		ShowRankingScore: true,
	}
	if containsCJK(query) {
		paginReq.Locales = []string{"jpn"}
	}
	result, err := d.searchIndex.Search(query, paginReq)
	if err != nil {
		return nil, 0, &DriverError{Op: "SearchByUserIDWithPagination", Err: err.Error()}
	}

	return d.extractDocs(result.Hits), result.EstimatedTotalHits, nil
}

func (d *MeilisearchDriver) extractDocs(hits []meilisearch.Hit) []SearchDocumentDriver {
	docs := make([]SearchDocumentDriver, 0, len(hits))
	for _, hit := range hits {
		docs = append(docs, SearchDocumentDriver{
			ID:      d.getString(hit, "id"),
			Title:   d.getString(hit, "title"),
			Content: d.getString(hit, "content"),
			Tags:    d.getStringSlice(hit, "tags"),
			Score:   d.getFloat64(hit, "_rankingScore"),
		})
	}
	return docs
}

func (d *MeilisearchDriver) RegisterSynonyms(ctx context.Context, synonyms map[string][]string) error {
	task, err := d.index.UpdateSynonyms(&synonyms)
	if err != nil {
		return &DriverError{
			Op:  "RegisterSynonyms",
			Err: "failed to register synonyms: " + err.Error(),
		}
	}

	_, err = d.index.WaitForTask(task.TaskUID, 15 * time.Second)
	if err != nil {
		return &DriverError{
			Op:  "RegisterSynonyms",
			Err: "failed to wait for synonyms update: " + err.Error(),
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
