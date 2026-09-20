//go:build contract

// Provider verification for search-indexer.
//
// Replays the Pact files published by search-indexer's consumers against the
// real rest.Handler and the real connectv2.CreateConnectServer mux, backed by
// fake port.SearchEngine / port.RecapSearchEngine implementations so no
// Meilisearch instance is required. Authentication is established at the TLS
// transport layer (mTLS peer-identity allowlist); the replay does not gate on
// X-Service-Token because it does not present a TLS peer.
package contract

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pact-foundation/pact-go/v2/models"
	"github.com/pact-foundation/pact-go/v2/provider"
	"github.com/stretchr/testify/require"

	"search-indexer/config"
	connectv2 "search-indexer/connect/v2"
	"search-indexer/domain"
	"search-indexer/logger"
	"search-indexer/port"
	"search-indexer/rest"
	"search-indexer/usecase"
)

// emptyResultState is toggled by the "search-indexer has no matching articles"
// provider state so the REST stub returns empty hits for that interaction.
var emptyResultState atomic.Bool

const (
	providerPactDirAltBackend = "../../../../alt-backend/pacts"
	providerPactDirRAG        = "../../../../rag-orchestrator/pacts"
	providerPactDirRoot       = "../../../../pacts"
)

// fakeContractSearchEngine backs both the REST and Connect-RPC endpoints.
// SearchByUserID returns fixture documents keyed by the exact query text the
// acolyte-orchestrator pact asserts byte-for-byte (no "type" matchers on
// those two interactions); every other query falls back to the generic
// "An LLM primer" fixture the rag-orchestrator/alt-backend pacts only
// type-match.
type fakeContractSearchEngine struct{}

func (f *fakeContractSearchEngine) IndexDocuments(ctx context.Context, docs []domain.SearchDocument) error {
	return nil
}
func (f *fakeContractSearchEngine) DeleteDocuments(ctx context.Context, ids []string) error {
	return nil
}
func (f *fakeContractSearchEngine) SearchByUserID(ctx context.Context, query string, userID string, limit int) ([]domain.SearchDocument, error) {
	if emptyResultState.Load() {
		return []domain.SearchDocument{}, nil
	}
	switch query {
	case "Iran tensions 2026":
		return []domain.SearchDocument{
			{
				ID:          "article-010",
				Title:       "Iran strait tensions escalate",
				Content:     "Recent events have intensified the standoff...",
				Tags:        []string{"iran", "geopolitics"},
				Language:    "en",
				Score:       0.91,
				PublishedAt: time.Date(2026, 4, 18, 9, 0, 0, 0, time.UTC),
			},
		}, nil
	case "AI market trends 2026":
		return []domain.SearchDocument{
			{
				ID:       "article-001",
				Title:    "AI Market Overview 2026",
				Content:  "The artificial intelligence market continues to expand...",
				Tags:     []string{"AI", "market", "2026"},
				Language: "en",
				Score:    0.85,
			},
			{
				ID:       "article-002",
				Title:    "AI市場 2026年展望",
				Content:  "人工知能市場は拡大を続けている...",
				Tags:     []string{"AI", "市場"},
				Language: "ja",
				Score:    0.78,
			},
		}, nil
	}
	return []domain.SearchDocument{
		{
			ID:      "article-1",
			Title:   "An LLM primer",
			Content: "Some content",
			Tags:    []string{"ai"},
		},
	}, nil
}
func (f *fakeContractSearchEngine) SearchByUserIDWithPagination(ctx context.Context, query string, userID string, offset, limit int64) ([]domain.SearchDocument, int64, error) {
	docs, err := f.SearchByUserID(ctx, query, userID, int(limit))
	return docs, int64(len(docs)), err
}
func (f *fakeContractSearchEngine) SearchByUserIDWithDateFilter(ctx context.Context, query string, userID string, publishedAfter, publishedBefore *time.Time, limit int) ([]domain.SearchDocument, error) {
	return f.SearchByUserID(ctx, query, userID, limit)
}
func (f *fakeContractSearchEngine) EnsureIndex(ctx context.Context) error {
	return nil
}
func (f *fakeContractSearchEngine) RegisterSynonyms(ctx context.Context, synonyms map[string][]string) error {
	return nil
}
func (f *fakeContractSearchEngine) PruneTaskHistory(ctx context.Context, olderThan time.Duration) error {
	return nil
}

var _ port.SearchEngine = (*fakeContractSearchEngine)(nil)

// fakeContractRecapSearchEngine backs the SearchRecaps Connect-RPC endpoint.
// The alt-backend pact only type-matches the recap fields, so one canned
// fixture covers both the by-tag and by-query request shapes.
type fakeContractRecapSearchEngine struct{}

func (f *fakeContractRecapSearchEngine) EnsureRecapIndex(ctx context.Context) error {
	return nil
}
func (f *fakeContractRecapSearchEngine) IndexRecapDocuments(ctx context.Context, docs []domain.RecapDocument) error {
	return nil
}
func (f *fakeContractRecapSearchEngine) SearchRecaps(ctx context.Context, query string, limit int) ([]domain.RecapDocument, int64, error) {
	docs := []domain.RecapDocument{
		{
			ID:         "job-1__technology",
			JobID:      "job-1",
			ExecutedAt: "2026-04-10T00:00:00Z",
			WindowDays: 7,
			Genre:      "technology",
			Summary:    "weekly recap",
			TopTerms:   []string{"ai"},
			Tags:       []string{"technology"},
			Bullets:    []string{"bullet"},
		},
	}
	return docs, int64(len(docs)), nil
}

var _ port.RecapSearchEngine = (*fakeContractRecapSearchEngine)(nil)

// startProviderStub starts an HTTP server that mounts the real rest.Handler
// and the real connectv2.CreateConnectServer mux, both backed by fakes, so
// pact replay exercises the actual routing, validation (e.g. the Connect
// handler's "user_id is required" guard) and response mapping instead of a
// hand-written double.
func startProviderStub(t *testing.T) int {
	t.Helper()
	logger.Init()

	// REST /v1/search uses the real rest.Handler with a fake search engine
	// returning canned hits. emptyResultState toggles empty-hits responses for
	// the acolyte "no matching articles" provider state.
	searchByUserUsecase := usecase.NewSearchByUserUsecase(&fakeContractSearchEngine{})
	restHandler := rest.NewHandler(searchByUserUsecase)

	// Connect-RPC mounts the real server (SearchArticles + SearchRecaps +
	// /health) so the pact replay reaches connectv2.CreateConnectServer
	// end-to-end, including its request validation, instead of a stub that
	// only mirrors the response shape.
	searchRecapsUsecase := usecase.NewSearchRecapsUsecase(&fakeContractRecapSearchEngine{})
	rlCfg := config.RateLimitConfig{RequestsPerSecond: 1000, Burst: 1000}
	connectServer := connectv2.CreateConnectServer(searchByUserUsecase, searchRecapsUsecase, rlCfg)

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/search", restHandler.SearchArticles)
	mux.Handle("/", connectServer)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = ln.Close() })

	go func() { _ = http.Serve(ln, mux) }()

	return ln.Addr().(*net.TCPAddr).Port
}

func findProviderPacts(t *testing.T) []string {
	t.Helper()
	// All three search-indexer consumers are now verified: rag-orchestrator,
	// alt-backend, and acolyte-orchestrator.
	candidates := []string{
		filepath.Join(providerPactDirRAG, "rag-orchestrator-search-indexer.json"),
		filepath.Join(providerPactDirAltBackend, "alt-backend-search-indexer.json"),
		filepath.Join(providerPactDirRoot, "acolyte-orchestrator-search-indexer.json"),
	}
	found := make([]string, 0, len(candidates))
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			found = append(found, c)
		} else {
			t.Logf("skipping missing pact: %s", c)
		}
	}
	return found
}

func TestVerifySearchIndexerProviderContracts(t *testing.T) {
	pactFiles := findProviderPacts(t)
	if len(pactFiles) == 0 {
		t.Skip("no consumer pacts found — run consumer tests first")
	}

	port := startProviderStub(t)

	stateHandlers := models.StateHandlers{
		"a service token is configured and search has indexed articles": func(setup bool, s models.ProviderState) (models.ProviderStateResponse, error) {
			emptyResultState.Store(false)
			return nil, nil
		},
		"a service token is configured and articles are indexed": func(setup bool, s models.ProviderState) (models.ProviderStateResponse, error) {
			emptyResultState.Store(false)
			return nil, nil
		},
		"a service token is configured and recap jobs are indexed under a tag": func(setup bool, s models.ProviderState) (models.ProviderStateResponse, error) {
			emptyResultState.Store(false)
			return nil, nil
		},
		"search-indexer has indexed articles": func(setup bool, s models.ProviderState) (models.ProviderStateResponse, error) {
			emptyResultState.Store(false)
			return nil, nil
		},
		"search-indexer has no matching articles": func(setup bool, s models.ProviderState) (models.ProviderStateResponse, error) {
			emptyResultState.Store(setup)
			return nil, nil
		},
		"search-indexer has indexed articles and a service token is configured": func(setup bool, s models.ProviderState) (models.ProviderStateResponse, error) {
			emptyResultState.Store(false)
			return nil, nil
		},
		"search-indexer has no matching articles and a service token is configured": func(setup bool, s models.ProviderState) (models.ProviderStateResponse, error) {
			emptyResultState.Store(setup)
			return nil, nil
		},
	}

	// FailIfNoPactsFound turns "the Broker held nothing for this provider" from
	// a warning into a failure. Without it the verifier logs `Ignoring no pacts
	// error` and reports a green verification of zero interactions, so a Broker
	// that never received these pacts — or selectors that resolve to nothing —
	// is indistinguishable from a provider that satisfies every consumer.
	// The file-mode branch always supplies a pact, because the test skips
	// earlier when none exists, so the flag can only fire against the Broker.
	verifyRequest := provider.VerifyRequest{
		Provider:           "search-indexer",
		ProviderBaseURL:    fmt.Sprintf("http://127.0.0.1:%d", port),
		PactFiles:          pactFiles,
		StateHandlers:      stateHandlers,
		FailIfNoPactsFound: true,
	}

	if brokerURL := os.Getenv("PACT_BROKER_BASE_URL"); brokerURL != "" {
		verifyRequest.PactFiles = nil
		verifyRequest.BrokerURL = brokerURL
		verifyRequest.BrokerUsername = os.Getenv("PACT_BROKER_USERNAME")
		verifyRequest.BrokerPassword = os.Getenv("PACT_BROKER_PASSWORD")
		verifyRequest.ConsumerVersionSelectors = []provider.Selector{
			&provider.ConsumerVersionSelector{Consumer: "rag-orchestrator", MainBranch: true},
			&provider.ConsumerVersionSelector{Consumer: "rag-orchestrator", DeployedOrReleased: true},
			&provider.ConsumerVersionSelector{Consumer: "alt-backend", MainBranch: true},
			&provider.ConsumerVersionSelector{Consumer: "alt-backend", DeployedOrReleased: true},
			&provider.ConsumerVersionSelector{Consumer: "acolyte-orchestrator", MainBranch: true},
			&provider.ConsumerVersionSelector{Consumer: "acolyte-orchestrator", DeployedOrReleased: true},
		}
		if ver := os.Getenv("PACT_PROVIDER_VERSION"); ver != "" {
			verifyRequest.ProviderVersion = ver
			verifyRequest.PublishVerificationResults = true
		}
		if branch := os.Getenv("PACT_PROVIDER_BRANCH"); branch != "" {
			verifyRequest.ProviderBranch = branch
		}
		// Pending pacts: new contracts warn instead of breaking the provider
		// build until they have been verified at least once.
		if os.Getenv("PACT_DISABLE_PENDING") != "true" {
			verifyRequest.EnablePending = true
		}
		if since := os.Getenv("PACT_INCLUDE_WIP_SINCE"); since != "" {
			if t, err := time.Parse(time.RFC3339, since); err == nil {
				verifyRequest.IncludeWIPPactsSince = &t
			}
		}
	}

	verifier := provider.NewVerifier()
	err := verifier.VerifyProvider(t, verifyRequest)
	require.NoError(t, err)
}

func TestProviderStub_RejectsInvalidRequests(t *testing.T) {
	port := startProviderStub(t)

	// Missing user_id rejected
	resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/v1/search?q=LLM", port))
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)

	// Malformed date filter rejected
	resp2, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/v1/search?q=LLM&user_id=u1&published_after=invalid-date", port))
	require.NoError(t, err)
	defer resp2.Body.Close()
	require.Equal(t, http.StatusBadRequest, resp2.StatusCode)

	// Inverted date filter rejected
	resp3, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/v1/search?q=LLM&user_id=u1&published_after=2026-04-20T00:00:00Z&published_before=2026-04-10T00:00:00Z", port))
	require.NoError(t, err)
	defer resp3.Body.Close()
	require.Equal(t, http.StatusBadRequest, resp3.StatusCode)
}
