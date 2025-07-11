package driver

import (
	"context"
	"search-indexer/search_engine"

	"github.com/meilisearch/meilisearch-go"
)

type MeilisearchDriver struct {
	client meilisearch.ServiceManager
	index  meilisearch.IndexManager
}

func NewMeilisearchDriver(client meilisearch.ServiceManager, indexName string) *MeilisearchDriver {
	return &MeilisearchDriver{
		client: client,
		index:  client.Index(indexName),
	}
}

func (d *MeilisearchDriver) IndexDocuments(ctx context.Context, docs []SearchDocumentDriver) error {
	if len(docs) == 0 {
		return nil
	}

	task, err := d.index.AddDocuments(docs)
	if err != nil {
		return &DriverError{
			Op:  "IndexDocuments",
			Err: err.Error(),
		}
	}

	// Wait for the indexing task to complete
	_, err = d.index.WaitForTask(task.TaskUID, 15*1000) // 15 seconds timeout
	if err != nil {
		return &DriverError{
			Op:  "IndexDocuments",
			Err: "failed to wait for indexing task: " + err.Error(),
		}
	}

	return nil
}

func (d *MeilisearchDriver) Search(ctx context.Context, query string, limit int) ([]SearchDocumentDriver, error) {
	searchRequest := &meilisearch.SearchRequest{
		Query: query,
		Limit: int64(limit),
	}

	result, err := d.index.Search(query, searchRequest)
	if err != nil {
		return nil, &DriverError{
			Op:  "Search",
			Err: err.Error(),
		}
	}

	docs := make([]SearchDocumentDriver, 0, len(result.Hits))
	for _, hit := range result.Hits {
		hitMap, ok := hit.(map[string]interface{})
		if !ok {
			continue
		}

		doc := SearchDocumentDriver{
			ID:      d.getString(hitMap, "id"),
			Title:   d.getString(hitMap, "title"),
			Content: d.getString(hitMap, "content"),
			Tags:    d.getStringSlice(hitMap, "tags"),
		}
		docs = append(docs, doc)
	}

	return docs, nil
}

func (d *MeilisearchDriver) SearchWithFilters(ctx context.Context, query string, filters []string, limit int) ([]SearchDocumentDriver, error) {
	filter := d.buildSecureFilter(filters)
	
	searchRequest := &meilisearch.SearchRequest{
		Query: query,
		Limit: int64(limit),
	}

	// Only add filter if it's not empty
	if filter != "" {
		searchRequest.Filter = filter
	}

	result, err := d.index.Search(query, searchRequest)
	if err != nil {
		return nil, &DriverError{
			Op:  "SearchWithFilters",
			Err: err.Error(),
		}
	}

	docs := make([]SearchDocumentDriver, 0, len(result.Hits))
	for _, hit := range result.Hits {
		hitMap, ok := hit.(map[string]interface{})
		if !ok {
			continue
		}

		doc := SearchDocumentDriver{
			ID:      d.getString(hitMap, "id"),
			Title:   d.getString(hitMap, "title"),
			Content: d.getString(hitMap, "content"),
			Tags:    d.getStringSlice(hitMap, "tags"),
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

		task, err := d.index.AddDocuments(dummyDoc)
		if err != nil {
			return &DriverError{
				Op:  "EnsureIndex",
				Err: "failed to create index: " + err.Error(),
			}
		}

		// Wait for index creation
		_, err = d.index.WaitForTask(task.TaskUID, 15*1000)
		if err != nil {
			return &DriverError{
				Op:  "EnsureIndex",
				Err: "failed to wait for index creation: " + err.Error(),
			}
		}

		// Delete the dummy document
		deleteTask, err := d.index.DeleteDocument("init")
		if err == nil {
			d.index.WaitForTask(deleteTask.TaskUID, 15*1000)
		}
	}

	// Set filterable attributes for tags
	_, err = d.index.UpdateFilterableAttributes(&[]string{"tags"})
	if err != nil {
		return &DriverError{
			Op:  "EnsureIndex",
			Err: "failed to set filterable attributes: " + err.Error(),
		}
	}

	return nil
}

func (d *MeilisearchDriver) getString(m map[string]interface{}, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func (d *MeilisearchDriver) getStringSlice(m map[string]interface{}, key string) []string {
	if v, ok := m[key]; ok {
		if slice, ok := v.([]interface{}); ok {
			result := make([]string, 0, len(slice))
			for _, item := range slice {
				if s, ok := item.(string); ok {
					result = append(result, s)
				}
			}
			return result
		}
	}
	return []string{}
}

func (d *MeilisearchDriver) RegisterSynonyms(ctx context.Context, synonyms map[string][]string) error {
	task, err := d.index.UpdateSynonyms(&synonyms)
	if err != nil {
		return &DriverError{
			Op:  "RegisterSynonyms",
			Err: "failed to register synonyms: " + err.Error(),
		}
	}

	_, err = d.index.WaitForTask(task.TaskUID, 15*1000)
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
	return search_engine.MakeSecureSearchFilter(filters)
}
