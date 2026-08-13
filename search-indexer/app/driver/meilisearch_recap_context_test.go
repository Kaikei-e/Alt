package driver

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/meilisearch/meilisearch-go"
)

// fakeRecapIndexManager mirrors fakeIndexManager but implements the
// non-context SDK methods MeilisearchRecapDriver calls, so the recap driver's
// wait behaviour can be observed without touching Meilisearch.
type fakeRecapIndexManager struct {
	meilisearch.IndexManager

	mu    sync.Mutex
	calls map[string]context.Context

	// fetchInfoErr makes EnsureIndex take the index-creation branch, which is
	// the one that runs at startup before the HTTP listeners open.
	fetchInfoErr error

	// waitBlock simulates a Meilisearch task queue that never drains.
	// release unblocks it at test teardown so no goroutine outlives the test.
	waitBlock bool
	release   chan struct{}

	// waitResp overrides the task WaitForTaskWithContext reports back, so a
	// terminal-but-failed task can be simulated.
	waitResp *meilisearch.Task

	// stall simulates the observed saturation behaviour: Meilisearch accepts
	// the connection but never answers. Context-aware calls abort on
	// ctx.Done(); the non-context ones have nothing to abort on and block.
	stall bool
}

func newFakeRecapIndexManager(t *testing.T) *fakeRecapIndexManager {
	f := &fakeRecapIndexManager{
		calls:   make(map[string]context.Context),
		release: make(chan struct{}),
	}
	t.Cleanup(func() { close(f.release) })
	return f
}

func (f *fakeRecapIndexManager) record(name string, ctx context.Context) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls[name] = ctx
}

func (f *fakeRecapIndexManager) ctxFor(name string) context.Context {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls[name]
}

func (f *fakeRecapIndexManager) called(name string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, ok := f.calls[name]
	return ok
}

// stallCtx models a request whose response never arrives but whose context
// can still cancel it.
func (f *fakeRecapIndexManager) stallCtx(ctx context.Context) error {
	if !f.stall {
		return nil
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-f.release:
		return context.Canceled
	}
}

// stallNoCtx models the same request issued through a non-context SDK method:
// meilisearch-go bakes context.Background() in and its default http.Client
// sets no response timeout, so nothing can unblock it.
func (f *fakeRecapIndexManager) stallNoCtx() {
	if f.stall {
		<-f.release
	}
}

func (f *fakeRecapIndexManager) FetchInfo() (*meilisearch.IndexResult, error) {
	f.record("FetchInfoNoContext", context.Background())
	f.stallNoCtx()
	if f.fetchInfoErr != nil {
		return nil, f.fetchInfoErr
	}
	return &meilisearch.IndexResult{}, nil
}

func (f *fakeRecapIndexManager) FetchInfoWithContext(ctx context.Context) (*meilisearch.IndexResult, error) {
	f.record("FetchInfo", ctx)
	if err := f.stallCtx(ctx); err != nil {
		return nil, err
	}
	if f.fetchInfoErr != nil {
		return nil, f.fetchInfoErr
	}
	return &meilisearch.IndexResult{}, nil
}

func (f *fakeRecapIndexManager) AddDocuments(_ interface{}, _ *meilisearch.DocumentOptions) (*meilisearch.TaskInfo, error) {
	f.record("AddDocumentsNoContext", context.Background())
	f.stallNoCtx()
	return &meilisearch.TaskInfo{TaskUID: 1}, nil
}

func (f *fakeRecapIndexManager) AddDocumentsWithContext(ctx context.Context, _ interface{}, _ *meilisearch.DocumentOptions) (*meilisearch.TaskInfo, error) {
	f.record("AddDocuments", ctx)
	if err := f.stallCtx(ctx); err != nil {
		return nil, err
	}
	return &meilisearch.TaskInfo{TaskUID: 1}, nil
}

func (f *fakeRecapIndexManager) DeleteDocument(_ string, _ *meilisearch.DocumentOptions) (*meilisearch.TaskInfo, error) {
	f.record("DeleteDocumentNoContext", context.Background())
	f.stallNoCtx()
	return &meilisearch.TaskInfo{TaskUID: 2}, nil
}

func (f *fakeRecapIndexManager) DeleteDocumentWithContext(ctx context.Context, _ string, _ *meilisearch.DocumentOptions) (*meilisearch.TaskInfo, error) {
	f.record("DeleteDocument", ctx)
	if err := f.stallCtx(ctx); err != nil {
		return nil, err
	}
	return &meilisearch.TaskInfo{TaskUID: 2}, nil
}

func (f *fakeRecapIndexManager) UpdateSearchableAttributes(_ *[]string) (*meilisearch.TaskInfo, error) {
	f.record("UpdateSearchableAttributesNoContext", context.Background())
	f.stallNoCtx()
	return &meilisearch.TaskInfo{TaskUID: 3}, nil
}

func (f *fakeRecapIndexManager) UpdateSearchableAttributesWithContext(ctx context.Context, _ *[]string) (*meilisearch.TaskInfo, error) {
	f.record("UpdateSearchableAttributes", ctx)
	if err := f.stallCtx(ctx); err != nil {
		return nil, err
	}
	return &meilisearch.TaskInfo{TaskUID: 3}, nil
}

func (f *fakeRecapIndexManager) UpdateFilterableAttributes(_ *[]interface{}) (*meilisearch.TaskInfo, error) {
	f.record("UpdateFilterableAttributesNoContext", context.Background())
	f.stallNoCtx()
	return &meilisearch.TaskInfo{TaskUID: 4}, nil
}

func (f *fakeRecapIndexManager) UpdateFilterableAttributesWithContext(ctx context.Context, _ *[]interface{}) (*meilisearch.TaskInfo, error) {
	f.record("UpdateFilterableAttributes", ctx)
	if err := f.stallCtx(ctx); err != nil {
		return nil, err
	}
	return &meilisearch.TaskInfo{TaskUID: 4}, nil
}

func (f *fakeRecapIndexManager) UpdateSortableAttributes(_ *[]string) (*meilisearch.TaskInfo, error) {
	f.record("UpdateSortableAttributesNoContext", context.Background())
	f.stallNoCtx()
	return &meilisearch.TaskInfo{TaskUID: 5}, nil
}

func (f *fakeRecapIndexManager) UpdateSortableAttributesWithContext(ctx context.Context, _ *[]string) (*meilisearch.TaskInfo, error) {
	f.record("UpdateSortableAttributes", ctx)
	if err := f.stallCtx(ctx); err != nil {
		return nil, err
	}
	return &meilisearch.TaskInfo{TaskUID: 5}, nil
}

func (f *fakeRecapIndexManager) Search(_ string, _ *meilisearch.SearchRequest) (*meilisearch.SearchResponse, error) {
	f.record("SearchNoContext", context.Background())
	f.stallNoCtx()
	return &meilisearch.SearchResponse{}, nil
}

func (f *fakeRecapIndexManager) SearchWithContext(ctx context.Context, _ string, _ *meilisearch.SearchRequest) (*meilisearch.SearchResponse, error) {
	f.record("Search", ctx)
	if err := f.stallCtx(ctx); err != nil {
		return nil, err
	}
	return &meilisearch.SearchResponse{}, nil
}

// WaitForTask is the misuse under test: its second argument is a poll
// interval and the SDK bakes context.Background() in, so a stalled task queue
// blocks the caller forever.
func (f *fakeRecapIndexManager) WaitForTask(_ int64, _ time.Duration) (*meilisearch.Task, error) {
	f.record("WaitForTaskNoContext", context.Background())
	if f.waitBlock {
		<-f.release
	}
	return &meilisearch.Task{Status: meilisearch.TaskStatusSucceeded}, nil
}

func (f *fakeRecapIndexManager) WaitForTaskWithContext(ctx context.Context, _ int64, _ time.Duration) (*meilisearch.Task, error) {
	f.record("WaitForTask", ctx)
	if f.waitBlock {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-f.release:
			return nil, context.Canceled
		}
	}
	if f.waitResp != nil {
		return f.waitResp, nil
	}
	return &meilisearch.Task{Status: meilisearch.TaskStatusSucceeded}, nil
}

// TestMeilisearchRecapDriver_IndexDocuments_WaitIsBounded pins the recap
// driver to the same bounded wait the article driver already uses: a stalled
// Meilisearch task queue must surface as an error instead of wedging the
// recap indexing goroutine while /health keeps returning 200.
func TestMeilisearchRecapDriver_IndexDocuments_WaitIsBounded(t *testing.T) {
	fake := newFakeRecapIndexManager(t)
	fake.waitBlock = true
	d := NewMeilisearchRecapDriver(&fakeServiceManager{idx: fake})

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- d.IndexDocuments(ctx, []RecapDocumentDriver{{ID: "recap-1"}})
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected IndexDocuments to return an error when the task never completes")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("IndexDocuments never returned: the task wait is unbounded")
	}

	if fake.called("WaitForTaskNoContext") {
		t.Fatal("the recap driver must not use the non-context WaitForTask; it hard-codes context.Background()")
	}
}

// TestMeilisearchRecapDriver_IndexDocuments_WaitHasOwnDeadline covers the
// caller that passes a context with no deadline at all -- the wait must still
// be bounded by the driver's own timeout, and must stay derived from the
// caller's context so cancellation propagates.
func TestMeilisearchRecapDriver_IndexDocuments_WaitHasOwnDeadline(t *testing.T) {
	fake := newFakeRecapIndexManager(t)
	d := NewMeilisearchRecapDriver(&fakeServiceManager{idx: fake})

	ctx := withMarker(context.Background(), "caller-marker")

	if err := d.IndexDocuments(ctx, []RecapDocumentDriver{{ID: "recap-1"}}); err != nil {
		t.Fatalf("IndexDocuments() error = %v", err)
	}

	waitCtx := fake.ctxFor("WaitForTask")
	if waitCtx == nil {
		t.Fatal("WaitForTaskWithContext was never called")
	}
	if _, ok := waitCtx.Deadline(); !ok {
		t.Fatal("WaitForTaskWithContext must be called with a context carrying a deadline (bounded wait)")
	}
	if got := markerOf(waitCtx); got != "caller-marker" {
		t.Fatalf("WaitForTaskWithContext ctx marker = %q, want it derived from the caller's context", got)
	}
}

// TestMeilisearchRecapDriver_IndexDocuments_FailsOnTaskStatusFailed mirrors
// the article driver: WaitForTaskWithContext returns a nil error for every
// terminal status, failed included, so a recap write that Meilisearch
// rejected must not be reported as a success.
func TestMeilisearchRecapDriver_IndexDocuments_FailsOnTaskStatusFailed(t *testing.T) {
	fake := newFakeRecapIndexManager(t)
	fake.waitResp = &meilisearch.Task{Status: meilisearch.TaskStatusFailed}
	d := NewMeilisearchRecapDriver(&fakeServiceManager{idx: fake})

	err := d.IndexDocuments(context.Background(), []RecapDocumentDriver{{ID: "recap-1"}})

	if err == nil {
		t.Fatal("expected IndexDocuments to return an error when the Meilisearch task status is failed, got nil")
	}
}

// TestMeilisearchRecapDriver_EnsureIndex_ReturnsWhenContextIsDone models the
// startup failure the bounded task wait does not cover: Meilisearch accepts
// the connection but never answers the request itself. EnsureIndex runs
// synchronously before the HTTP/Connect listeners open, so a request that
// cannot be cancelled means :9300 and :9301 never bind at all.
func TestMeilisearchRecapDriver_EnsureIndex_ReturnsWhenContextIsDone(t *testing.T) {
	fake := newFakeRecapIndexManager(t)
	fake.stall = true
	d := NewMeilisearchRecapDriver(&fakeServiceManager{idx: fake})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	done := make(chan error, 1)
	go func() {
		done <- d.EnsureIndex(ctx)
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected EnsureIndex to return an error when the caller's context is already done")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("EnsureIndex never returned: a stalled Meilisearch request is not cancellable")
	}

	for _, call := range []string{"FetchInfoNoContext", "AddDocumentsNoContext"} {
		if fake.called(call) {
			t.Fatalf("%s was used; the non-context SDK variants hard-code context.Background() and cannot be cancelled", call)
		}
	}
}

// TestMeilisearchRecapDriver_EnsureIndex_PropagatesCallerContext pins every
// EnsureIndex call to the caller's context, including the settings updates
// that run after the index exists.
func TestMeilisearchRecapDriver_EnsureIndex_PropagatesCallerContext(t *testing.T) {
	fake := newFakeRecapIndexManager(t)
	d := NewMeilisearchRecapDriver(&fakeServiceManager{idx: fake})

	ctx := withMarker(context.Background(), "caller-marker")

	if err := d.EnsureIndex(ctx); err != nil {
		t.Fatalf("EnsureIndex() error = %v", err)
	}

	for _, call := range []string{
		"FetchInfo",
		"UpdateSearchableAttributes",
		"UpdateFilterableAttributes",
		"UpdateSortableAttributes",
	} {
		if got := markerOf(fake.ctxFor(call)); got != "caller-marker" {
			t.Fatalf("%s ctx marker = %q, want the caller's context to propagate", call, got)
		}
	}
}

// TestMeilisearchRecapDriver_IndexDocuments_PropagatesCallerContext covers the
// recap write path's document upload, not just its task wait.
func TestMeilisearchRecapDriver_IndexDocuments_PropagatesCallerContext(t *testing.T) {
	fake := newFakeRecapIndexManager(t)
	d := NewMeilisearchRecapDriver(&fakeServiceManager{idx: fake})

	ctx := withMarker(context.Background(), "caller-marker")

	if err := d.IndexDocuments(ctx, []RecapDocumentDriver{{ID: "recap-1"}}); err != nil {
		t.Fatalf("IndexDocuments() error = %v", err)
	}

	if got := markerOf(fake.ctxFor("AddDocuments")); got != "caller-marker" {
		t.Fatalf("AddDocumentsWithContext ctx marker = %q, want the caller's context to propagate", got)
	}
}

// TestMeilisearchRecapDriver_Search_PropagatesCallerContext covers the read
// path, which serves Connect-RPC requests that carry their own deadline.
func TestMeilisearchRecapDriver_Search_PropagatesCallerContext(t *testing.T) {
	fake := newFakeRecapIndexManager(t)
	d := NewMeilisearchRecapDriver(&fakeServiceManager{idx: fake})

	ctx := withMarker(context.Background(), "caller-marker")

	if _, _, err := d.Search(ctx, "query", 10); err != nil {
		t.Fatalf("Search() error = %v", err)
	}

	if got := markerOf(fake.ctxFor("Search")); got != "caller-marker" {
		t.Fatalf("SearchWithContext ctx marker = %q, want the caller's context to propagate", got)
	}
}

// TestMeilisearchRecapDriver_EnsureIndex_WaitIsBounded covers the startup
// path: EnsureIndex runs before the HTTP listeners open, so an unbounded wait
// there means the ports never come up at all.
func TestMeilisearchRecapDriver_EnsureIndex_WaitIsBounded(t *testing.T) {
	fake := newFakeRecapIndexManager(t)
	fake.fetchInfoErr = errors.New("index not found")
	fake.waitBlock = true
	d := NewMeilisearchRecapDriver(&fakeServiceManager{idx: fake})

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- d.EnsureIndex(ctx)
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected EnsureIndex to return an error when index creation never completes")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("EnsureIndex never returned: the task wait is unbounded")
	}
}
