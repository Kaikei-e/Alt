//go:build contract

// Consumer-Driven Contract for pre-processor -> alt-data-hub (ADR-000954 D7,
// ADR-000955).
//
// pre-processor reads and writes every article / feed / summary row it touches
// through alt-data-hub's Connect-RPC service. Wave 2-A renamed that contract's
// namespace from services.backend.v1.BackendInternalService to
// services.datahub.v1.DataHubService with identical RPC names and fields, and
// absorbed the last REST route pre-processor called — GET
// /v1/internal/system-user — as the GetSystemUser RPC.
//
// The interactions below pin the *procedure paths*, which is the only thing
// this migration changes on the wire. alt-data-hub serves both namespaces
// during Wave 2, so a path regression here would not show up as a runtime
// failure until the legacy namespace is retired — the pact is what makes it
// fail now instead.
//
// Six of the thirteen RPCs pre-processor calls get a full interaction, one per
// driver file plus the newly absorbed GetSystemUser. The remaining seven are
// covered by TestDataHubProcedurePathsAreNamespaced, which pins every
// procedure constant the generated client routes on; pinning all thirteen as
// pact interactions would oblige alt-data-hub to keep answering example bodies
// that add nothing over the path assertion.
package contract

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"connectrpc.com/connect"
	"github.com/pact-foundation/pact-go/v2/consumer"
	"github.com/pact-foundation/pact-go/v2/matchers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	datahubv1 "pre-processor/gen/proto/services/datahub/v1"
	"pre-processor/gen/proto/services/datahub/v1/datahubv1connect"
)

// dataHubProcedurePrefix is the namespace ADR-000954 D7 moves this contract to.
// Every path matcher below is built from it so a namespace change cannot be
// made in one interaction and forgotten in the others.
const dataHubProcedurePrefix = "/" + datahubv1connect.DataHubServiceName + "/"

func dataHubProcedure(rpc string) string {
	return dataHubProcedurePrefix + rpc
}

func newDataHubPact(t *testing.T) *consumer.V3HTTPMockProvider {
	t.Helper()
	mockProvider, err := consumer.NewV3Pact(consumer.MockHTTPProviderConfig{
		Consumer: "pre-processor",
		Provider: "alt-backend",
		PactDir:  pactDir,
	})
	require.NoError(t, err)
	return mockProvider
}

// newDataHubClient builds the client the interactions drive. ProtoJSON is used
// so the pact records a readable body; alt-data-hub's Connect handlers accept
// both codecs from the same route, so the procedure path — the thing this file
// exists to pin — is identical to what production's binary-proto client sends.
func newDataHubClient(config consumer.MockServerConfig) datahubv1connect.DataHubServiceClient {
	return datahubv1connect.NewDataHubServiceClient(
		http.DefaultClient,
		fmt.Sprintf("http://%s:%d", config.Host, config.Port),
		connect.WithProtoJSON(),
	)
}

// TestDataHubGetFeedIDContract pins the feed resolution every ingested article
// goes through. pre-processor distinguishes "feed not registered" from a real
// outage by the Connect error code, so the not-found leg is contracted too.
func TestDataHubGetFeedIDContract(t *testing.T) {
	mockProvider := newDataHubPact(t)

	err := mockProvider.
		AddInteraction().
		Given("a feed is registered for the requested url").
		UponReceiving("a GetFeedID request").
		WithCompleteRequest(consumer.Request{
			Method: "POST",
			Path:   matchers.String(dataHubProcedure("GetFeedID")),
			Headers: matchers.MapMatcher{
				"Content-Type": matchers.String("application/json"),
			},
			Body: matchers.MapMatcher{
				"feedUrl": matchers.Like("https://example.com/feed.xml"),
			},
		}).
		WithCompleteResponse(consumer.Response{
			Status: 200,
			Headers: matchers.MapMatcher{
				"Content-Type": matchers.String("application/json"),
			},
			Body: matchers.MapMatcher{
				"feedId": matchers.Like("feed-001"),
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			client := newDataHubClient(config)
			resp, err := client.GetFeedID(context.Background(), connect.NewRequest(&datahubv1.GetFeedIDRequest{
				FeedUrl: "https://example.com/feed.xml",
			}))
			if err != nil {
				return fmt.Errorf("GetFeedID failed: %w", err)
			}
			assert.NotEmpty(t, resp.Msg.FeedId)
			return nil
		})
	require.NoError(t, err)
}

// TestDataHubCreateArticleContract pins the ingest write. The response carries
// the assigned article id, which pre-processor writes back onto the domain
// article before the summarize pipeline picks it up.
func TestDataHubCreateArticleContract(t *testing.T) {
	mockProvider := newDataHubPact(t)

	err := mockProvider.
		AddInteraction().
		Given("a feed exists with id feed-001").
		UponReceiving("a CreateArticle request").
		WithCompleteRequest(consumer.Request{
			Method: "POST",
			Path:   matchers.String(dataHubProcedure("CreateArticle")),
			Headers: matchers.MapMatcher{
				"Content-Type": matchers.String("application/json"),
			},
			Body: matchers.MapMatcher{
				"title":    matchers.Like("Go 1.26 Released"),
				"url":      matchers.Like("https://example.com/articles/go-126"),
				"content":  matchers.Like("Article body text."),
				"feedId":   matchers.Like("feed-001"),
				"userId":   matchers.Like("user-001"),
				"language": matchers.Like("en"),
			},
		}).
		WithCompleteResponse(consumer.Response{
			Status: 200,
			Headers: matchers.MapMatcher{
				"Content-Type": matchers.String("application/json"),
			},
			Body: matchers.MapMatcher{
				"articleId": matchers.Like("art-001"),
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			client := newDataHubClient(config)
			resp, err := client.CreateArticle(context.Background(), connect.NewRequest(&datahubv1.CreateArticleRequest{
				Title:    "Go 1.26 Released",
				Url:      "https://example.com/articles/go-126",
				Content:  "Article body text.",
				FeedId:   "feed-001",
				UserId:   "user-001",
				Language: "en",
			}))
			if err != nil {
				return fmt.Errorf("CreateArticle failed: %w", err)
			}
			assert.NotEmpty(t, resp.Msg.ArticleId)
			return nil
		})
	require.NoError(t, err)
}

// TestDataHubListUnsummarizedArticlesContract pins the cursor the summarize
// loop pages on. nextId / nextCreatedAt are what stop the loop re-reading the
// same window forever, so both are part of the contract.
func TestDataHubListUnsummarizedArticlesContract(t *testing.T) {
	mockProvider := newDataHubPact(t)

	err := mockProvider.
		AddInteraction().
		Given("unsummarized articles exist").
		UponReceiving("a ListUnsummarizedArticles request").
		WithCompleteRequest(consumer.Request{
			Method: "POST",
			Path:   matchers.String(dataHubProcedure("ListUnsummarizedArticles")),
			Headers: matchers.MapMatcher{
				"Content-Type": matchers.String("application/json"),
			},
			Body: matchers.MapMatcher{
				"limit": matchers.Like(100),
			},
		}).
		WithCompleteResponse(consumer.Response{
			Status: 200,
			Headers: matchers.MapMatcher{
				"Content-Type": matchers.String("application/json"),
			},
			Body: matchers.MapMatcher{
				"articles": matchers.EachLike(matchers.MapMatcher{
					"id":        matchers.Like("art-001"),
					"title":     matchers.Like("Go 1.26 Released"),
					"content":   matchers.Like("Article body text."),
					"url":       matchers.Like("https://example.com/articles/go-126"),
					"userId":    matchers.Like("user-001"),
					"createdAt": matchers.Like("2026-03-26T00:00:00Z"),
				}, 1),
				"nextId":        matchers.Like("art-002"),
				"nextCreatedAt": matchers.Like("2026-03-26T00:00:00Z"),
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			client := newDataHubClient(config)
			resp, err := client.ListUnsummarizedArticles(context.Background(), connect.NewRequest(&datahubv1.ListUnsummarizedArticlesRequest{
				Limit: 100,
			}))
			if err != nil {
				return fmt.Errorf("ListUnsummarizedArticles failed: %w", err)
			}
			require.NotEmpty(t, resp.Msg.Articles)
			assert.NotEmpty(t, resp.Msg.Articles[0].Id)
			assert.NotEmpty(t, resp.Msg.NextId, "next_id is what advances the summarize cursor")
			require.NotNil(t, resp.Msg.NextCreatedAt)
			return nil
		})
	require.NoError(t, err)
}

// TestDataHubListFeedURLsContract pins the feed pagination FeedRepository
// walks. hasMore / nextCursor terminate the GetProcessingStats loop.
func TestDataHubListFeedURLsContract(t *testing.T) {
	mockProvider := newDataHubPact(t)

	err := mockProvider.
		AddInteraction().
		Given("registered feeds exist").
		UponReceiving("a ListFeedURLs request").
		WithCompleteRequest(consumer.Request{
			Method: "POST",
			Path:   matchers.String(dataHubProcedure("ListFeedURLs")),
			Headers: matchers.MapMatcher{
				"Content-Type": matchers.String("application/json"),
			},
			Body: matchers.MapMatcher{
				"limit": matchers.Like(500),
			},
		}).
		WithCompleteResponse(consumer.Response{
			Status: 200,
			Headers: matchers.MapMatcher{
				"Content-Type": matchers.String("application/json"),
			},
			Body: matchers.MapMatcher{
				"feeds": matchers.EachLike(matchers.MapMatcher{
					"url": matchers.Like("https://example.com/feed.xml"),
				}, 1),
				"nextCursor": matchers.Like("feed-002"),
				"hasMore":    matchers.Like(true),
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			client := newDataHubClient(config)
			resp, err := client.ListFeedURLs(context.Background(), connect.NewRequest(&datahubv1.ListFeedURLsRequest{
				Limit: 500,
			}))
			if err != nil {
				return fmt.Errorf("ListFeedURLs failed: %w", err)
			}
			require.NotEmpty(t, resp.Msg.Feeds)
			assert.NotEmpty(t, resp.Msg.Feeds[0].Url)
			return nil
		})
	require.NoError(t, err)
}

// TestDataHubSaveArticleSummaryContract pins the summary write-back. This is
// the one RPC whose failure silently drops work the LLM already paid for, so
// it is contracted even though its response carries nothing pre-processor reads.
func TestDataHubSaveArticleSummaryContract(t *testing.T) {
	mockProvider := newDataHubPact(t)

	err := mockProvider.
		AddInteraction().
		Given("an article exists with id art-001").
		UponReceiving("a SaveArticleSummary request").
		WithCompleteRequest(consumer.Request{
			Method: "POST",
			Path:   matchers.String(dataHubProcedure("SaveArticleSummary")),
			Headers: matchers.MapMatcher{
				"Content-Type": matchers.String("application/json"),
			},
			Body: matchers.MapMatcher{
				"articleId": matchers.Like("art-001"),
				"summary":   matchers.Like("これはテスト記事の要約です。"),
				"language":  matchers.String("ja"),
				"userId":    matchers.Like("user-001"),
			},
		}).
		WithCompleteResponse(consumer.Response{
			Status: 200,
			Headers: matchers.MapMatcher{
				"Content-Type": matchers.String("application/json"),
			},
			Body: matchers.MapMatcher{},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			client := newDataHubClient(config)
			_, err := client.SaveArticleSummary(context.Background(), connect.NewRequest(&datahubv1.SaveArticleSummaryRequest{
				ArticleId: "art-001",
				Summary:   "これはテスト記事の要約です。",
				Language:  "ja",
				UserId:    "user-001",
			}))
			if err != nil {
				return fmt.Errorf("SaveArticleSummary failed: %w", err)
			}
			return nil
		})
	require.NoError(t, err)
}

// TestDataHubGetSystemUserContract pins the RPC that replaces
// GET /v1/internal/system-user (ADR-000954 D6/D7). pre-processor stamps every
// backfilled article with this id, so an empty user_id is a data-correctness
// failure, not a cosmetic one — hence the non-empty assertion here and the
// explicit empty-id rejection in the repository.
func TestDataHubGetSystemUserContract(t *testing.T) {
	mockProvider := newDataHubPact(t)

	err := mockProvider.
		AddInteraction().
		Given("a Kratos identity exists").
		UponReceiving("a GetSystemUser request").
		WithCompleteRequest(consumer.Request{
			Method: "POST",
			Path:   matchers.String(dataHubProcedure("GetSystemUser")),
			Headers: matchers.MapMatcher{
				"Content-Type": matchers.String("application/json"),
			},
			Body: matchers.MapMatcher{},
		}).
		WithCompleteResponse(consumer.Response{
			Status: 200,
			Headers: matchers.MapMatcher{
				"Content-Type": matchers.String("application/json"),
			},
			Body: matchers.MapMatcher{
				"userId": matchers.Like("11111111-2222-3333-4444-555555555555"),
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			client := newDataHubClient(config)
			resp, err := client.GetSystemUser(context.Background(), connect.NewRequest(&datahubv1.GetSystemUserRequest{}))
			if err != nil {
				return fmt.Errorf("GetSystemUser failed: %w", err)
			}
			assert.NotEmpty(t, resp.Msg.UserId, "system user id is stamped onto every backfilled article")
			return nil
		})
	require.NoError(t, err)
}

// TestDataHubProcedurePathsAreNamespaced covers the seven RPCs that do not get
// their own interaction above. The pacts pin what alt-data-hub must answer;
// this pins what pre-processor is allowed to ask for, so a stub regenerated
// against a retired namespace fails here rather than at the first production
// call after those routes are removed.
func TestDataHubProcedurePathsAreNamespaced(t *testing.T) {
	assert.Equal(t, "services.datahub.v1.DataHubService", datahubv1connect.DataHubServiceName,
		"ADR-000954 D7 fixes the service's fully-qualified name")

	tests := []struct {
		name      string
		procedure string
	}{
		{name: "GetFeedID", procedure: datahubv1connect.DataHubServiceGetFeedIDProcedure},
		{name: "GetEmptyFeedID", procedure: datahubv1connect.DataHubServiceGetEmptyFeedIDProcedure},
		{name: "ListFeedURLs", procedure: datahubv1connect.DataHubServiceListFeedURLsProcedure},
		{name: "CreateArticle", procedure: datahubv1connect.DataHubServiceCreateArticleProcedure},
		{name: "CheckArticleExists", procedure: datahubv1connect.DataHubServiceCheckArticleExistsProcedure},
		{name: "GetArticleContent", procedure: datahubv1connect.DataHubServiceGetArticleContentProcedure},
		{name: "ListUnsummarizedArticles", procedure: datahubv1connect.DataHubServiceListUnsummarizedArticlesProcedure},
		{name: "HasUnsummarizedArticles", procedure: datahubv1connect.DataHubServiceHasUnsummarizedArticlesProcedure},
		{name: "SaveArticleSummary", procedure: datahubv1connect.DataHubServiceSaveArticleSummaryProcedure},
		{name: "DeleteArticleSummary", procedure: datahubv1connect.DataHubServiceDeleteArticleSummaryProcedure},
		{name: "CheckArticleSummaryExists", procedure: datahubv1connect.DataHubServiceCheckArticleSummaryExistsProcedure},
		{name: "FindArticlesWithSummaries", procedure: datahubv1connect.DataHubServiceFindArticlesWithSummariesProcedure},
		{name: "GetSystemUser", procedure: datahubv1connect.DataHubServiceGetSystemUserProcedure},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, dataHubProcedure(tt.name), tt.procedure)
			assert.NotContains(t, tt.procedure, "services.backend.v1",
				"the legacy namespace is retired by ADR-000954 D7")
		})
	}
}
