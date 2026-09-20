package augur_adapter

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	"alt/orchestrator/gateway/rag_gateway"
	"alt/orchestrator/port/rag_integration_port"
	"alt/utils/logger"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestMain(m *testing.M) {
	logger.InitLogger()
	m.Run()
}

func TestAugurAdapter_UpsertArticle_ClassifiesFailures(t *testing.T) {
	input := rag_integration_port.UpsertArticleInput{
		ArticleID: "a1",
		URL:       "https://example.com/a1",
		UserID:    "11111111-2222-3333-4444-555555555555",
	}

	t.Run("transport error is transient", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		client := NewMockRagClientInterface(ctrl)
		client.EXPECT().
			UpsertIndexWithResponse(gomock.Any(), gomock.Any()).
			Return(nil, errors.New("dial tcp: connection refused"))

		err := NewAugurAdapter(client).UpsertArticle(t.Context(), input)

		if err == nil {
			t.Fatal("expected an error")
		}
		if !errors.Is(err, rag_integration_port.ErrRagUpsertTransient) {
			t.Errorf("transport error must be classified transient, got: %v", err)
		}
	})

	t.Run("500 from rag-orchestrator is transient", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		client := NewMockRagClientInterface(ctrl)
		client.EXPECT().
			UpsertIndexWithResponse(gomock.Any(), gomock.Any()).
			Return(&rag_gateway.UpsertIndexResponse{HTTPResponse: &http.Response{StatusCode: http.StatusInternalServerError}}, nil)

		err := NewAugurAdapter(client).UpsertArticle(t.Context(), input)

		if err == nil {
			t.Fatal("expected an error")
		}
		if !errors.Is(err, rag_integration_port.ErrRagUpsertTransient) {
			t.Errorf("a 500 must be classified transient (retryable), got: %v", err)
		}
	})

	t.Run("caller's job context dying mid-request IS transient", func(t *testing.T) {
		// SIGTERM during a redeploy cancels the harvester's job context
		// while UpsertIndexWithResponse is in flight. Nothing about this
		// article failed — it simply never got its turn. Classifying it
		// terminal wrote the row FAILED, which the PENDING-only claim
		// query never re-fetches and outbox-prune never removes, so every
		// redeploy permanently un-indexed whichever article was mid-upsert.
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		client := NewMockRagClientInterface(ctrl)

		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		client.EXPECT().
			UpsertIndexWithResponse(gomock.Any(), gomock.Any()).
			Return(nil, context.Canceled)

		err := NewAugurAdapter(client).UpsertArticle(ctx, input)

		if err == nil {
			t.Fatal("expected an error")
		}
		if !errors.Is(err, rag_integration_port.ErrRagUpsertTransient) {
			t.Errorf("a caller-context cancellation must be classified transient so the row returns to PENDING, got: %v", err)
		}
	})

	t.Run("one upsert cannot spend the caller's whole budget", func(t *testing.T) {
		// Releasing a canceled upsert back to PENDING is only affordable
		// because a single article cannot occupy the worker for the whole
		// job timeout: without a budget of its own, one article that never
		// returns starves the other nine rows of the batch until the job
		// times out, and each retry costs another full window. The budget
		// caps that at one article's worth, and the outbox's attempt
		// budget caps how often it is paid.
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		client := NewMockRagClientInterface(ctrl)
		client.EXPECT().
			UpsertIndexWithResponse(gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, _ rag_gateway.UpsertIndexJSONRequestBody, _ ...rag_gateway.RequestEditorFn) (*rag_gateway.UpsertIndexResponse, error) {
				<-ctx.Done() // the embedder never answers
				return nil, ctx.Err()
			})

		// The caller's context carries no deadline at all, so anything
		// that bounds this call has to come from the adapter itself.
		adapter := &AugurAdapter{client: client, upsertTimeout: 50 * time.Millisecond}
		done := make(chan error, 1)
		go func() { done <- adapter.UpsertArticle(context.Background(), input) }()

		select {
		case err := <-done:
			if err == nil {
				t.Fatal("expected an error")
			}
			if !errors.Is(err, rag_integration_port.ErrRagUpsertTransient) {
				t.Errorf("a blown per-article budget must stay retryable, got: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("UpsertArticle never returned: it has no budget of its own and inherits the caller's")
		}
	})

	t.Run("the budget is not the binding constraint on a heavy article", func(t *testing.T) {
		// The budget exists so one stuck article cannot spend the outbox
		// worker's whole window and release the other nine claimed rows
		// unattempted. But it is a ceiling on a call that used to inherit
		// that window, so setting it below what a legitimately heavy article
		// takes does not cost that article one attempt: a blown budget is
		// transient (the case above), so the row returns to PENDING, is
		// re-claimed next tick, and is cut at the same point again until the
		// worker's attempt budget is spent and the row is marked terminally
		// FAILED. That is the same permanent loss the transient
		// classification exists to prevent, reached by a different road.
		//
		// A heavy article (500+KB, 100+ chunks) takes 10-30s on the local
		// embedder; three times the top of that range is the headroom for a
		// cold model load or a host under batch load. Articles that slow used
		// to finish on the job's context and must still finish.
		const heavyArticleUpsertUnderLoad = 90 * time.Second
		// The timeout orchestrator/job/registry.go registers outbox-worker
		// with, for a batch of ten.
		const outboxWorkerJobBudget = 5 * time.Minute

		if upsertArticleTimeout < heavyArticleUpsertUnderLoad {
			t.Errorf("per-article budget %s is under the %s a heavy article can legitimately take: every such article is cut, released to PENDING and cut again until it is marked FAILED",
				upsertArticleTimeout, heavyArticleUpsertUnderLoad)
		}
		if upsertArticleTimeout > outboxWorkerJobBudget/2 {
			t.Errorf("per-article budget %s leaves under half of the %s job budget for the other nine rows of the batch",
				upsertArticleTimeout, outboxWorkerJobBudget)
		}
	})

	t.Run("400 from rag-orchestrator is NOT transient", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		client := NewMockRagClientInterface(ctrl)
		client.EXPECT().
			UpsertIndexWithResponse(gomock.Any(), gomock.Any()).
			Return(&rag_gateway.UpsertIndexResponse{HTTPResponse: &http.Response{StatusCode: http.StatusBadRequest}}, nil)

		err := NewAugurAdapter(client).UpsertArticle(t.Context(), input)

		if err == nil {
			t.Fatal("expected an error")
		}
		if errors.Is(err, rag_integration_port.ErrRagUpsertTransient) {
			t.Errorf("a 400 (permanent rejection) must not be classified transient, got: %v", err)
		}
	})

	t.Run("200 is success", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		client := NewMockRagClientInterface(ctrl)
		client.EXPECT().
			UpsertIndexWithResponse(gomock.Any(), gomock.Any()).
			Return(&rag_gateway.UpsertIndexResponse{HTTPResponse: &http.Response{StatusCode: http.StatusOK}}, nil)

		if err := NewAugurAdapter(client).UpsertArticle(t.Context(), input); err != nil {
			t.Errorf("expected no error, got: %v", err)
		}
	})
}

func TestAugurAdapter_UpsertArticle_MissingUserID_FailsLoudly(t *testing.T) {
	testCases := []struct {
		name   string
		userID string
	}{
		{name: "empty user_id", userID: ""},
		{name: "whitespace user_id", userID: "   \t\n"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			client := NewMockRagClientInterface(ctrl)
			// No call to UpsertIndexWithResponse must be made when user_id is missing.

			adapter := NewAugurAdapter(client)
			err := adapter.UpsertArticle(context.Background(), rag_integration_port.UpsertArticleInput{
				ArticleID: "article-123",
				Title:     "Article without owner",
				URL:       "https://example.com/unowned",
				Body:      "content",
				UserID:    tc.userID,
			})

			require.Error(t, err)
			assert.True(t, errors.Is(err, rag_integration_port.ErrMissingUserID),
				"error must wrap ErrMissingUserID, got: %v", err)
			assert.False(t, errors.Is(err, rag_integration_port.ErrRagUpsertTransient),
				"missing user_id must be a permanent rejection, never classified as transient")
		})
	}
}

func TestAugurAdapter_UpsertArticle_RequestBody_GoldenJSON(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := NewMockRagClientInterface(ctrl)

	publishedAt := time.Date(2026, 9, 20, 10, 0, 0, 0, time.UTC)
	updatedAt := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)

	input := rag_integration_port.UpsertArticleInput{
		ArticleID:   "6f1a2f7e-1f1e-4c2a-9a3e-5b6c7d8e9f01",
		UserID:      "11111111-2222-3333-4444-555555555555",
		Title:       "Test Article Title",
		URL:         "https://example.com/article",
		Body:        "Full text content of the article to be indexed for RAG retrieval.",
		PublishedAt: &publishedAt,
		UpdatedAt:   &updatedAt,
	}

	var capturedBody rag_gateway.UpsertIndexJSONRequestBody
	client.EXPECT().
		UpsertIndexWithResponse(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, body rag_gateway.UpsertIndexJSONRequestBody, _ ...rag_gateway.RequestEditorFn) (*rag_gateway.UpsertIndexResponse, error) {
			capturedBody = body
			return &rag_gateway.UpsertIndexResponse{HTTPResponse: &http.Response{StatusCode: http.StatusOK}}, nil
		})

	adapter := NewAugurAdapter(client)
	err := adapter.UpsertArticle(context.Background(), input)
	require.NoError(t, err)

	bodyBytes, err := json.Marshal(capturedBody)
	require.NoError(t, err)

	const expectedGoldenJSON = `{"article_id":"6f1a2f7e-1f1e-4c2a-9a3e-5b6c7d8e9f01","body":"Full text content of the article to be indexed for RAG retrieval.","published_at":"2026-09-20T10:00:00Z","title":"Test Article Title","updated_at":"2026-09-20T12:00:00Z","url":"https://example.com/article","user_id":"11111111-2222-3333-4444-555555555555"}`

	assert.JSONEq(t, expectedGoldenJSON, string(bodyBytes),
		"RAG upsert request body must match expected golden JSON format including user_id")
}

func TestAugurAdapter_RetrieveContext_RejectsBlankUserID(t *testing.T) {
	testCases := []struct {
		name   string
		userID string
	}{
		{name: "empty user_id", userID: ""},
		{name: "whitespace user_id", userID: "   \t\n"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			client := NewMockRagClientInterface(ctrl)
			// No call to RetrieveContextWithResponse must be made when user_id is missing.

			adapter := NewAugurAdapter(client)
			contexts, err := adapter.RetrieveContext(context.Background(), "test query", []string{"art-1"}, tc.userID)

			require.Error(t, err)
			assert.Nil(t, contexts)
			assert.True(t, errors.Is(err, rag_integration_port.ErrMissingUserID),
				"error must wrap ErrMissingUserID, got: %v", err)
		})
	}
}

func TestAugurAdapter_RetrieveContext_RequestBody_GoldenJSON(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := NewMockRagClientInterface(ctrl)

	const (
		query  = "test search query"
		userID = "11111111-2222-3333-4444-555555555555"
	)
	candidateIDs := []string{"art-1", "art-2"}

	var capturedBody rag_gateway.RetrieveContextJSONRequestBody
	client.EXPECT().
		RetrieveContextWithResponse(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, body rag_gateway.RetrieveContextJSONRequestBody, _ ...rag_gateway.RequestEditorFn) (*rag_gateway.RetrieveContextResponse, error) {
			capturedBody = body
			emptyContexts := []rag_gateway.Context{}
			return &rag_gateway.RetrieveContextResponse{
				HTTPResponse: &http.Response{StatusCode: http.StatusOK},
				JSON200: &rag_gateway.RetrieveResponse{
					Contexts: &emptyContexts,
				},
			}, nil
		})

	adapter := NewAugurAdapter(client)
	_, err := adapter.RetrieveContext(context.Background(), query, candidateIDs, userID)
	require.NoError(t, err)

	bodyBytes, err := json.Marshal(capturedBody)
	require.NoError(t, err)

	const expectedGoldenJSON = `{"candidate_article_ids":["art-1","art-2"],"query":"test search query","user_id":"11111111-2222-3333-4444-555555555555"}`

	assert.JSONEq(t, expectedGoldenJSON, string(bodyBytes),
		"RAG retrieve context request body must match expected golden JSON format including user_id")
}
