package rag_http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"rag-orchestrator/internal/adapter/rag_http"
	"rag-orchestrator/internal/adapter/rag_http/openapi"
	"rag-orchestrator/internal/domain"
	"rag-orchestrator/internal/usecase"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
)

type dummyRetrieveUsecase struct {
	response *usecase.RetrieveContextOutput
}

func (d *dummyRetrieveUsecase) Execute(ctx context.Context, input usecase.RetrieveContextInput) (*usecase.RetrieveContextOutput, error) {
	return d.response, nil
}

type stubLLMClient struct {
	response *domain.LLMResponse
}

func (s *stubLLMClient) Generate(ctx context.Context, prompt string, maxTokens int) (*domain.LLMResponse, error) {
	return s.response, nil
}

func (s *stubLLMClient) Version() string { return "stub" }

func (s *stubLLMClient) GenerateStream(ctx context.Context, prompt string, maxTokens int) (<-chan domain.LLMStreamChunk, <-chan error, error) {
	return nil, nil, errors.New("streaming not implemented")
}

func (s *stubLLMClient) Chat(ctx context.Context, messages []domain.Message, maxTokens int) (*domain.LLMResponse, error) {
	return s.response, nil
}

func (s *stubLLMClient) ChatStream(ctx context.Context, messages []domain.Message, maxTokens int) (<-chan domain.LLMStreamChunk, <-chan error, error) {
	return nil, nil, errors.New("streaming not implemented")
}

type stubStreamUsecase struct {
	events <-chan usecase.StreamEvent
}

func (s *stubStreamUsecase) Execute(ctx context.Context, input usecase.AnswerWithRAGInput) (*usecase.AnswerWithRAGOutput, error) {
	return nil, nil
}

func (s *stubStreamUsecase) Stream(ctx context.Context, input usecase.AnswerWithRAGInput) <-chan usecase.StreamEvent {
	return s.events
}

func TestHandler_AnswerWithRAG_TPU(t *testing.T) {
	e := echo.New()

	chunkID := uuid.New()
	retrieve := &dummyRetrieveUsecase{
		response: &usecase.RetrieveContextOutput{
			Contexts: []usecase.ContextItem{
				{
					ChunkID:         chunkID,
					ChunkText:       "TPU provides high throughput for matrix multiplies.",
					URL:             "https://example.com/tpu",
					Title:           "TPU overview",
					PublishedAt:     "2025-12-25T00:00:00Z",
					Score:           0.9,
					DocumentVersion: 1,
				},
			},
		},
	}

	llmResponse := &domain.LLMResponse{
		Text: `{
  "quotes": [{"chunk_id":"` + chunkID.String() + `","quote":"TPU excels at GPUM-style matrix lods."}],
  "answer": "TPUはGoogleの専用加速装置で、浮動小数点行列を低コストで並列処理します。[` + chunkID.String() + `]",
  "citations": [{"chunk_id":"` + chunkID.String() + `","url":"https://example.com/tpu","title":"TPU overview","score":0.9,"document_version":1}],
  "fallback": false,
  "reason": ""
}`,
		Done: true,
	}

	testLogger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	answerUC := usecase.NewAnswerWithRAGUsecase(
		retrieve,
		usecase.NewXMLPromptBuilder("Answer in Japanese."),
		&stubLLMClient{response: llmResponse},
		usecase.NewOutputValidator(),
		5,
		256,
		"alpha-v1",
		"ja",
		testLogger,
	)

	handler := rag_http.NewHandler(retrieve, answerUC, nil, nil, nil)

	body := bytes.NewBufferString(`{"query":"TPU"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/rag/answer", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if assert.NoError(t, handler.AnswerWithRAG(c)) {
		assert.Equal(t, http.StatusOK, rec.Code)

		var resp openapi.AnswerResponse
		assert.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		assert.NotNil(t, resp.Answer)
		assert.False(t, *resp.Fallback)
		assert.NotNil(t, resp.Citations)
		assert.Equal(t, 1, len(*resp.Citations))
		assert.Equal(t, chunkID.String(), *(*resp.Citations)[0].ChunkId)
	}
}

func TestHandler_AnswerWithRAGStream(t *testing.T) {
	e := echo.New()

	events := make(chan usecase.StreamEvent, 3)
	finalOutput := &usecase.AnswerWithRAGOutput{
		Answer:    "streamed answer",
		Citations: nil,
		Contexts:  nil,
		Fallback:  false,
		Reason:    "",
		Debug: usecase.AnswerDebug{
			RetrievalSetID: "stream-1",
			PromptVersion:  "alpha-v1",
		},
	}
	events <- usecase.StreamEvent{
		Kind: usecase.StreamEventKindMeta,
		Payload: usecase.StreamMeta{
			Contexts: []usecase.ContextItem{},
			Debug:    finalOutput.Debug,
		},
	}
	events <- usecase.StreamEvent{
		Kind:    usecase.StreamEventKindDelta,
		Payload: "chunked",
	}
	events <- usecase.StreamEvent{
		Kind:    usecase.StreamEventKindDone,
		Payload: finalOutput,
	}
	close(events)

	handler := rag_http.NewHandler(nil, &stubStreamUsecase{events: events}, nil, nil, nil)

	body := bytes.NewBufferString(`{"query":"streaming"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/rag/answer/stream", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if assert.NoError(t, handler.AnswerWithRAGStream(c)) {
		assert.Equal(t, http.StatusOK, rec.Code)
		response := rec.Body.String()
		assert.Contains(t, response, "event: meta")
		assert.Contains(t, response, "event: delta")
		assert.Contains(t, response, "event: done")
		assert.Contains(t, response, `"Answer":"streamed answer"`)
	}
}

// dummyIndexUsecase captures the parameters passed to Upsert
type dummyIndexUsecase struct {
	capturedURL   string
	capturedTitle string
	returnError   error
}

func (d *dummyIndexUsecase) Upsert(ctx context.Context, articleID, title, url, body string) error {
	d.capturedURL = url
	d.capturedTitle = title
	return d.returnError
}

func (d *dummyIndexUsecase) Delete(ctx context.Context, articleID string) error {
	return nil
}

func TestUpsertIndex_PassesUrlToUsecase(t *testing.T) {
	e := echo.New()
	dummy := &dummyIndexUsecase{}
	handler := rag_http.NewHandler(nil, nil, dummy, nil, nil)

	// Prepare request with URL field populated
	reqBody := openapi.UpsertIndexRequest{
		ArticleId: "test-article-123",
		Title:     "Test Article Title",
		Url:       "https://example.com/test-article",
		Body:      "This is test article content for verification.",
		UserId:    "user-456",
	}

	bodyBytes, err := json.Marshal(reqBody)
	assert.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/v1/rag/index/upsert", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	// Execute handler
	err = handler.UpsertIndex(c)

	// Verify URL was passed to usecase (not empty string)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "https://example.com/test-article", dummy.capturedURL, "URL should be passed from request to usecase")
	assert.Equal(t, "Test Article Title", dummy.capturedTitle, "Title should be passed correctly")
}

func TestUpsertIndex_ReturnsErrorWhenUsecaseFails(t *testing.T) {
	e := echo.New()
	dummy := &dummyIndexUsecase{
		returnError: errors.New("indexing failed"),
	}
	handler := rag_http.NewHandler(nil, nil, dummy, nil, nil)

	reqBody := openapi.UpsertIndexRequest{
		ArticleId: "test-article-123",
		Title:     "Test Article",
		Url:       "https://example.com/article",
		Body:      "Content",
		UserId:    "user-456",
	}

	bodyBytes, err := json.Marshal(reqBody)
	assert.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/v1/rag/index/upsert", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err = handler.UpsertIndex(c)

	assert.NoError(t, err) // handler doesn't return error, but sends error response
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}
