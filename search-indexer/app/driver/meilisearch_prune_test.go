package driver

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/meilisearch/meilisearch-go"
)

// fakePruneServiceManager implements the two ServiceManager methods
// PruneTaskHistory calls; everything else panics loudly if exercised.
type fakePruneServiceManager struct {
	meilisearch.ServiceManager

	idx meilisearch.IndexManager

	deleteErr error
	waitResp  *meilisearch.Task
	waitErr   error
}

func (f *fakePruneServiceManager) Index(_ string) meilisearch.IndexManager { return f.idx }

func (f *fakePruneServiceManager) DeleteTasksWithContext(_ context.Context, _ *meilisearch.DeleteTasksQuery) (*meilisearch.TaskInfo, error) {
	if f.deleteErr != nil {
		return nil, f.deleteErr
	}
	return &meilisearch.TaskInfo{TaskUID: 7}, nil
}

func (f *fakePruneServiceManager) WaitForTaskWithContext(_ context.Context, _ int64, _ time.Duration) (*meilisearch.Task, error) {
	if f.waitErr != nil {
		return nil, f.waitErr
	}
	if f.waitResp != nil {
		return f.waitResp, nil
	}
	return &meilisearch.Task{Status: meilisearch.TaskStatusSucceeded}, nil
}

func newPruneDriver(fake *fakePruneServiceManager) *MeilisearchDriver {
	fake.idx = newFakeIndexManager()
	d := NewMeilisearchDriver(fake, "articles")
	d.taskPollInterval = time.Millisecond
	return d
}

// TestMeilisearchDriver_PruneTaskHistory_FailsOnTaskStatusFailed mirrors the
// check IndexDocuments and the recap driver already carry:
// WaitForTaskWithContext returns a nil error for every terminal status,
// TaskStatusFailed included. Discarding the task therefore reported a prune
// Meilisearch rejected as a successful one, and the prune loop logged "task
// history prune ok" while the task database kept growing -- the exact
// condition that wedged all writes in the 2026-07-22 incident.
func TestMeilisearchDriver_PruneTaskHistory_FailsOnTaskStatusFailed(t *testing.T) {
	d := newPruneDriver(&fakePruneServiceManager{
		waitResp: &meilisearch.Task{UID: 7, Status: meilisearch.TaskStatusFailed},
	})

	err := d.PruneTaskHistory(context.Background(), 72*time.Hour)

	if err == nil {
		t.Fatal("expected PruneTaskHistory to return an error when the taskDeletion task status is failed, got nil")
	}
	var driverErr *DriverError
	if !errors.As(err, &driverErr) {
		t.Fatalf("PruneTaskHistory() error = %T, want *DriverError", err)
	}
	if driverErr.Op != "PruneTaskHistory" {
		t.Errorf("DriverError.Op = %q, want %q", driverErr.Op, "PruneTaskHistory")
	}
}

// TestMeilisearchDriver_PruneTaskHistory_SucceedsWhenTaskSucceeds keeps the
// failure check from turning every prune into an error.
func TestMeilisearchDriver_PruneTaskHistory_SucceedsWhenTaskSucceeds(t *testing.T) {
	d := newPruneDriver(&fakePruneServiceManager{
		waitResp: &meilisearch.Task{UID: 7, Status: meilisearch.TaskStatusSucceeded},
	})

	if err := d.PruneTaskHistory(context.Background(), 72*time.Hour); err != nil {
		t.Fatalf("PruneTaskHistory() error = %v, want nil", err)
	}
}
