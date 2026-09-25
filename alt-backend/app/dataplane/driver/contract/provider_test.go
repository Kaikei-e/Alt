//go:build contract

// Package contract contains provider verification tests for the data plane.
//
// Every consumer of services.datahub.v1.DataHubService is verified here, plus the
// browser-facing services alt-butterfly-facade proxies:
//
//	consumer            pacticipant it names as provider
//	recap-worker        alt-backend
//	search-indexer      alt-backend
//	pre-processor       alt-backend
//	tag-generator       alt-backend
//	alt-butterfly-facade alt-backend
//	rag-orchestrator    alt-data-hub
//	alt-backend         alt-data-hub
//	alt-harvester       alt-data-hub
//	recap-worker        alt-data-hub
//
// Two provider names for one binary is a naming debt, not a second surface:
// alt-data-hub is the deployment that serves DataHubService after the
// ADR-000954 split, and services.yaml registers it as `kind: runtime` rather
// than as a pacticipant of its own. Renaming the pacticipant is a separate
// change — a rename on the Broker orphans the verification history of every
// pact under the old name — so until then the rag-orchestrator pact is
// verified under the name it was published with, against the same stub.
package contract

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"testing"

	"github.com/pact-foundation/pact-go/v2/models"
	"github.com/pact-foundation/pact-go/v2/provider"
	"github.com/stretchr/testify/require"
)

const (
	pactDir = "../../../../../pacts"
	// ragPactDir: rag-orchestrator writes its pacts into its own module rather
	// than the repo-root directory, and scripts/pact-check.sh publishes from
	// both.
	ragPactDir = "../../../../../rag-orchestrator/pacts"
	// altBackendPactDir holds the pacts this module writes as a *consumer* —
	// including the two in-family ones alt-backend and alt-harvester publish
	// against alt-data-hub since ADR-000954 Wave 3. Same convention as the
	// sovereign, pre-processor and search-indexer consumer pacts alongside
	// them; scripts/pact-check.sh publishes this directory too.
	altBackendPactDir = "../../../../pacts"

	providerName = "alt-backend"
	// dataHubProviderName is the pacticipant rag-orchestrator publishes
	// against — see the package comment.
	dataHubProviderName = "alt-data-hub"

	recapWorkerPactFile         = "recap-worker-alt-backend.json"
	searchIndexerPactFile       = "search-indexer-alt-backend.json"
	altButterflyFacadePactFile  = "alt-butterfly-facade-alt-backend.json"
	preProcessorPactFile        = "pre-processor-alt-backend.json"
	tagGeneratorPactFile        = "tag-generator-alt-backend.json"
	ragOrchestratorPactFile     = "rag-orchestrator-alt-data-hub.json"
	altBackendDataHubPactFile   = "alt-backend-alt-data-hub.json"
	altHarvesterDataHubPactFile = "alt-harvester-alt-data-hub.json"
	// recap-worker holds two pacts against this binary under two provider
	// names: the recap window reads still name alt-backend, the notification
	// enqueue names alt-data-hub. The Broker keys a pact on the (consumer,
	// provider) pair, so they are two files rather than two interactions.
	recapWorkerDataHubPactFile = "recap-worker-alt-data-hub.json"
)

// recapArticleResponse mirrors the Connect-RPC JSON shape produced by
// BackendInternalService/ListRecapArticles. protojson uses camelCase, so
// the JSON tags do the same.
type recapArticleResponse struct {
	ArticleID string `json:"articleId"`
	Title     string `json:"title"`
	FullText  string `json:"fulltext"`
}

type recapArticlesResponse struct {
	Range    rangeResponse          `json:"range"`
	Total    int                    `json:"total"`
	Page     int                    `json:"page"`
	PageSize int                    `json:"pageSize"`
	HasMore  bool                   `json:"hasMore"`
	Articles []recapArticleResponse `json:"articles"`
}

// legacyRestRecapArticleResponse / legacyRestRecapArticlesResponse mirror the
// pre-Connect-RPC REST shape. Broker "DeployedOrReleased" pacts from the
// transitional commits still expect snake_case fields with a tags array.
// Kept only as long as those pacts are the deployed version on the broker.
type legacyRestRecapArticleResponse struct {
	ArticleID string          `json:"article_id"`
	Title     string          `json:"title"`
	FullText  string          `json:"fulltext"`
	Tags      []legacyRestTag `json:"tags"`
}

type legacyRestTag struct {
	Label string `json:"label"`
}

type legacyRestRecapArticlesResponse struct {
	Range    rangeResponse                    `json:"range"`
	Total    int                              `json:"total"`
	Page     int                              `json:"page"`
	PageSize int                              `json:"page_size"`
	HasMore  bool                             `json:"has_more"`
	Articles []legacyRestRecapArticleResponse `json:"articles"`
}

type rangeResponse struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// listRecapArticlesRequest mirrors the Connect-RPC request body.
type listRecapArticlesRequest struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// listFeedsInWindowRequest mirrors the Connect-RPC request body for ListFeedsInWindow.
type listFeedsInWindowRequest struct {
	From     string `json:"from"`
	To       string `json:"to"`
	Page     *int   `json:"page,omitempty"`
	PageSize *int   `json:"pageSize,omitempty"`
}

type feedInWindowResponse struct {
	ID          string  `json:"id"`
	Title       string  `json:"title"`
	Description string  `json:"description"`
	WebsiteURL  string  `json:"websiteUrl"`
	PubDate     string  `json:"pubDate,omitempty"`
	CreatedAt   string  `json:"createdAt,omitempty"`
	UpdatedAt   string  `json:"updatedAt,omitempty"`
	ArticleID   *string `json:"articleId,omitempty"`
	IsRead      bool    `json:"isRead"`
	FeedLinkID  *string `json:"feedLinkId,omitempty"`
	OgImageURL  *string `json:"ogImageUrl,omitempty"`
}

type feedsInWindowResponse struct {
	Feeds    []feedInWindowResponse `json:"feeds"`
	Total    int                    `json:"total"`
	Page     int                    `json:"page"`
	PageSize int                    `json:"pageSize"`
	HasMore  bool                   `json:"hasMore"`
}

// dataHubProcedure mounts one procedure of services.datahub.v1.DataHubService, the
// only name the data plane answers to since ADR-000954 Wave 2-C.
//
// It mounted the retired services.backend.v1.BackendInternalService path as
// well while Wave 2-B moved the consumers one PR at a time, so that a peer's
// migration did not also need an edit here. That blanket dual-mount is gone:
// a stub that answered the old name for every procedure would let any future
// consumer pact naming it verify green against a provider that would 404 it
// in production. The BackendInternalService routes that remain are mounted
// explicitly in the transitional-shims block below, scoped to the pacts the
// production-deployed recap-worker and search-indexer still publish, with a
// remove-once-deployed-past marker.
func dataHubProcedure(mux *http.ServeMux, procedure string, h http.HandlerFunc) {
	mux.HandleFunc("/services.datahub.v1.DataHubService/"+procedure, h)
}

// jsonPost answers POST with body and rejects every other method, which is the
// shape every Connect-RPC unary procedure has over the JSON wire format.
func jsonPost(body map[string]interface{}) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(body)
	}
}

// noopStates turns a list of provider-state names into handlers that do
// nothing.
//
// Every state here is a no-op for the same reason: this verification runs
// against a stub, not against a database, so "articles exist" is already true
// by construction.
//
// Declaring the names does NOT make an undeclared one an error, which is the
// opposite of what this comment claimed until 2026-08. pact-go's
// stateHandlerMiddleware (provider/verifier.go) answers a state it has no
// handler for with a `[WARN] no state handler found` line and HTTP 200, so an
// unknown precondition is silently treated as satisfied and the interaction
// runs against whatever the stub's default happens to be. Believing otherwise
// is what let the knowledge-sovereign harness verify a foreign consumer's pact
// and report the resulting mismatch as a contract break. Declare them so the
// list documents the agreed preconditions, and keep FilterConsumers pinned so
// no pact this list was not written for reaches the verifier.
func noopStates(names ...string) models.StateHandlers {
	handlers := models.StateHandlers{}
	for _, name := range names {
		handlers[name] = func(bool, models.ProviderState) (models.ProviderStateResponse, error) {
			return nil, nil
		}
	}
	return handlers
}

// withStates adds the states that do something to a set of declared no-ops. A
// state that has to change what the stub answers cannot go through noopStates,
// and every state a pact names still has to be declared or the verification
// fails, so the two lists are merged rather than kept apart.
func withStates(base models.StateHandlers, extra models.StateHandlers) models.StateHandlers {
	for name, handler := range extra {
		base[name] = handler
	}
	return base
}

// verifyConsumer runs one consumer's pact against the stub, in file mode
// locally and against the Broker in CI.
//
// The three verifications below differ only in consumer name, pact file and
// state list, so they share this rather than each carrying its own copy of the
// broker-vs-file branch. The three that came before it are left as they are:
// each has an accreted exception (recap-worker's transitional shims,
// search-indexer's missing WIP handling) and folding those in would mean
// parameters that exist for one caller.
//
// failIfNoPactsFound carries the flag documented above TestVerifyRecapWorkerContract.
func verifyConsumer(t *testing.T, consumer, providerPacticipant, pactPath string, states models.StateHandlers, failIfNoPactsFound bool) {
	t.Helper()

	brokerURL := os.Getenv("PACT_BROKER_BASE_URL")
	if brokerURL == "" {
		if _, err := os.Stat(pactPath); os.IsNotExist(err) {
			t.Skipf("No Broker URL set and pact file not found: %s. "+
				"Run the %s consumer tests first.", pactPath, consumer)
		}
	}

	verifyRequest := provider.VerifyRequest{
		Provider:           providerPacticipant,
		ProviderBaseURL:    fmt.Sprintf("http://127.0.0.1:%d", startStubServer(t)),
		FilterConsumers:    []string{consumer},
		StateHandlers:      states,
		FailIfNoPactsFound: failIfNoPactsFound,
	}

	if brokerURL != "" {
		verifyRequest.BrokerURL = brokerURL
		verifyRequest.BrokerUsername = os.Getenv("PACT_BROKER_USERNAME")
		verifyRequest.BrokerPassword = os.Getenv("PACT_BROKER_PASSWORD")
		verifyRequest.ConsumerVersionSelectors = []provider.Selector{
			&provider.ConsumerVersionSelector{Consumer: consumer, MainBranch: true},
			&provider.ConsumerVersionSelector{Consumer: consumer, DeployedOrReleased: true},
		}
		if ver := os.Getenv("PACT_PROVIDER_VERSION"); ver != "" {
			verifyRequest.ProviderVersion = ver
		}
		if branch := os.Getenv("PACT_PROVIDER_BRANCH"); branch != "" {
			verifyRequest.ProviderBranch = branch
		}
		verifyRequest.PublishVerificationResults = os.Getenv("PACT_PROVIDER_VERSION") != ""
		if os.Getenv("PACT_DISABLE_PENDING") != "true" {
			verifyRequest.EnablePending = true
		}
	} else {
		verifyRequest.PactFiles = []string{pactPath}
	}

	require.NoError(t, provider.NewVerifier().VerifyProvider(t, verifyRequest))
}
