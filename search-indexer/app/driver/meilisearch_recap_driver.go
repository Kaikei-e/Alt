package driver

import (
	"context"
	"encoding/json"
	"time"

	"github.com/meilisearch/meilisearch-go"
)

// MeilisearchRecapDriver handles Meilisearch operations for the "recaps" index.
type MeilisearchRecapDriver struct {
	client meilisearch.ServiceManager
	index  meilisearch.IndexManager
}

// RecapDocumentDriver represents a recap document in Meilisearch.
type RecapDocumentDriver struct {
	ID         string   `json:"id"`
	JobID      string   `json:"job_id"`
	ExecutedAt string   `json:"executed_at"`
	WindowDays int      `json:"window_days"`
	Genre      string   `json:"genre"`
	Summary    string   `json:"summary"`
	TopTerms   []string `json:"top_terms"`
	Tags       []string `json:"tags"`
	Bullets    []string `json:"bullets"`
}

// NewMeilisearchRecapDriver creates a new Meilisearch driver for the "recaps" index.
func NewMeilisearchRecapDriver(client meilisearch.ServiceManager) *MeilisearchRecapDriver {
	return &MeilisearchRecapDriver{
		client: client,
		index:  client.Index("recaps"),
	}
}

// EnsureIndex creates and configures the "recaps" index.
func (d *MeilisearchRecapDriver) EnsureIndex(ctx context.Context) error {
	// Try to fetch index info; if it doesn't exist, create it
	_, err := d.index.FetchInfo()
	if err != nil {
		dummyDoc := []map[string]interface{}{
			{
				"id":         "init",
				"job_id":     "",
				"genre":      "",
				"top_terms":  []string{},
				"summary":    "",
				"executed_at": "",
			},
		}

		pk := "id"
		task, err := d.index.AddDocuments(dummyDoc, &meilisearch.DocumentOptions{PrimaryKey: &pk})
		if err != nil {
			return &DriverError{Op: "EnsureRecapIndex", Err: "failed to create index: " + err.Error()}
		}
		if _, err = d.index.WaitForTask(task.TaskUID, 15*time.Second); err != nil {
			return &DriverError{Op: "EnsureRecapIndex", Err: "failed to wait for index creation: " + err.Error()}
		}

		deleteTask, err := d.index.DeleteDocument("init", nil)
		if err == nil {
			_, _ = d.index.WaitForTask(deleteTask.TaskUID, 15*time.Second)
		}
	}

	// Searchable attributes: tags first (semantic), then top_terms (statistical), summary, genre
	searchableAttrs := []string{"tags", "top_terms", "summary", "genre"}
	if _, err := d.index.UpdateSearchableAttributes(&searchableAttrs); err != nil {
		return &DriverError{Op: "EnsureRecapIndex", Err: "failed to set searchable attributes: " + err.Error()}
	}

	// Filterable attributes for faceting
	filterableAttrs := []interface{}{"genre", "window_days"}
	if _, err := d.index.UpdateFilterableAttributes(&filterableAttrs); err != nil {
		return &DriverError{Op: "EnsureRecapIndex", Err: "failed to set filterable attributes: " + err.Error()}
	}

	// Sortable attributes
	sortableAttrs := []string{"executed_at"}
	if _, err := d.index.UpdateSortableAttributes(&sortableAttrs); err != nil {
		return &DriverError{Op: "EnsureRecapIndex", Err: "failed to set sortable attributes: " + err.Error()}
	}

	return nil
}

// IndexDocuments indexes recap documents into Meilisearch.
func (d *MeilisearchRecapDriver) IndexDocuments(ctx context.Context, docs []RecapDocumentDriver) error {
	if len(docs) == 0 {
		return nil
	}

	pk := "id"
	task, err := d.index.AddDocuments(docs, &meilisearch.DocumentOptions{PrimaryKey: &pk})
	if err != nil {
		return &DriverError{Op: "IndexRecapDocuments", Err: err.Error()}
	}

	if _, err = d.index.WaitForTask(task.TaskUID, 15*time.Second); err != nil {
		return &DriverError{Op: "IndexRecapDocuments", Err: "failed to wait for indexing: " + err.Error()}
	}

	return nil
}

// Search searches the recaps index.
func (d *MeilisearchRecapDriver) Search(ctx context.Context, query string, limit int) ([]RecapDocumentDriver, int64, error) {
	result, err := d.index.Search(query, &meilisearch.SearchRequest{
		Limit: int64(limit),
		Sort:  []string{"executed_at:desc"},
	})
	if err != nil {
		return nil, 0, &DriverError{Op: "SearchRecaps", Err: err.Error()}
	}

	docs := make([]RecapDocumentDriver, 0, len(result.Hits))
	for _, hit := range result.Hits {
		docs = append(docs, RecapDocumentDriver{
			ID:         d.getString(hit, "id"),
			JobID:      d.getString(hit, "job_id"),
			ExecutedAt: d.getString(hit, "executed_at"),
			WindowDays: d.getInt(hit, "window_days"),
			Genre:      d.getString(hit, "genre"),
			Summary:    d.getString(hit, "summary"),
			TopTerms:   d.getStringSlice(hit, "top_terms"),
			Tags:       d.getStringSlice(hit, "tags"),
			Bullets:    d.getStringSlice(hit, "bullets"),
		})
	}

	return docs, result.EstimatedTotalHits, nil
}

func (d *MeilisearchRecapDriver) getString(m meilisearch.Hit, key string) string {
	if v, ok := m[key]; ok {
		var s string
		if err := json.Unmarshal(v, &s); err == nil {
			return s
		}
	}
	return ""
}

func (d *MeilisearchRecapDriver) getInt(m meilisearch.Hit, key string) int {
	if v, ok := m[key]; ok {
		var n float64
		if err := json.Unmarshal(v, &n); err == nil {
			return int(n)
		}
	}
	return 0
}

func (d *MeilisearchRecapDriver) getStringSlice(m meilisearch.Hit, key string) []string {
	if v, ok := m[key]; ok {
		var slice []string
		if err := json.Unmarshal(v, &slice); err == nil {
			return slice
		}
	}
	return []string{}
}
