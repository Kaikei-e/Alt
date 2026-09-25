//go:build contract

package contract

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/pact-foundation/pact-go/v2/models"
	"github.com/pact-foundation/pact-go/v2/provider"
	"github.com/stretchr/testify/require"
)

// FailIfNoPactsFound is set on every verification below whose consumer pact is
// published into the Broker this runs against.
//
// Without it the verifier answers an empty Broker with `Ignoring no pacts
// error` and reports a green verification of zero interactions, so "the
// consumer never published" and "the provider satisfies every consumer" look
// identical from the outside — the silent-fallback shape this repo forbids
// elsewhere. Every verification in this package passed that way while the
// Broker held nothing at all for alt-backend.
//
// It is off for exactly one consumer, tag-generator; the reason is at that
// call site. The file-mode branch always supplies a pact, because each test
// skips earlier when the file is missing, so the flag can only fire against
// the Broker.
func TestVerifyRecapWorkerContract(t *testing.T) {
	pactFile := filepath.Join(pactDir, recapWorkerPactFile)

	// Support Broker mode via environment variables
	brokerURL := os.Getenv("PACT_BROKER_BASE_URL")

	if brokerURL == "" {
		// Local file mode: check pact file exists
		if _, err := os.Stat(pactFile); os.IsNotExist(err) {
			t.Skipf("No Broker URL set and pact file not found: %s. "+
				"Set PACT_BROKER_BASE_URL or run Rust consumer tests first.", pactFile)
		}
	}

	port := startStubServer(t)

	verifyRequest := provider.VerifyRequest{
		Provider:           providerName,
		ProviderBaseURL:    fmt.Sprintf("http://127.0.0.1:%d", port),
		FilterConsumers:    []string{"recap-worker"},
		FailIfNoPactsFound: true,
		StateHandlers: withStates(models.StateHandlers{
			"articles exist in the recap window": func(setup bool, s models.ProviderState) (models.ProviderStateResponse, error) {
				// No-op: stub server always returns articles
				return nil, nil
			},
			"feeds exist in the recap window": func(setup bool, s models.ProviderState) (models.ProviderStateResponse, error) {
				// No-op: stub server always returns feeds
				return nil, nil
			},
			"feeds exist in the window": func(setup bool, s models.ProviderState) (models.ProviderStateResponse, error) {
				// No-op: stub server always returns feeds
				return nil, nil
			},
			"feeds exist in window": func(setup bool, s models.ProviderState) (models.ProviderStateResponse, error) {
				// No-op: stub server always returns feeds
				return nil, nil
			},
			"the feed window limit is eight days": func(setup bool, s models.ProviderState) (models.ProviderStateResponse, error) {
				// No-op: request exceeds 8 days, stub rejects with 400
				return nil, nil
			},
			"tags exist for the requested articles": func(setup bool, s models.ProviderState) (models.ProviderStateResponse, error) {
				// No-op: stub server always returns tags for art-001
				return nil, nil
			},
		}, readStateStates()),
	}

	if brokerURL != "" {
		verifyRequest.BrokerURL = brokerURL
		verifyRequest.BrokerUsername = os.Getenv("PACT_BROKER_USERNAME")
		verifyRequest.BrokerPassword = os.Getenv("PACT_BROKER_PASSWORD")
		verifyRequest.ConsumerVersionSelectors = []provider.Selector{
			&provider.ConsumerVersionSelector{Consumer: "recap-worker", MainBranch: true},
			&provider.ConsumerVersionSelector{Consumer: "recap-worker", DeployedOrReleased: true},
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
		if since := os.Getenv("PACT_INCLUDE_WIP_SINCE"); since != "" {
			if t, err := time.Parse(time.RFC3339, since); err == nil {
				verifyRequest.IncludeWIPPactsSince = &t
			}
		}
	} else {
		verifyRequest.PactFiles = []string{pactFile}
	}

	verifier := provider.NewVerifier()
	err := verifier.VerifyProvider(t, verifyRequest)
	require.NoError(t, err)
}

// TestVerifyAltButterflyFacadeContract verifies that alt-backend satisfies
// the BFF's proxy-layer contract for FeedService.GetFeed/GetFeedStats and
// KnowledgeHomeAdminService.GetOverview. The BFF fans these Connect-RPC
// calls out to alt-backend; alt-backend must keep the wire format stable.
func TestVerifyAltButterflyFacadeContract(t *testing.T) {
	pactFile := filepath.Join(pactDir, altButterflyFacadePactFile)

	brokerURL := os.Getenv("PACT_BROKER_BASE_URL")
	if brokerURL == "" {
		if _, err := os.Stat(pactFile); os.IsNotExist(err) {
			t.Skipf("No Broker URL set and pact file not found: %s. "+
				"Run alt-butterfly-facade consumer tests first.", pactFile)
		}
	}

	port := startStubServer(t)

	verifyRequest := provider.VerifyRequest{
		Provider:           providerName,
		ProviderBaseURL:    fmt.Sprintf("http://127.0.0.1:%d", port),
		FilterConsumers:    []string{"alt-butterfly-facade"},
		FailIfNoPactsFound: true,
		StateHandlers: models.StateHandlers{
			"article does not exist": func(setup bool, s models.ProviderState) (models.ProviderStateResponse, error) {
				return nil, nil
			},
			"feed stats are available": func(setup bool, s models.ProviderState) (models.ProviderStateResponse, error) {
				return nil, nil
			},
			"knowledge home admin service is available": func(setup bool, s models.ProviderState) (models.ProviderStateResponse, error) {
				return nil, nil
			},
			// No-op: the prefetch procedure's precondition is a
			// composition-root setting, and the handler mounted for this
			// verification declares it enabled. A state handler that flipped
			// it would only be testing the state handler.
			"article content prefetch is enabled": func(setup bool, s models.ProviderState) (models.ProviderStateResponse, error) {
				return nil, nil
			},
		},
	}

	if brokerURL != "" {
		verifyRequest.BrokerURL = brokerURL
		verifyRequest.BrokerUsername = os.Getenv("PACT_BROKER_USERNAME")
		verifyRequest.BrokerPassword = os.Getenv("PACT_BROKER_PASSWORD")
		verifyRequest.ConsumerVersionSelectors = []provider.Selector{
			&provider.ConsumerVersionSelector{Consumer: "alt-butterfly-facade", MainBranch: true},
			&provider.ConsumerVersionSelector{Consumer: "alt-butterfly-facade", DeployedOrReleased: true},
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
		if since := os.Getenv("PACT_INCLUDE_WIP_SINCE"); since != "" {
			if t, err := time.Parse(time.RFC3339, since); err == nil {
				verifyRequest.IncludeWIPPactsSince = &t
			}
		}
	} else {
		verifyRequest.PactFiles = []string{pactFile}
	}

	verifier := provider.NewVerifier()
	err := verifier.VerifyProvider(t, verifyRequest)
	require.NoError(t, err)
}

// TestVerifySearchIndexerContract verifies that alt-backend's Connect-RPC
// BackendInternalService fulfills the contract expected by search-indexer
// (GetLatestArticleTimestamp + ListArticlesWithTags via JSON wire format).
func TestVerifySearchIndexerContract(t *testing.T) {
	pactFile := filepath.Join(pactDir, searchIndexerPactFile)

	brokerURL := os.Getenv("PACT_BROKER_BASE_URL")
	if brokerURL == "" {
		if _, err := os.Stat(pactFile); os.IsNotExist(err) {
			t.Skipf("No Broker URL set and pact file not found: %s. "+
				"Run search-indexer consumer tests first.", pactFile)
		}
	}

	port := startStubServer(t)

	verifyRequest := provider.VerifyRequest{
		Provider:           providerName,
		ProviderBaseURL:    fmt.Sprintf("http://127.0.0.1:%d", port),
		FilterConsumers:    []string{"search-indexer"},
		FailIfNoPactsFound: true,
		StateHandlers: models.StateHandlers{
			"articles exist in the database": func(setup bool, s models.ProviderState) (models.ProviderStateResponse, error) {
				return nil, nil
			},
			"articles with tags exist for backward pagination": func(setup bool, s models.ProviderState) (models.ProviderStateResponse, error) {
				return nil, nil
			},
			"an article with a source publication timestamp exists": func(setup bool, s models.ProviderState) (models.ProviderStateResponse, error) {
				return nil, nil
			},
		},
	}

	if brokerURL != "" {
		verifyRequest.BrokerURL = brokerURL
		verifyRequest.BrokerUsername = os.Getenv("PACT_BROKER_USERNAME")
		verifyRequest.BrokerPassword = os.Getenv("PACT_BROKER_PASSWORD")
		verifyRequest.ConsumerVersionSelectors = []provider.Selector{
			&provider.ConsumerVersionSelector{Consumer: "search-indexer", MainBranch: true},
			&provider.ConsumerVersionSelector{Consumer: "search-indexer", DeployedOrReleased: true},
		}
		if ver := os.Getenv("PACT_PROVIDER_VERSION"); ver != "" {
			verifyRequest.ProviderVersion = ver
		}
		if branch := os.Getenv("PACT_PROVIDER_BRANCH"); branch != "" {
			verifyRequest.ProviderBranch = branch
		}
		verifyRequest.PublishVerificationResults = os.Getenv("PACT_PROVIDER_VERSION") != ""
	} else {
		verifyRequest.PactFiles = []string{pactFile}
	}

	verifier := provider.NewVerifier()
	err := verifier.VerifyProvider(t, verifyRequest)
	require.NoError(t, err)
}

// TestVerifyPreProcessorContract verifies the crawl/summarise loop's half of
// services.datahub.v1.DataHubService.
//
// pre-processor is the only consumer that both reads and writes through this
// service, so it is the one whose pact would catch a write path answering with
// a shape its caller cannot read — and GetSystemUser is here rather than in a
// REST suite because ADR-000954 D6 moved it onto this service. Until this
// existed, alt-backend had a published pact from pre-processor that no
// provider job verified: the pact was generated, published, and never checked
// against anything.
func TestVerifyPreProcessorContract(t *testing.T) {
	verifyConsumer(t, "pre-processor", providerName,
		filepath.Join(pactDir, preProcessorPactFile),
		noopStates(
			"a feed exists with id feed-001",
			"a feed is registered for the requested url",
			"a Kratos identity exists",
			"registered feeds exist",
			"unsummarized articles exist",
			"an article exists with id art-001",
		), true)
}

// TestVerifyTagGeneratorContract verifies the tagging loop: find untagged
// articles, read one's body, write tags back one article at a time or in a
// batch.
//
// This is the one verification here that cannot demand a pact. tag-generator's
// consumer pacts are generated by its Python suite, which the proto-contract
// workflow never runs as a consumer leg, so nothing publishes
// tag-generator-alt-backend.json into that workflow's Broker and the pact this
// replays there is the committed snapshot, not a freshly generated one. Only
// scripts/pact-check.sh, which does run that leg, puts a published pact in
// front of this verification.
func TestVerifyTagGeneratorContract(t *testing.T) {
	verifyConsumer(t, "tag-generator", providerName,
		filepath.Join(pactDir, tagGeneratorPactFile),
		noopStates(
			"untagged articles exist",
			"an article with body text exists",
			"the article exists and has no tags",
			"both articles exist and have no tags",
		), false)
}
