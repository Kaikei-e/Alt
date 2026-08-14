package driver

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/meilisearch/meilisearch-go"
)

// stallingSearchIndex holds every SearchWithContext open until release is
// closed or the caller's context dies. That is the only way to keep a
// singleflight window provably open while a second caller joins it -- a
// non-blocking fake would finish the flight before the follower arrives and
// the test would silently stop exercising the coalescing path.
type stallingSearchIndex struct {
	meilisearch.IndexManager

	entered chan context.Context // one value per SearchWithContext invocation
	release chan struct{}

	mu    sync.Mutex
	calls int
}

func newStallingSearchIndex() *stallingSearchIndex {
	return &stallingSearchIndex{
		entered: make(chan context.Context, 8),
		release: make(chan struct{}),
	}
}

func (s *stallingSearchIndex) SearchWithContext(ctx context.Context, _ string, _ *meilisearch.SearchRequest) (*meilisearch.SearchResponse, error) {
	s.mu.Lock()
	s.calls++
	s.mu.Unlock()
	s.entered <- ctx

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-s.release:
		return &meilisearch.SearchResponse{
			Hits:               meilisearch.Hits{{"id": json.RawMessage(`"a1"`)}},
			EstimatedTotalHits: 1,
			ProcessingTimeMs:   3,
		}, nil
	}
}

func (s *stallingSearchIndex) callCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

// TestMeilisearchDriver_Search_LeaderCancelDoesNotFailOtherCallers reproduces
// the production fan-out shape that singleflight exists for: RAG fires several
// identical SearchArticles inside 300ms, the first caller's HTTP client hangs
// up, and everybody else -- whose own request is still very much alive -- used
// to get that leader's context.Canceled back as a 500. The shared call runs
// under the leader's context, so one impatient caller took the whole fan-out
// down with it.
func TestMeilisearchDriver_Search_LeaderCancelDoesNotFailOtherCallers(t *testing.T) {
	idx := newStallingSearchIndex()
	d := NewMeilisearchDriverWithClients(&fakeServiceManager{idx: idx}, nil, "articles")

	leaderCtx, cancelLeader := context.WithCancel(context.Background())
	defer cancelLeader()

	leaderErr := make(chan error, 1)
	go func() {
		_, err := d.Search(leaderCtx, "vance", 10)
		leaderErr <- err
	}()

	// Once the leader is parked inside SearchWithContext its flight is open,
	// so any caller using the same cache key must join it instead of starting
	// its own.
	select {
	case <-idx.entered:
	case <-time.After(2 * time.Second):
		t.Fatal("leader never reached SearchWithContext")
	}

	type searchResult struct {
		docs []SearchDocumentDriver
		err  error
	}
	follower := make(chan searchResult, 1)
	go func() {
		docs, err := d.Search(context.Background(), "vance", 10)
		follower <- searchResult{docs: docs, err: err}
	}()

	// singleflight exposes no "the follower has joined" hook, so give it a
	// generous slice of wall clock -- same approach as the sibling
	// cancellation test in meilisearch_singleflight_test.go.
	time.Sleep(200 * time.Millisecond)

	cancelLeader()

	select {
	case err := <-leaderErr:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("leader: got %v, want context.Canceled after cancelling its own request", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("leader: did not return after cancellation")
	}

	// The follower must now be searching again under its own live context. If
	// it returns before that, it inherited the leader's cancellation.
	select {
	case got := <-follower:
		t.Fatalf("follower returned err=%v (docs=%d) on the leader's cancellation; a caller whose own context is alive must still get results", got.err, len(got.docs))
	case <-idx.entered:
	case <-time.After(2 * time.Second):
		t.Fatal("follower never reached SearchWithContext under a live context")
	}
	close(idx.release)

	select {
	case got := <-follower:
		if got.err != nil {
			t.Fatalf("follower: got err=%v, want the search to survive the leader's cancellation", got.err)
		}
		if len(got.docs) != 1 || got.docs[0].ID != "a1" {
			t.Fatalf("follower: docs = %+v, want the single stub hit", got.docs)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("follower: did not return")
	}

	if got := idx.callCount(); got != 2 {
		t.Errorf("SearchWithContext calls = %d, want 2 (the doomed leader flight plus one re-lead)", got)
	}
}

// TestMeilisearchDriver_Search_RealFailureIsNotRetried pins the other half of
// the failover rule: a genuine Meilisearch failure must surface to every
// waiter as-is. Re-running the search on non-cancellation errors would turn a
// broken engine into an N-times amplified load.
func TestMeilisearchDriver_Search_RealFailureIsNotRetried(t *testing.T) {
	boom := errors.New("index not found")
	idx := &failingSearchIndex{err: boom}
	d := NewMeilisearchDriverWithClients(&fakeServiceManager{idx: idx}, nil, "articles")

	_, err := d.Search(context.Background(), "vance", 10)
	if !errors.Is(err, boom) {
		t.Fatalf("got %v, want the underlying engine error", err)
	}
	if got := idx.callCount(); got != 1 {
		t.Errorf("SearchWithContext calls = %d, want 1 (engine failures must not be retried)", got)
	}
}

// failingSearchIndex fails every search with a fixed non-context error.
type failingSearchIndex struct {
	meilisearch.IndexManager

	err error

	mu    sync.Mutex
	calls int
}

func (s *failingSearchIndex) SearchWithContext(_ context.Context, _ string, _ *meilisearch.SearchRequest) (*meilisearch.SearchResponse, error) {
	s.mu.Lock()
	s.calls++
	s.mu.Unlock()
	return nil, s.err
}

func (s *failingSearchIndex) callCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}
