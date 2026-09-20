package morning_letter_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	morningletterv2 "alt/gen/proto/alt/morning_letter/v2"
	"alt/gen/proto/alt/morning_letter/v2/morningletterv2connect"

	"rag-orchestrator/internal/adapter/connect/morning_letter"
	"rag-orchestrator/internal/domain"
	"rag-orchestrator/internal/usecase"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type mockAnswerWithRAGUsecase struct {
	mock.Mock
}

func (m *mockAnswerWithRAGUsecase) Execute(ctx context.Context, input usecase.AnswerWithRAGInput) (*usecase.AnswerWithRAGOutput, error) {
	args := m.Called(ctx, input)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*usecase.AnswerWithRAGOutput), args.Error(1)
}

func (m *mockAnswerWithRAGUsecase) Stream(ctx context.Context, input usecase.AnswerWithRAGInput) <-chan usecase.StreamEvent {
	args := m.Called(ctx, input)
	return args.Get(0).(<-chan usecase.StreamEvent)
}

type mockArticleClient struct {
	mock.Mock
}

func (m *mockArticleClient) GetRecentArticles(ctx context.Context, withinHours int, limit int) ([]domain.ArticleMetadata, error) {
	args := m.Called(ctx, withinHours, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]domain.ArticleMetadata), args.Error(1)
}

func TestMorningLetterHandler_StreamChat_Unauthenticated(t *testing.T) {
	mockArticle := new(mockArticleClient)
	mockAnswer := new(mockAnswerWithRAGUsecase)
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))

	handler := morning_letter.NewHandler(mockArticle, mockAnswer, nil, logger)

	mux := http.NewServeMux()
	path, connectHandler := morningletterv2connect.NewMorningLetterServiceHandler(handler)
	mux.Handle(path, connectHandler)
	server := httptest.NewServer(mux)
	defer server.Close()

	client := morningletterv2connect.NewMorningLetterServiceClient(server.Client(), server.URL)

	t.Run("missing X-Alt-User-Id header", func(t *testing.T) {
		req := connect.NewRequest(&morningletterv2.StreamChatRequest{
			Messages: []*morningletterv2.ChatMessage{
				{Role: "user", Content: "おはようございます"},
			},
		})

		stream, err := client.StreamChat(context.Background(), req)
		require.NoError(t, err)

		hasMsg := stream.Receive()
		assert.False(t, hasMsg)
		require.Error(t, stream.Err())
		assert.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(stream.Err()))
	})

	t.Run("invalid X-Alt-User-Id header", func(t *testing.T) {
		req := connect.NewRequest(&morningletterv2.StreamChatRequest{
			Messages: []*morningletterv2.ChatMessage{
				{Role: "user", Content: "おはようございます"},
			},
		})
		req.Header().Set("X-Alt-User-Id", "not-a-valid-uuid")

		stream, err := client.StreamChat(context.Background(), req)
		require.NoError(t, err)

		hasMsg := stream.Receive()
		assert.False(t, hasMsg)
		require.Error(t, stream.Err())
		assert.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(stream.Err()))
	})
}

func TestMorningLetterHandler_StreamChat_PassesUserID(t *testing.T) {
	mockArticle := new(mockArticleClient)
	mockAnswer := new(mockAnswerWithRAGUsecase)
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))

	handler := morning_letter.NewHandler(mockArticle, mockAnswer, nil, logger)

	mux := http.NewServeMux()
	path, connectHandler := morningletterv2connect.NewMorningLetterServiceHandler(handler)
	mux.Handle(path, connectHandler)
	server := httptest.NewServer(mux)
	defer server.Close()

	client := morningletterv2connect.NewMorningLetterServiceClient(server.Client(), server.URL)

	userID := uuid.New()
	req := connect.NewRequest(&morningletterv2.StreamChatRequest{
		Messages: []*morningletterv2.ChatMessage{
			{Role: "user", Content: "今日のニュースは？"},
		},
	})
	req.Header().Set("X-Alt-User-Id", userID.String())

	events := make(chan usecase.StreamEvent, 2)
	events <- usecase.StreamEvent{Kind: usecase.StreamEventKindDelta, Payload: "ニュースのまとめです"}
	events <- usecase.StreamEvent{Kind: usecase.StreamEventKindDone, Payload: &usecase.AnswerWithRAGOutput{
		Answer: "ニュースのまとめです",
	}}
	close(events)

	mockArticle.On("GetRecentArticles", mock.Anything, mock.Anything, mock.Anything).
		Return([]domain.ArticleMetadata{{ID: uuid.New()}}, nil)

	mockAnswer.On("Stream", mock.Anything, mock.MatchedBy(func(in usecase.AnswerWithRAGInput) bool {
		return in.UserID == userID.String() && in.Query == "今日のニュースは？"
	})).Return((<-chan usecase.StreamEvent)(events))

	stream, err := client.StreamChat(context.Background(), req)
	require.NoError(t, err)

	// First event is meta event sent by handler
	require.True(t, stream.Receive())
	metaResp := stream.Msg()
	assert.Equal(t, "meta", metaResp.Kind)

	// Second event is delta
	require.True(t, stream.Receive())
	deltaResp := stream.Msg()
	assert.Equal(t, "delta", deltaResp.Kind)

	// Third event is done
	require.True(t, stream.Receive())
	doneResp := stream.Msg()
	assert.Equal(t, "done", doneResp.Kind)

	assert.False(t, stream.Receive())
	assert.NoError(t, stream.Err())

	mockAnswer.AssertExpectations(t)
}
