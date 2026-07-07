package driver

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"search-indexer/logger"

	"github.com/meilisearch/meilisearch-go"
)

// TestMain ensures logger.Logger is initialized before any test in this
// package runs -- recordProcessing (invoked by Search/SearchWithFilters/
// etc.) calls logger.Logger.DebugContext unconditionally, and the package
// had no test exercising those methods end-to-end before this file.
func TestMain(m *testing.M) {
	logger.Init()
	os.Exit(m.Run())
}

// fakeIndexManager implements only the meilisearch.IndexManager methods
// MeilisearchDriver actually calls, embedding the interface so the compiler
// is satisfied for the rest; any unimplemented method panics loudly if
// exercised rather than silently returning a zero value.
type fakeIndexManager struct {
	meilisearch.IndexManager

	mu    sync.Mutex
	calls map[string]context.Context

	// waitBlock simulates a Meilisearch task that never completes -- the
	// exact scenario the old WaitForTask(taskUID, 15*time.Second) misuse
	// could not bound, since its second argument is a poll interval, not a
	// timeout.
	waitBlock bool
	waitResp  *meilisearch.Task
	waitErr   error
}

func newFakeIndexManager() *fakeIndexManager {
	return &fakeIndexManager{calls: make(map[string]context.Context)}
}

func (f *fakeIndexManager) record(name string, ctx context.Context) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls[name] = ctx
}

func (f *fakeIndexManager) ctxFor(name string) context.Context {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls[name]
}

func (f *fakeIndexManager) FetchInfoWithContext(ctx context.Context) (*meilisearch.IndexResult, error) {
	f.record("FetchInfo", ctx)
	return &meilisearch.IndexResult{}, nil
}

func (f *fakeIndexManager) AddDocumentsWithContext(ctx context.Context, _ interface{}, _ *meilisearch.DocumentOptions) (*meilisearch.TaskInfo, error) {
	f.record("AddDocuments", ctx)
	return &meilisearch.TaskInfo{TaskUID: 1}, nil
}

func (f *fakeIndexManager) DeleteDocumentWithContext(ctx context.Context, _ string, _ *meilisearch.DocumentOptions) (*meilisearch.TaskInfo, error) {
	f.record("DeleteDocument", ctx)
	return &meilisearch.TaskInfo{TaskUID: 1}, nil
}

func (f *fakeIndexManager) DeleteDocumentsWithContext(ctx context.Context, _ []string, _ *meilisearch.DocumentOptions) (*meilisearch.TaskInfo, error) {
	f.record("DeleteDocuments", ctx)
	return &meilisearch.TaskInfo{TaskUID: 1}, nil
}

func (f *fakeIndexManager) UpdateSearchableAttributesWithContext(ctx context.Context, _ *[]string) (*meilisearch.TaskInfo, error) {
	f.record("UpdateSearchableAttributes", ctx)
	return &meilisearch.TaskInfo{TaskUID: 1}, nil
}

func (f *fakeIndexManager) UpdateFilterableAttributesWithContext(ctx context.Context, _ *[]interface{}) (*meilisearch.TaskInfo, error) {
	f.record("UpdateFilterableAttributes", ctx)
	return &meilisearch.TaskInfo{TaskUID: 1}, nil
}

func (f *fakeIndexManager) UpdateRankingRulesWithContext(ctx context.Context, _ *[]string) (*meilisearch.TaskInfo, error) {
	f.record("UpdateRankingRules", ctx)
	return &meilisearch.TaskInfo{TaskUID: 1}, nil
}

func (f *fakeIndexManager) UpdateLocalizedAttributesWithContext(ctx context.Context, _ []*meilisearch.LocalizedAttributes) (*meilisearch.TaskInfo, error) {
	f.record("UpdateLocalizedAttributes", ctx)
	return &meilisearch.TaskInfo{TaskUID: 1}, nil
}

func (f *fakeIndexManager) UpdateSearchCutoffMsWithContext(ctx context.Context, _ int64) (*meilisearch.TaskInfo, error) {
	f.record("UpdateSearchCutoffMs", ctx)
	return &meilisearch.TaskInfo{TaskUID: 1}, nil
}

func (f *fakeIndexManager) UpdateSynonymsWithContext(ctx context.Context, _ *map[string][]string) (*meilisearch.TaskInfo, error) {
	f.record("UpdateSynonyms", ctx)
	return &meilisearch.TaskInfo{TaskUID: 1}, nil
}

func (f *fakeIndexManager) SearchWithContext(ctx context.Context, _ string, _ *meilisearch.SearchRequest) (*meilisearch.SearchResponse, error) {
	f.record("Search", ctx)
	return &meilisearch.SearchResponse{}, nil
}

func (f *fakeIndexManager) WaitForTaskWithContext(ctx context.Context, _ int64, _ time.Duration) (*meilisearch.Task, error) {
	f.record("WaitForTask", ctx)
	if f.waitBlock {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	if f.waitResp != nil || f.waitErr != nil {
		return f.waitResp, f.waitErr
	}
	return &meilisearch.Task{Status: meilisearch.TaskStatusSucceeded}, nil
}

// fakeServiceManager implements only Index(); everything else panics if
// exercised, which no code path under test needs.
type fakeServiceManager struct {
	meilisearch.ServiceManager
	idx meilisearch.IndexManager
}

func (f *fakeServiceManager) Index(_ string) meilisearch.IndexManager {
	return f.idx
}

type ctxMarkerKey struct{}

func withMarker(ctx context.Context, v string) context.Context {
	return context.WithValue(ctx, ctxMarkerKey{}, v)
}

func markerOf(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	v, _ := ctx.Value(ctxMarkerKey{}).(string)
	return v
}

// TestMeilisearchDriver_IndexDocuments_BoundsWaitByTimeoutNotJustInterval
// reproduces the HIGH finding: WaitForTask(taskUID, 15*time.Second)'s second
// argument is a polling interval, not a timeout (confirmed by reading the
// vendored meilisearch-go v0.36.2 source -- waitForTask loops on a ticker
// and only stops early via ctx.Done()). A task that never completes used to
// hang IndexDocuments forever. This asserts IndexDocuments now returns
// promptly once the driver's bounded wait context expires.
func TestMeilisearchDriver_IndexDocuments_BoundsWaitByTimeoutNotJustInterval(t *testing.T) {
	fake := newFakeIndexManager()
	fake.waitBlock = true
	sm := &fakeServiceManager{idx: fake}
	d := NewMeilisearchDriverWithClients(sm, nil, "articles")
	d.taskWaitTimeout = 50 * time.Millisecond // keep the test fast

	start := time.Now()
	err := d.IndexDocuments(context.Background(), []SearchDocumentDriver{{ID: "a"}})
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected IndexDocuments to return an error when the task never completes")
	}
	if elapsed > time.Second {
		t.Fatalf("IndexDocuments did not respect the bounded wait timeout, took %v", elapsed)
	}

	waitCtx := fake.ctxFor("WaitForTask")
	if waitCtx == nil {
		t.Fatal("WaitForTaskWithContext was never called")
	}
	if _, ok := waitCtx.Deadline(); !ok {
		t.Fatal("WaitForTaskWithContext must be called with a context carrying a deadline (bounded wait)")
	}
}

// TestMeilisearchDriver_IndexDocuments_PropagatesCallerContext reproduces
// the HIGH finding that AddDocuments (and friends) used the non-context SDK
// variants, so a caller's deadline/cancellation never reached Meilisearch.
func TestMeilisearchDriver_IndexDocuments_PropagatesCallerContext(t *testing.T) {
	fake := newFakeIndexManager()
	sm := &fakeServiceManager{idx: fake}
	d := NewMeilisearchDriverWithClients(sm, nil, "articles")

	ctx := withMarker(context.Background(), "caller-marker")

	if err := d.IndexDocuments(ctx, []SearchDocumentDriver{{ID: "a"}}); err != nil {
		t.Fatalf("IndexDocuments() error = %v", err)
	}

	if got := markerOf(fake.ctxFor("AddDocuments")); got != "caller-marker" {
		t.Fatalf("AddDocumentsWithContext ctx marker = %q, want the caller's context to propagate", got)
	}
	if got := markerOf(fake.ctxFor("WaitForTask")); got != "caller-marker" {
		t.Fatalf("WaitForTaskWithContext ctx marker = %q, want it derived from the caller's context", got)
	}
}

// TestMeilisearchDriver_DeleteDocuments_PropagatesCallerContext covers the
// delete path the same way.
func TestMeilisearchDriver_DeleteDocuments_PropagatesCallerContext(t *testing.T) {
	fake := newFakeIndexManager()
	sm := &fakeServiceManager{idx: fake}
	d := NewMeilisearchDriverWithClients(sm, nil, "articles")

	ctx := withMarker(context.Background(), "caller-marker")

	if err := d.DeleteDocuments(ctx, []string{"a"}); err != nil {
		t.Fatalf("DeleteDocuments() error = %v", err)
	}

	if got := markerOf(fake.ctxFor("DeleteDocuments")); got != "caller-marker" {
		t.Fatalf("DeleteDocumentsWithContext ctx marker = %q, want the caller's context to propagate", got)
	}
}

// TestMeilisearchDriver_Search_PropagatesCallerContext covers the read path,
// which doesn't go through waitForTask at all.
func TestMeilisearchDriver_Search_PropagatesCallerContext(t *testing.T) {
	fake := newFakeIndexManager()
	sm := &fakeServiceManager{idx: fake}
	d := NewMeilisearchDriverWithClients(sm, nil, "articles")

	ctx := withMarker(context.Background(), "caller-marker")

	if _, err := d.Search(ctx, "query", 10); err != nil {
		t.Fatalf("Search() error = %v", err)
	}

	if got := markerOf(fake.ctxFor("Search")); got != "caller-marker" {
		t.Fatalf("SearchWithContext ctx marker = %q, want the caller's context to propagate", got)
	}
}

// TestMeilisearchDriver_RegisterSynonyms_PropagatesCallerContext covers the
// synonyms write path (usecase.registerBatchSynonyms's downstream call).
func TestMeilisearchDriver_RegisterSynonyms_PropagatesCallerContext(t *testing.T) {
	fake := newFakeIndexManager()
	sm := &fakeServiceManager{idx: fake}
	d := NewMeilisearchDriverWithClients(sm, nil, "articles")

	ctx := withMarker(context.Background(), "caller-marker")

	if err := d.RegisterSynonyms(ctx, map[string][]string{"a": {"b"}}); err != nil {
		t.Fatalf("RegisterSynonyms() error = %v", err)
	}

	if got := markerOf(fake.ctxFor("UpdateSynonyms")); got != "caller-marker" {
		t.Fatalf("UpdateSynonymsWithContext ctx marker = %q, want the caller's context to propagate", got)
	}
	if got := markerOf(fake.ctxFor("WaitForTask")); got != "caller-marker" {
		t.Fatalf("WaitForTaskWithContext ctx marker = %q, want it derived from the caller's context", got)
	}
}

// TestMeilisearchDriver_EnsureIndex_PropagatesCallerContext covers the
// startup EnsureIndex path across FetchInfo + the settings update calls.
func TestMeilisearchDriver_EnsureIndex_PropagatesCallerContext(t *testing.T) {
	fake := newFakeIndexManager()
	sm := &fakeServiceManager{idx: fake}
	d := NewMeilisearchDriverWithClients(sm, nil, "articles")

	ctx := withMarker(context.Background(), "caller-marker")

	if err := d.EnsureIndex(ctx); err != nil {
		t.Fatalf("EnsureIndex() error = %v", err)
	}

	for _, call := range []string{
		"FetchInfo",
		"UpdateSearchableAttributes",
		"UpdateFilterableAttributes",
		"UpdateRankingRules",
		"UpdateLocalizedAttributes",
	} {
		if got := markerOf(fake.ctxFor(call)); got != "caller-marker" {
			t.Fatalf("%s ctx marker = %q, want the caller's context to propagate", call, got)
		}
	}
}
