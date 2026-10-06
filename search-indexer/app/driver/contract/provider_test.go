//go:build contract

// Provider verification for search-indexer.
//
// Replays the Pact files published by search-indexer's consumers against the
// real rest.Handler and the real connectv2.CreateConnectServer mux, backed by
// fake port.SearchEngine / port.RecapSearchEngine implementations so no
// Meilisearch instance is required. The real user-auth usecase consumes fixed
// introspection fixtures. Pact replay swaps a consumer's placeholder JWT for a
// live fixture token but never adds a missing one; request payloads,
// validation and response mapping remain unchanged.
package contract

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
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

const contractUserID = "00000000-0000-0000-0000-000000000001"

// These are test-only, signed JWTs, mapped to the subjects represented by
// existing consumer fixtures. Unknown tokens never receive active claims.
var (
	contractExpiry = time.Now().Add(5 * time.Minute).Unix()
	contractTokens = makeContractTokens()
)

func makeContractTokens() map[string]string {
	tokens := make(map[string]string)
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	for _, sub := range []string{contractUserID, "user-1"} {
		claims := fmt.Sprintf(`{"sub":%q,"tenant_id":"default","iss":"auth-hub","exp":%d}`, sub, contractExpiry)
		unsigned := header + "." + base64.RawURLEncoding.EncodeToString([]byte(claims))
		mac := hmac.New(sha256.New, []byte("pact-fixture-signing-key-only-32-bytes"))
		_, _ = mac.Write([]byte(unsigned))
		tokens[sub] = unsigned + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	}
	return tokens
}

type contractAuthHub struct{}

func (*contractAuthHub) IntrospectToken(_ context.Context, token string) (*port.TokenIntrospection, error) {
	for sub, fixture := range contractTokens {
		if token == fixture {
			return &port.TokenIntrospection{Active: time.Now().Unix() < contractExpiry, Sub: sub, TenantID: "default", Exp: contractExpiry}, nil
		}
	}
	return nil, fmt.Errorf("unknown contract token")
}

// contractAuthFilter swaps the credential a consumer sends for a live fixture
// token minted for the interaction's user_id. Pact files can only hold a
// placeholder JWT, but the header's presence is the contract: a request without
// one is passed through untouched and must fail with 401.
func contractAuthFilter(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hasAuthorization := len(r.Header.Values("Authorization")) > 0
		hasBackendToken := len(r.Header.Values("X-Alt-Backend-Token")) > 0
		if !hasAuthorization && !hasBackendToken {
			next.ServeHTTP(w, r)
			return
		}
		userID := r.URL.Query().Get("user_id")
		if userID == "" && r.Body != nil && strings.HasSuffix(r.URL.Path, "/SearchArticles") {
			body, err := io.ReadAll(r.Body)
			if err != nil {
				http.Error(w, "read contract request", http.StatusBadRequest)
				return
			}
			r.Body = io.NopCloser(strings.NewReader(string(body)))
			var payload struct {
				UserID string `json:"userId"`
			}
			if err := json.Unmarshal(body, &payload); err == nil {
				userID = payload.UserID
			}
		}
		if token, ok := contractTokens[userID]; ok {
			if hasAuthorization {
				r.Header.Set("Authorization", "Bearer "+token)
			}
			if hasBackendToken {
				r.Header.Set("X-Alt-Backend-Token", token)
			}
		}
		next.ServeHTTP(w, r)
	})
}

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
	authUsecase := usecase.NewAuthUsecase(&contractAuthHub{})
	restHandler := rest.NewHandler(searchByUserUsecase, authUsecase)

	// Connect-RPC mounts the real server (SearchArticles + SearchRecaps +
	// /health) so the pact replay reaches connectv2.CreateConnectServer
	// end-to-end, including its request validation, instead of a stub that
	// only mirrors the response shape.
	searchRecapsUsecase := usecase.NewSearchRecapsUsecase(&fakeContractRecapSearchEngine{})
	rlCfg := config.RateLimitConfig{RequestsPerSecond: 1000, Burst: 1000}
	connectServer := connectv2.CreateConnectServer(searchByUserUsecase, searchRecapsUsecase, authUsecase, rlCfg)

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
		"search has indexed articles": func(setup bool, s models.ProviderState) (models.ProviderStateResponse, error) {
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
		// The "Iran tensions 2026" fixture in fakeContractSearchEngine carries
		// the published_at this state promises.
		"search-indexer has articles with published_at metadata indexed": func(setup bool, s models.ProviderState) (models.ProviderStateResponse, error) {
			emptyResultState.Store(false)
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
		RequestFilter:      contractAuthFilter,
	}

	if brokerURL := os.Getenv("PACT_BROKER_BASE_URL"); brokerURL != "" {
		verifyRequest.PactFiles = nil
		verifyRequest.BrokerURL = brokerURL
		verifyRequest.BrokerUsername = os.Getenv("PACT_BROKER_USERNAME")
		verifyRequest.BrokerPassword = os.Getenv("PACT_BROKER_PASSWORD")
		lockstep := lockstepConsumers(os.Getenv("PACT_LOCKSTEP_CONSUMERS"))
		for _, s := range consumerSelectors(searchIndexerConsumers, lockstep) {
			verifyRequest.ConsumerVersionSelectors = append(verifyRequest.ConsumerVersionSelectors, &provider.ConsumerVersionSelector{
				Consumer:           s.Consumer,
				MainBranch:         s.MainBranch,
				DeployedOrReleased: s.DeployedOrReleased,
			})
			if s.MainBranch && lockstep[s.Consumer] {
				t.Logf("lockstep: not verifying the production-deployed pact of %s (PACT_LOCKSTEP_CONSUMERS)", s.Consumer)
			}
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
	resp2, err := providerGET(fmt.Sprintf("http://127.0.0.1:%d/v1/search?q=LLM&user_id=%s&published_after=invalid-date", port, contractUserID), contractTokens[contractUserID])
	require.NoError(t, err)
	defer func() { _ = resp2.Body.Close() }()
	require.Equal(t, http.StatusBadRequest, resp2.StatusCode)

	// Inverted date filter rejected
	resp3, err := providerGET(fmt.Sprintf("http://127.0.0.1:%d/v1/search?q=LLM&user_id=%s&published_after=2026-04-20T00:00:00Z&published_before=2026-04-10T00:00:00Z", port, contractUserID), contractTokens[contractUserID])
	require.NoError(t, err)
	defer func() { _ = resp3.Body.Close() }()
	require.Equal(t, http.StatusBadRequest, resp3.StatusCode)
}

func providerGET(url, token string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return http.DefaultClient.Do(req)
}

func TestProviderStub_RequiresMatchingUserAuthorization(t *testing.T) {
	port := startProviderStub(t)
	url := fmt.Sprintf("http://127.0.0.1:%d/v1/search?q=LLM&user_id=%s", port, contractUserID)
	for _, tc := range []struct {
		name   string
		token  string
		status int
	}{
		{"missing", "", http.StatusUnauthorized},
		{"invalid", "invalid-token", http.StatusForbidden},
		{"wrong_subject", contractTokens["user-1"], http.StatusForbidden},
		{"matching_subject", contractTokens[contractUserID], http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := providerGET(url, tc.token)
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()
			require.Equal(t, tc.status, resp.StatusCode)
		})
	}
}

func TestProviderStub_ConnectRequiresMatchingUserAuthorization(t *testing.T) {
	port := startProviderStub(t)
	url := fmt.Sprintf("http://127.0.0.1:%d/services.search.v2.SearchService/SearchArticles", port)
	payload := fmt.Sprintf(`{"query":"LLM","userId":%q,"limit":20}`, contractUserID)
	for _, tc := range []struct {
		name   string
		token  string
		status int
	}{
		{"missing", "", http.StatusUnauthorized},
		{"invalid", "invalid-token", http.StatusForbidden},
		{"wrong_subject", contractTokens["user-1"], http.StatusForbidden},
		{"matching_subject", contractTokens[contractUserID], http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(payload))
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Connect-Protocol-Version", "1")
			if tc.token != "" {
				req.Header.Set("Authorization", "Bearer "+tc.token)
			}
			resp, err := http.DefaultClient.Do(req)
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()
			require.Equal(t, tc.status, resp.StatusCode)
		})
	}
}

// A consumer that drops the user JWT must fail verification, so the filter
// only swaps a credential the request already carries for a live fixture token;
// it never supplies one that is missing.
func TestContractAuthFilterReplacesPresentCredentials(t *testing.T) {
	for _, tc := range []struct {
		header string
		want   string
	}{
		{"Authorization", "Bearer " + contractTokens[contractUserID]},
		{"X-Alt-Backend-Token", contractTokens[contractUserID]},
	} {
		t.Run(tc.header, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, "/v1/search?user_id="+contractUserID, nil)
			require.NoError(t, err)
			req.Header.Set(tc.header, "pact-header.pact-claims.pact-signature")
			called := false
			contractAuthFilter(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
				called = true
				require.Equal(t, []string{tc.want}, r.Header.Values(tc.header))
			})).ServeHTTP(nil, req)
			require.True(t, called)
		})
	}
}

func TestContractAuthFilterDoesNotAddMissingCredentials(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, "/v1/search?user_id="+contractUserID, nil)
	require.NoError(t, err)
	called := false
	contractAuthFilter(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		called = true
		require.Empty(t, r.Header.Values("Authorization"))
		require.Empty(t, r.Header.Values("X-Alt-Backend-Token"))
	})).ServeHTTP(nil, req)
	require.True(t, called)
}

func TestContractAuthFilterPreservesConnectPayload(t *testing.T) {
	const payload = `{"query":"LLM","userId":"user-1","limit":20}`
	req, err := http.NewRequest(http.MethodPost, "/services.search.v2.SearchService/SearchArticles", strings.NewReader(payload))
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer pact-header.pact-claims.pact-signature")
	called := false
	contractAuthFilter(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		called = true
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		require.Equal(t, payload, string(body))
		require.Equal(t, "Bearer "+contractTokens["user-1"], r.Header.Get("Authorization"))
	})).ServeHTTP(nil, req)
	require.True(t, called)
}
