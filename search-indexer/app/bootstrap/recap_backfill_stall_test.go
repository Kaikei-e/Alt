package bootstrap

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"search-indexer/config"
	"search-indexer/domain"
	"search-indexer/usecase"
)

// stalledRecapRepo always returns a full page of documents that share one
// ExecutedAt with HasMore=true, reproducing recap-worker's inclusive, untied
// `since` bound when a single job's tie group is larger than one page: the
// cursor can never advance, so the batch is served again on every fetch. A
// safety cap trips hotLoop and cancels the context if the loop calls it far
// more often than the stall guard should ever allow -- that is the failure the
// guard exists to prevent, and lets this test terminate against buggy code
// instead of spinning forever.
type stalledRecapRepo struct {
	calls     int64
	hotLoop   atomic.Bool
	cancel    context.CancelFunc
	safetyCap int64
}

func (r *stalledRecapRepo) GetIndexableGenres(ctx context.Context, since string, limit int) ([]domain.RecapDocument, bool, error) {
	if atomic.AddInt64(&r.calls, 1) > r.safetyCap {
		r.hotLoop.Store(true)
		if r.cancel != nil {
			r.cancel()
		}
		return nil, false, nil
	}
	// A page of docs all sharing the same ExecutedAt, truncated (HasMore=true).
	docs := []domain.RecapDocument{
		{ID: "job1__a", JobID: "job1", ExecutedAt: "2026-08-22T00:00:05Z", Genre: "a"},
		{ID: "job1__b", JobID: "job1", ExecutedAt: "2026-08-22T00:00:05Z", Genre: "b"},
	}
	return docs, true, nil
}

type noopRecapSearchEngine struct{}

func (noopRecapSearchEngine) EnsureRecapIndex(ctx context.Context) error { return nil }
func (noopRecapSearchEngine) IndexRecapDocuments(ctx context.Context, docs []domain.RecapDocument) error {
	return nil
}
func (noopRecapSearchEngine) SearchRecaps(ctx context.Context, query string, limit int) ([]domain.RecapDocument, int64, error) {
	return nil, 0, nil
}

// TestRunRecapIndexLoop_DoesNotHotSpinOnStalledCursor asserts that Phase 1
// gives up (advancing to Phase 2, which then blocks on its poll interval)
// instead of looping forever on a cursor that never advances. Without the
// stall guard the loop calls GetIndexableGenres tens of thousands of times
// with zero delay and this test trips the safety cap.
func TestRunRecapIndexLoop_DoesNotHotSpinOnStalledCursor(t *testing.T) {
	// Shrink the inter-iteration pause so the (bounded) stall run is fast.
	prevPause := recapBackfillStallPause
	recapBackfillStallPause = time.Millisecond
	t.Cleanup(func() { recapBackfillStallPause = prevPause })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	repo := &stalledRecapRepo{cancel: cancel, safetyCap: 5000}
	uc := usecase.NewIndexRecapsUsecase(repo, noopRecapSearchEngine{})

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		runRecapIndexLoop(ctx, uc)
	}()

	// Give Phase 1 time to exhaust the stall budget and reach Phase 2 (which
	// blocks on config.RecapIndexInterval), then stop the loop.
	time.Sleep(300 * time.Millisecond)
	cancel()

	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("runRecapIndexLoop did not return after context cancel")
	}

	if repo.hotLoop.Load() {
		t.Fatal("Phase 1 hot-spun on a stalled cursor: GetIndexableGenres exceeded the safety cap")
	}
	// Phase 1 should stop after ~recapBackfillMaxStalls no-progress iterations,
	// then Phase 2 makes at most a handful of polls before the interval sleep.
	// A generous bound still catches an unbounded hot loop.
	if calls := atomic.LoadInt64(&repo.calls); calls > recapBackfillMaxStalls+50 {
		t.Fatalf("GetIndexableGenres called %d times; want it bounded by the stall guard (~%d)", calls, recapBackfillMaxStalls)
	}
}

func newFakeMeilisearchForRecapWiring(t *testing.T, failRecapSettings bool, recapRequested *atomic.Bool, onReady func()) *httptest.Server {
	t.Helper()
	var readyOnce sync.Once
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/health":
			_, _ = w.Write([]byte(`{"status":"available"}`))
		case r.URL.Path == "/indexes/articles":
			_, _ = w.Write([]byte(`{"uid":"articles","primaryKey":"id"}`))
		case strings.HasPrefix(r.URL.Path, "/indexes/articles/settings"):
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte(`{"taskUid":1}`))
		case r.URL.Path == "/indexes/recaps":
			if recapRequested != nil {
				recapRequested.Store(true)
			}
			_, _ = w.Write([]byte(`{"uid":"recaps","primaryKey":"id"}`))
		case strings.HasPrefix(r.URL.Path, "/indexes/recaps/settings"):
			if recapRequested != nil {
				recapRequested.Store(true)
			}
			if failRecapSettings {
				readyOnce.Do(onReady)
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`{"message":"recap settings failed","code":"internal","type":"internal"}`))
				return
			}
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte(`{"taskUid":1}`))
		case strings.HasPrefix(r.URL.Path, "/tasks/"):
			readyOnce.Do(onReady)
			_, _ = w.Write([]byte(`{"uid":1,"status":"succeeded"}`))
		default:
			_, _ = w.Write([]byte(`{}`))
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func setRecapBootstrapEnv(t *testing.T, meiliURL string, recapWorkerURL string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "search-indexer"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	certPath := filepath.Join(dir, "cert.pem")
	keyPath := filepath.Join(dir, "key.pem")
	if err := os.WriteFile(certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("MTLS_CERT_FILE", certPath)
	t.Setenv("MTLS_KEY_FILE", keyPath)
	t.Setenv("MTLS_CA_FILE", certPath)
	t.Setenv("OTEL_ENABLED", "false")
	t.Setenv("BACKEND_API_URL", meiliURL)
	t.Setenv("USER_JWT_INTROSPECTION_URL", "https://auth-hub:9443/internal/token/introspect")
	t.Setenv("MEILISEARCH_HOST", meiliURL)
	t.Setenv("MEILISEARCH_API_KEY", "test-master-key")
	t.Setenv("MEILISEARCH_SEARCH_API_KEY", "test-search-key")
	t.Setenv("INFERENCE_AUTH", "disabled")
	t.Setenv("REDIS_AUTH", "disabled")
	t.Setenv("CONSUMER_ENABLED", "false")
	t.Setenv("MTLS_ENFORCE", "false")
	t.Setenv("MTLS_LISTEN", "false")
	t.Setenv("OPS_LISTEN", "127.0.0.1:0")

	prevEmbedder := config.MeiliHybridEmbedder
	config.MeiliHybridEmbedder = ""
	t.Cleanup(func() { config.MeiliHybridEmbedder = prevEmbedder })

	prevHTTPAddr := config.HTTPAddr
	config.HTTPAddr = "127.0.0.1:0"
	t.Cleanup(func() { config.HTTPAddr = prevHTTPAddr })

	prevRecapURL := config.RecapWorkerURL
	config.RecapWorkerURL = recapWorkerURL
	t.Cleanup(func() { config.RecapWorkerURL = prevRecapURL })
}

func TestRun_EnsureRecapIndexFailureAbortsStartup(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	srv := newFakeMeilisearchForRecapWiring(t, true, nil, func() {
		time.AfterFunc(50*time.Millisecond, cancel)
	})
	setRecapBootstrapEnv(t, srv.URL, srv.URL)

	err := Run(ctx)
	if err == nil {
		t.Fatal("Run() = nil, want startup error when EnsureRecapIndex fails")
	}
	if !strings.Contains(err.Error(), "EnsureRecapIndex") && !strings.Contains(err.Error(), "recap") {
		t.Fatalf("Run() error = %v, want recap index failure", err)
	}
}

func TestRun_RecapWorkerURLUnsetFailsStartup(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	srv := newFakeMeilisearchForRecapWiring(t, false, nil, func() {
		time.AfterFunc(50*time.Millisecond, cancel)
	})
	setRecapBootstrapEnv(t, srv.URL, "")

	err := Run(ctx)
	if err == nil {
		t.Fatal("Run() = nil, want startup error when RECAP_WORKER_URL is unset")
	}
	if !strings.Contains(err.Error(), "RECAP_WORKER_URL") {
		t.Fatalf("Run() error = %v, want error naming RECAP_WORKER_URL", err)
	}
}

func TestRun_RecapWorkerURLExplicitlyDisabledSucceeds(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var recapRequested atomic.Bool
	srv := newFakeMeilisearchForRecapWiring(t, true, &recapRequested, func() {
		time.AfterFunc(50*time.Millisecond, cancel)
	})
	setRecapBootstrapEnv(t, srv.URL, "disabled")

	if err := Run(ctx); err != nil {
		t.Fatalf("Run() = %v, want nil when RECAP_WORKER_URL=disabled", err)
	}
	if recapRequested.Load() {
		t.Fatal("EnsureRecapIndex was called despite RECAP_WORKER_URL=disabled")
	}
}
