//go:build contract

// Package contract contains Provider Verification tests for rag-orchestrator.
// It verifies that rag-orchestrator's HTTP endpoints satisfy consumer contracts,
// notably alt-backend's contract for article upsert, document owner backfill,
// and retrieve context scoping.
package contract

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"testing"
	"time"

	"rag-orchestrator/internal/adapter/rag_http"
	"rag-orchestrator/internal/adapter/rag_http/openapi"
	"rag-orchestrator/internal/middleware"
	"rag-orchestrator/internal/usecase"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/pact-foundation/pact-go/v2/models"
	"github.com/pact-foundation/pact-go/v2/provider"
	"github.com/stretchr/testify/require"
)

type providerFakeIndexUsecase struct{}

func (f *providerFakeIndexUsecase) Upsert(_ context.Context, _, _, _, _, _ string) error {
	return nil
}

func (f *providerFakeIndexUsecase) Delete(_ context.Context, _ string) error {
	return nil
}

func (f *providerFakeIndexUsecase) BackfillOwners(_ context.Context, items []usecase.OwnerBackfillItem) (usecase.OwnerBackfillResult, error) {
	return usecase.OwnerBackfillResult{
		Updated:    int64(len(items)),
		AlreadySet: 0,
		NotFound:   0,
	}, nil
}

type providerFakeRetrieveUsecase struct{}

func (f *providerFakeRetrieveUsecase) Execute(_ context.Context, _ usecase.RetrieveContextInput) (*usecase.RetrieveContextOutput, error) {
	return &usecase.RetrieveContextOutput{
		Contexts: []usecase.ContextItem{
			{
				ChunkID:         uuid.MustParse("11111111-2222-3333-4444-555555555555"),
				ChunkText:       "sample chunk text",
				URL:             "https://example.com/article",
				Title:           "Test Article",
				PublishedAt:     "2026-09-20T10:00:00Z",
				Score:           0.95,
				DocumentVersion: 1,
			},
		},
	}, nil
}

const providerContractToken = "test-rag-api-token-minimum-24-characters-long"

func startRagProviderServer(t *testing.T) int {
	t.Helper()

	e := echo.New()
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))

	apiAuthMW := middleware.NewAPIAuthMiddleware(providerContractToken, true, logger)
	e.Use(apiAuthMW.EchoMiddleware())

	indexUC := &providerFakeIndexUsecase{}
	retrieveUC := &providerFakeRetrieveUsecase{}

	handler := rag_http.NewHandler(retrieveUC, nil, indexUC, nil, nil, logger)
	openapi.RegisterHandlers(e, handler)

	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	port := l.Addr().(*net.TCPAddr).Port

	server := &http.Server{
		Handler: e,
	}

	go func() {
		_ = server.Serve(l)
	}()

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
		_ = l.Close()
	})

	return port
}

const (
	altBackendRagPactCandidate1 = "../../../../alt-backend/pacts/alt-backend-rag-orchestrator.json"
	altBackendRagPactCandidate2 = "../../../../pacts/alt-backend-rag-orchestrator.json"
)

func resolveRagProviderPact() string {
	for _, path := range []string{altBackendRagPactCandidate1, altBackendRagPactCandidate2} {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return ""
}

func TestVerifyAltBackendRagProviderContracts(t *testing.T) {
	brokerURL := os.Getenv("PACT_BROKER_BASE_URL")
	pactFile := resolveRagProviderPact()

	if brokerURL == "" && pactFile == "" {
		t.Skipf("no broker URL set and local pact not found at %s or %s", altBackendRagPactCandidate1, altBackendRagPactCandidate2)
	}

	port := startRagProviderServer(t)

	stateHandlers := models.StateHandlers{
		"rag-orchestrator accepts article upserts with owner": func(setup bool, s models.ProviderState) (models.ProviderStateResponse, error) {
			return nil, nil
		},
		"rag-orchestrator accepts document owner backfill": func(setup bool, s models.ProviderState) (models.ProviderStateResponse, error) {
			return nil, nil
		},
		"rag-orchestrator retrieves context for query with owner": func(setup bool, s models.ProviderState) (models.ProviderStateResponse, error) {
			return nil, nil
		},
	}

	verifyRequest := provider.VerifyRequest{
		Provider:           "rag-orchestrator",
		ProviderBaseURL:    fmt.Sprintf("http://127.0.0.1:%d", port),
		StateHandlers:      stateHandlers,
		FailIfNoPactsFound: true,
	}

	if brokerURL != "" {
		verifyRequest.BrokerURL = brokerURL
		verifyRequest.BrokerUsername = os.Getenv("PACT_BROKER_USERNAME")
		verifyRequest.BrokerPassword = os.Getenv("PACT_BROKER_PASSWORD")
		verifyRequest.ConsumerVersionSelectors = []provider.Selector{
			&provider.ConsumerVersionSelector{Consumer: "alt-backend", MainBranch: true},
			&provider.ConsumerVersionSelector{Consumer: "alt-backend", DeployedOrReleased: true},
		}
		if ver := os.Getenv("PACT_PROVIDER_VERSION"); ver != "" {
			verifyRequest.ProviderVersion = ver
			verifyRequest.PublishVerificationResults = true
		}
		if branch := os.Getenv("PACT_PROVIDER_BRANCH"); branch != "" {
			verifyRequest.ProviderBranch = branch
		}
	} else {
		verifyRequest.PactFiles = []string{pactFile}
	}

	verifier := provider.NewVerifier()
	err := verifier.VerifyProvider(t, verifyRequest)
	require.NoError(t, err)
}
