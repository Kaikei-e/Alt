package driver

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/meilisearch/meilisearch-go"
)

// MeilisearchRecapDriver handles Meilisearch operations for the "recaps" index.
type MeilisearchRecapDriver struct {
	client meilisearch.ServiceManager
	index  meilisearch.IndexManager

	taskWaitTimeout  time.Duration
	taskPollInterval time.Duration
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
		client:           client,
		index:            client.Index("recaps"),
		taskWaitTimeout:  meilisearchTaskWaitTimeout,
		taskPollInterval: meilisearchTaskPollInterval,
	}
}

// waitForTask bounds every task wait. WaitForTask's second argument is a
// polling interval and the non-context variant bakes context.Background() in,
// so a stalled Meilisearch task queue would otherwise wedge the caller
// forever -- at startup that means EnsureIndex never returns and the HTTP
// listeners never open. See the comment on meilisearchTaskWaitTimeout.
//
// The task itself carries the outcome: WaitForTaskWithContext returns a nil
// error once the task reaches ANY terminal status, TaskStatusFailed included,
// so a write Meilisearch rejected reads as a successful one unless the status
// is inspected here.
func (d *MeilisearchRecapDriver) waitForTask(ctx context.Context, taskUID int64) (*meilisearch.Task, error) {
	waitCtx, cancel := context.WithTimeout(ctx, d.taskWaitTimeout)
	defer cancel()
	task, err := d.index.WaitForTaskWithContext(waitCtx, taskUID, d.taskPollInterval)
	if err != nil {
		return task, err
	}
	if task != nil && task.Status == meilisearch.TaskStatusFailed {
		return task, fmt.Errorf("meilisearch task %d failed: %s (code=%s)", task.UID, task.Error.Message, task.Error.Code)
	}
	return task, nil
}

// EnsureIndex creates and configures the "recaps" index.
func (d *MeilisearchRecapDriver) EnsureIndex(ctx context.Context) error {
	// Try to fetch index info; if it doesn't exist, create it
	_, err := d.index.FetchInfoWithContext(ctx)
	if err != nil {
		dummyDoc := []map[string]interface{}{
			{
				"id":          "init",
				"job_id":      "",
				"genre":       "",
				"top_terms":   []string{},
				"summary":     "",
				"executed_at": "",
			},
		}

		pk := "id"
		task, err := d.index.AddDocumentsWithContext(ctx, dummyDoc, &meilisearch.DocumentOptions{PrimaryKey: &pk})
		if err != nil {
			return &DriverError{Op: "EnsureRecapIndex", Err: fmt.Errorf("failed to create index: %w", err)}
		}
		if _, err = d.waitForTask(ctx, task.TaskUID); err != nil {
			return &DriverError{Op: "EnsureRecapIndex", Err: fmt.Errorf("failed to wait for index creation: %w", err)}
		}

		deleteTask, err := d.index.DeleteDocumentWithContext(ctx, "init", nil)
		if err == nil {
			_, _ = d.waitForTask(ctx, deleteTask.TaskUID)
		}
	}

	// Settings are applied before indexing, and each async task is waited on
	// so the next write already sees them.

	// Searchable attributes: tags first (semantic), then top_terms (statistical), summary, genre
	searchableAttrs := []string{"tags", "top_terms", "summary", "genre"}
	searchableTask, err := d.index.UpdateSearchableAttributesWithContext(ctx, &searchableAttrs)
	if err != nil {
		return &DriverError{Op: "EnsureRecapIndex", Err: fmt.Errorf("failed to set searchable attributes: %w", err)}
	}
	if _, err := d.waitForTask(ctx, searchableTask.TaskUID); err != nil {
		return &DriverError{Op: "EnsureRecapIndex", Err: fmt.Errorf("failed to wait for searchable attributes update: %w", err)}
	}

	// Filterable attributes for faceting
	filterableAttrs := []interface{}{"genre", "window_days"}
	filterableTask, err := d.index.UpdateFilterableAttributesWithContext(ctx, &filterableAttrs)
	if err != nil {
		return &DriverError{Op: "EnsureRecapIndex", Err: fmt.Errorf("failed to set filterable attributes: %w", err)}
	}
	if _, err := d.waitForTask(ctx, filterableTask.TaskUID); err != nil {
		return &DriverError{Op: "EnsureRecapIndex", Err: fmt.Errorf("failed to wait for filterable attributes update: %w", err)}
	}

	// Sortable attributes
	sortableAttrs := []string{"executed_at"}
	sortableTask, err := d.index.UpdateSortableAttributesWithContext(ctx, &sortableAttrs)
	if err != nil {
		return &DriverError{Op: "EnsureRecapIndex", Err: fmt.Errorf("failed to set sortable attributes: %w", err)}
	}
	if _, err := d.waitForTask(ctx, sortableTask.TaskUID); err != nil {
		return &DriverError{Op: "EnsureRecapIndex", Err: fmt.Errorf("failed to wait for sortable attributes update: %w", err)}
	}

	return nil
}

// IndexDocuments indexes recap documents into Meilisearch.
func (d *MeilisearchRecapDriver) IndexDocuments(ctx context.Context, docs []RecapDocumentDriver) error {
	if len(docs) == 0 {
		return nil
	}

	pk := "id"
	task, err := d.index.AddDocumentsWithContext(ctx, docs, &meilisearch.DocumentOptions{PrimaryKey: &pk})
	if err != nil {
		return &DriverError{Op: "IndexRecapDocuments", Err: err}
	}

	if _, err = d.waitForTask(ctx, task.TaskUID); err != nil {
		return &DriverError{Op: "IndexRecapDocuments", Err: fmt.Errorf("failed to wait for indexing: %w", err)}
	}

	return nil
}

// Search searches the recaps index.
func (d *MeilisearchRecapDriver) Search(ctx context.Context, query string, limit int) ([]RecapDocumentDriver, int64, error) {
	result, err := d.index.SearchWithContext(ctx, query, &meilisearch.SearchRequest{
		Limit: int64(limit),
		Sort:  []string{"executed_at:desc"},
	})
	if err != nil {
		return nil, 0, &DriverError{Op: "SearchRecaps", Err: err}
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
