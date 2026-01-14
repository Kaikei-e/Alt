package augur_test

import (
	"context"
	"io"
	"log/slog"
	"testing"

	augurv2 "alt/gen/proto/alt/augur/v2"

	"rag-orchestrator/internal/adapter/connect/augur"
	"rag-orchestrator/internal/usecase"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// MockAnswerWithRAGUsecase mocks the AnswerWithRAGUsecase interface
type MockAnswerWithRAGUsecase struct {
	mock.Mock
}

func (m *MockAnswerWithRAGUsecase) Execute(ctx context.Context, input usecase.AnswerWithRAGInput) (*usecase.AnswerWithRAGOutput, error) {
	args := m.Called(ctx, input)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*usecase.AnswerWithRAGOutput), args.Error(1)
}

func (m *MockAnswerWithRAGUsecase) Stream(ctx context.Context, input usecase.AnswerWithRAGInput) <-chan usecase.StreamEvent {
	args := m.Called(ctx, input)
	return args.Get(0).(<-chan usecase.StreamEvent)
}

// MockRetrieveContextUsecase mocks the RetrieveContextUsecase interface
type MockRetrieveContextUsecase struct {
	mock.Mock
}

func (m *MockRetrieveContextUsecase) Execute(ctx context.Context, input usecase.RetrieveContextInput) (*usecase.RetrieveContextOutput, error) {
	args := m.Called(ctx, input)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*usecase.RetrieveContextOutput), args.Error(1)
}

func TestHandler_RetrieveContext_Success(t *testing.T) {
	mockAnswer := new(MockAnswerWithRAGUsecase)
	mockRetrieve := new(MockRetrieveContextUsecase)
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))

	handler := augur.NewHandler(mockAnswer, mockRetrieve, logger)

	ctx := context.Background()
	req := connect.NewRequest(&augurv2.RetrieveContextRequest{
		Query: "AIについて",
		Limit: 5,
	})

	mockRetrieve.On("Execute", ctx, mock.MatchedBy(func(input usecase.RetrieveContextInput) bool {
		return input.Query == "AIについて"
	})).Return(&usecase.RetrieveContextOutput{
		Contexts: []usecase.ContextItem{
			{
				URL:         "https://example.com/article1",
				Title:       "AI記事1",
				PublishedAt: "2026-01-14T10:00:00Z",
				Score:       0.95,
			},
			{
				URL:         "https://example.com/article2",
				Title:       "AI記事2",
				PublishedAt: "2026-01-14T09:00:00Z",
				Score:       0.85,
			},
		},
	}, nil)

	resp, err := handler.RetrieveContext(ctx, req)

	require.NoError(t, err)
	assert.Len(t, resp.Msg.Contexts, 2)
	assert.Equal(t, "https://example.com/article1", resp.Msg.Contexts[0].Url)
	assert.Equal(t, "AI記事1", resp.Msg.Contexts[0].Title)
	assert.Equal(t, float32(0.95), resp.Msg.Contexts[0].Score)
}

func TestHandler_RetrieveContext_EmptyQuery(t *testing.T) {
	mockAnswer := new(MockAnswerWithRAGUsecase)
	mockRetrieve := new(MockRetrieveContextUsecase)
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))

	handler := augur.NewHandler(mockAnswer, mockRetrieve, logger)

	ctx := context.Background()
	req := connect.NewRequest(&augurv2.RetrieveContextRequest{
		Query: "",
	})

	_, err := handler.RetrieveContext(ctx, req)

	require.Error(t, err)
	connectErr := err.(*connect.Error)
	assert.Equal(t, connect.CodeInvalidArgument, connectErr.Code())
}

func TestHandler_RetrieveContext_WithLimit(t *testing.T) {
	mockAnswer := new(MockAnswerWithRAGUsecase)
	mockRetrieve := new(MockRetrieveContextUsecase)
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))

	handler := augur.NewHandler(mockAnswer, mockRetrieve, logger)

	ctx := context.Background()
	req := connect.NewRequest(&augurv2.RetrieveContextRequest{
		Query: "AIについて",
		Limit: 1, // Limit to 1 result
	})

	mockRetrieve.On("Execute", ctx, mock.MatchedBy(func(input usecase.RetrieveContextInput) bool {
		return input.Query == "AIについて"
	})).Return(&usecase.RetrieveContextOutput{
		Contexts: []usecase.ContextItem{
			{
				URL:         "https://example.com/article1",
				Title:       "AI記事1",
				PublishedAt: "2026-01-14T10:00:00Z",
				Score:       0.95,
			},
			{
				URL:         "https://example.com/article2",
				Title:       "AI記事2",
				PublishedAt: "2026-01-14T09:00:00Z",
				Score:       0.85,
			},
		},
	}, nil)

	resp, err := handler.RetrieveContext(ctx, req)

	require.NoError(t, err)
	// Should be limited to 1 result
	assert.Len(t, resp.Msg.Contexts, 1)
	assert.Equal(t, "https://example.com/article1", resp.Msg.Contexts[0].Url)
}

func TestNewHandler(t *testing.T) {
	mockAnswer := new(MockAnswerWithRAGUsecase)
	mockRetrieve := new(MockRetrieveContextUsecase)
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))

	handler := augur.NewHandler(mockAnswer, mockRetrieve, logger)

	assert.NotNil(t, handler)
}
