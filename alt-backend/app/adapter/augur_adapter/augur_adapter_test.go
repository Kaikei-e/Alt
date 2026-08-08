package augur_adapter

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"alt/orchestrator/gateway/rag_gateway"
	"alt/orchestrator/port/rag_integration_port"
	"alt/utils/logger"

	"go.uber.org/mock/gomock"
)

func TestMain(m *testing.M) {
	logger.InitLogger()
	m.Run()
}

func TestAugurAdapter_UpsertArticle_ClassifiesFailures(t *testing.T) {
	input := rag_integration_port.UpsertArticleInput{ArticleID: "a1", URL: "https://example.com/a1"}

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

	t.Run("caller's own context deadline expiring mid-request is NOT transient", func(t *testing.T) {
		// This is the outbox worker's job timeout firing while
		// UpsertIndexWithResponse is still in flight, not a
		// rag-orchestrator-side failure. Classifying it transient would
		// have the worker release the row and retry it into the same job
		// timeout again on the very next tick — stalling the whole worker
		// for another full timeout window per attempt instead of once.
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
		if errors.Is(err, rag_integration_port.ErrRagUpsertTransient) {
			t.Errorf("a caller-context cancellation must not be classified transient, got: %v", err)
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
