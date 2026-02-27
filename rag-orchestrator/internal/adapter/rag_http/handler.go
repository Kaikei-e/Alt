package rag_http

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"rag-orchestrator/internal/adapter/rag_http/openapi"
	"rag-orchestrator/internal/domain"
	"rag-orchestrator/internal/usecase"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

// EmbedderFactory creates a VectorEncoder from a URL.
type EmbedderFactory func(url string, model string, timeout int) domain.VectorEncoder

// IndexUsecaseFactory creates an IndexArticleUsecase with a given encoder.
type IndexUsecaseFactory func(encoder domain.VectorEncoder) usecase.IndexArticleUsecase

type Handler struct {
	retrieveUsecase      usecase.RetrieveContextUsecase
	answerUsecase        usecase.AnswerWithRAGUsecase
	indexUsecase         usecase.IndexArticleUsecase
	jobRepo              domain.RagJobRepository
	morningLetterUsecase usecase.MorningLetterUsecase

	// For hyper-boost support
	embedderFactory     EmbedderFactory
	indexUsecaseFactory IndexUsecaseFactory
	embeddingModel      string
	embedderTimeout     int
}

func mapAnswerRequestToInput(req openapi.AnswerRequest) usecase.AnswerWithRAGInput {
	input := usecase.AnswerWithRAGInput{
		Query: req.Query,
	}
	if req.CandidateArticleIds != nil {
		input.CandidateArticleIDs = *req.CandidateArticleIds
	}
	if req.Locale != nil {
		input.Locale = *req.Locale
	}
	if req.UserId != nil {
		input.UserID = *req.UserId
	}
	if req.MaxChunks != nil {
		input.MaxChunks = int(*req.MaxChunks)
	}
	if req.MaxTokens != nil {
		input.MaxTokens = int(*req.MaxTokens)
	}
	return input
}

// HandlerOption configures the Handler.
type HandlerOption func(*Handler)

// WithEmbedderOverride enables X-Embedder-URL header support for hyper-boost.
func WithEmbedderOverride(
	embedderFactory EmbedderFactory,
	indexUsecaseFactory IndexUsecaseFactory,
	embeddingModel string,
	embedderTimeout int,
) HandlerOption {
	return func(h *Handler) {
		h.embedderFactory = embedderFactory
		h.indexUsecaseFactory = indexUsecaseFactory
		h.embeddingModel = embeddingModel
		h.embedderTimeout = embedderTimeout
	}
}

func NewHandler(
	retrieveUsecase usecase.RetrieveContextUsecase,
	answerUsecase usecase.AnswerWithRAGUsecase,
	indexUsecase usecase.IndexArticleUsecase,
	jobRepo domain.RagJobRepository,
	morningLetterUsecase usecase.MorningLetterUsecase,
	opts ...HandlerOption,
) *Handler {
	h := &Handler{
		retrieveUsecase:      retrieveUsecase,
		answerUsecase:        answerUsecase,
		indexUsecase:         indexUsecase,
		jobRepo:              jobRepo,
		morningLetterUsecase: morningLetterUsecase,
	}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

// Ensure Handler implements ServerInterface
var _ openapi.ServerInterface = (*Handler)(nil)

// Delete or tombstone an article from the index
// (POST /internal/rag/index/delete)
func (h *Handler) DeleteIndex(ctx echo.Context) error {
	return ctx.JSON(http.StatusNotImplemented, map[string]string{"status": "not implemented"})
}

const upsertTimeout = 90 * time.Second

// Upsert an article to the RAG index
// (POST /internal/rag/index/upsert)
func (h *Handler) UpsertIndex(ctx echo.Context) error {
	var req openapi.UpsertIndexRequest
	if err := ctx.Bind(&req); err != nil {
		return ctx.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request"})
	}

	// Server-side timeout decoupled from caller's context
	timeoutCtx, cancel := context.WithTimeout(ctx.Request().Context(), upsertTimeout)
	defer cancel()

	// Check for embedder override (hyper-boost)
	indexUsecase := h.indexUsecase
	if embedderURL := ctx.Request().Header.Get("X-Embedder-URL"); embedderURL != "" && h.embedderFactory != nil && h.indexUsecaseFactory != nil {
		encoder := h.embedderFactory(embedderURL, h.embeddingModel, h.embedderTimeout)
		indexUsecase = h.indexUsecaseFactory(encoder)
	}

	// Index the article with all required fields from the request
	if err := indexUsecase.Upsert(
		timeoutCtx,
		req.ArticleId,
		req.Title,
		req.Url, // URL is a required field per OpenAPI spec
		req.Body,
	); err != nil {
		return ctx.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	return ctx.JSON(http.StatusOK, map[string]string{"status": "indexed"})
}

// Answer a query using RAG (with LLM generation)
// (POST /v1/rag/answer)
func (h *Handler) AnswerWithRAG(ctx echo.Context) error {
	var req openapi.AnswerRequest
	if err := ctx.Bind(&req); err != nil {
		return ctx.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request"})
	}

	input := usecase.AnswerWithRAGInput{
		Query: req.Query,
	}
	if req.CandidateArticleIds != nil {
		input.CandidateArticleIDs = *req.CandidateArticleIds
	}
	if req.Locale != nil {
		input.Locale = *req.Locale
	}
	if req.UserId != nil {
		input.UserID = *req.UserId
	}
	if req.MaxChunks != nil {
		input.MaxChunks = int(*req.MaxChunks)
	}
	if req.MaxTokens != nil {
		input.MaxTokens = int(*req.MaxTokens)
	}

	output, err := h.answerUsecase.Execute(ctx.Request().Context(), input)
	if err != nil {
		return ctx.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	contexts := make([]openapi.Context, 0, len(output.Contexts))
	for _, c := range output.Contexts {
		chunkText := c.ChunkText
		url := c.URL
		title := c.Title
		score := float32(c.Score)
		docVer := int64(c.DocumentVersion)
		chunkID := c.ChunkID.String()

		var pubAt *time.Time
		if c.PublishedAt != "" {
			if parsed, perr := time.Parse(time.RFC3339, c.PublishedAt); perr == nil {
				pubAt = &parsed
			}
		}

		contexts = append(contexts, openapi.Context{
			ChunkText:       &chunkText,
			Url:             &url,
			Title:           &title,
			PublishedAt:     pubAt,
			Score:           &score,
			DocumentVersion: &docVer,
			ChunkId:         &chunkID,
		})
	}

	citations := make([]openapi.AnswerCitation, 0, len(output.Citations))
	for _, cite := range output.Citations {
		chunkID := cite.ChunkID
		chunkText := cite.ChunkText
		url := cite.URL
		title := cite.Title
		score := float32(cite.Score)
		docVer := int64(cite.DocumentVersion)

		citations = append(citations, openapi.AnswerCitation{
			ChunkId:         &chunkID,
			ChunkText:       &chunkText,
			Url:             &url,
			Title:           &title,
			Score:           &score,
			DocumentVersion: &docVer,
		})
	}

	var answerPtr *string
	if !output.Fallback && output.Answer != "" {
		answerPtr = &output.Answer
	}

	fallback := output.Fallback
	var reasonPtr *string
	if output.Reason != "" {
		reasonPtr = &output.Reason
	}
	debug := openapi.AnswerDebug{
		RetrievalSetId: &output.Debug.RetrievalSetID,
		PromptVersion:  &output.Debug.PromptVersion,
	}

	var citationsPtr *[]openapi.AnswerCitation
	if len(citations) > 0 {
		citationsPtr = &citations
	}

	return ctx.JSON(http.StatusOK, openapi.AnswerResponse{
		Answer:    answerPtr,
		Contexts:  &contexts,
		Citations: citationsPtr,
		Fallback:  &fallback,
		Reason:    reasonPtr,
		Debug:     &debug,
	})
}

// AnswerWithRAGStream streams a RAG answer using Server-Sent Events.
// (POST /v1/rag/answer/stream)
func (h *Handler) AnswerWithRAGStream(ctx echo.Context) error {
	var req openapi.AnswerRequest
	if err := ctx.Bind(&req); err != nil {
		return ctx.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request"})
	}

	input := mapAnswerRequestToInput(req)
	events := h.answerUsecase.Stream(ctx.Request().Context(), input)

	res := ctx.Response()
	res.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	res.Header().Set("Cache-Control", "no-cache, no-transform")
	res.Header().Set("Connection", "keep-alive")

	flusher, ok := res.Writer.(http.Flusher)
	if !ok {
		return ctx.JSON(http.StatusInternalServerError, map[string]string{"error": "streaming not supported"})
	}
	flusher.Flush()

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Request().Context().Done():
			return nil
		case event, ok := <-events:
			if !ok {
				return nil
			}
			if err := writeSSE(res.Writer, event.Kind, event.Payload); err != nil {
				return err
			}
			flusher.Flush()
			if event.Kind == usecase.StreamEventKindDone || event.Kind == usecase.StreamEventKindFallback {
				return nil
			}
		case <-ticker.C:
			if _, err := io.WriteString(res.Writer, ":\n\n"); err != nil {
				return err
			}
			flusher.Flush()
		}
	}
}

// Backfill enqueues an article for indexing
// (POST /internal/rag/backfill)
func (h *Handler) Backfill(ctx echo.Context) error {
	var body map[string]interface{}
	if err := ctx.Bind(&body); err != nil {
		return ctx.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request"})
	}

	// Validate required fields
	if _, ok := body["article_id"]; !ok {
		return ctx.JSON(http.StatusBadRequest, map[string]string{"error": "missing article_id"})
	}
	if _, ok := body["title"]; !ok {
		return ctx.JSON(http.StatusBadRequest, map[string]string{"error": "missing title"})
	}
	if _, ok := body["body"]; !ok {
		return ctx.JSON(http.StatusBadRequest, map[string]string{"error": "missing body"})
	}

	job := &domain.RagJob{
		ID:        uuid.New(),
		JobType:   "backfill_article",
		Payload:   body,
		Status:    "new",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := h.jobRepo.Enqueue(ctx.Request().Context(), job); err != nil {
		return ctx.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	return ctx.JSON(http.StatusAccepted, map[string]string{"job_id": job.ID.String(), "status": "queued"})
}

// Retrieve context for a query (Retrieve-Only)
// (POST /v1/rag/retrieve)
func (h *Handler) RetrieveContext(ctx echo.Context) error {
	var req openapi.RetrieveRequest
	if err := ctx.Bind(&req); err != nil {
		return ctx.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request"})
	}

	input := usecase.RetrieveContextInput{
		Query: req.Query,
	}
	if req.CandidateArticleIds != nil {
		input.CandidateArticleIDs = *req.CandidateArticleIds
	}

	output, err := h.retrieveUsecase.Execute(ctx.Request().Context(), input)
	if err != nil {
		// Differentiate errors ideally
		return ctx.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	contexts := make([]openapi.Context, 0, len(output.Contexts))
	for _, c := range output.Contexts {
		score := float32(c.Score)
		docVer := int64(c.DocumentVersion)

		var pubAt *time.Time
		if c.PublishedAt != "" {
			if t, err := time.Parse(time.RFC3339, c.PublishedAt); err == nil {
				pubAt = &t
			}
		}

		contexts = append(contexts, openapi.Context{
			ChunkText:       &c.ChunkText,
			Url:             &c.URL,
			Title:           &c.Title,
			PublishedAt:     pubAt,
			Score:           &score,
			DocumentVersion: &docVer,
		})
	}

	return ctx.JSON(http.StatusOK, openapi.RetrieveResponse{
		Contexts: &contexts,
	})
}

func writeSSE(w io.Writer, kind usecase.StreamEventKind, payload interface{}) error {
	if _, err := fmt.Fprintf(w, "event: %s\n", kind); err != nil {
		return err
	}

	var data string
	switch v := payload.(type) {
	case nil:
		data = ""
	case string:
		data = v
	case []byte:
		data = string(v)
	default:
		bytes, err := json.Marshal(v)
		if err != nil {
			return err
		}
		data = string(bytes)
	}

	for _, line := range strings.Split(data, "\n") {
		if _, err := fmt.Fprintf(w, "data: %s\n", line); err != nil {
			return err
		}
	}

	if _, err := io.WriteString(w, "\n"); err != nil {
		return err
	}

	return nil
}

// MorningLetterRequest defines the request for morning letter
type MorningLetterRequest struct {
	Query       string `json:"query"`
	WithinHours *int   `json:"within_hours,omitempty"`
	TopicLimit  *int   `json:"topic_limit,omitempty"`
	Locale      string `json:"locale,omitempty"`
}

// MorningLetterResponse defines the response for morning letter
type MorningLetterResponse struct {
	Topics          []TopicSummary     `json:"topics"`
	TimeWindow      TimeWindowResponse `json:"time_window"`
	ArticlesScanned int                `json:"articles_scanned"`
	GenerationInfo  GenerationInfo     `json:"generation_info"`
}

// TopicSummary represents a summarized topic
type TopicSummary struct {
	Topic       string       `json:"topic"`
	Headline    string       `json:"headline"`
	Summary     string       `json:"summary"`
	Importance  float32      `json:"importance"`
	ArticleRefs []ArticleRef `json:"article_refs"`
	Keywords    []string     `json:"keywords"`
}

// ArticleRef is a lightweight reference to a source article
type ArticleRef struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	URL         string    `json:"url"`
	PublishedAt time.Time `json:"published_at"`
}

// TimeWindowResponse represents the time range for the query
type TimeWindowResponse struct {
	Since time.Time `json:"since"`
	Until time.Time `json:"until"`
}

// GenerationInfo contains info about the LLM generation
type GenerationInfo struct {
	Model    string `json:"model"`
	Fallback bool   `json:"fallback"`
}

// MorningLetter extracts important topics from recent articles
// (POST /v1/rag/morning-letter)
func (h *Handler) MorningLetter(ctx echo.Context) error {
	var req MorningLetterRequest
	if err := ctx.Bind(&req); err != nil {
		return ctx.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request"})
	}

	if req.Query == "" {
		return ctx.JSON(http.StatusBadRequest, map[string]string{"error": "query is required"})
	}

	input := usecase.MorningLetterInput{
		Query:  req.Query,
		Locale: req.Locale,
	}
	if req.WithinHours != nil {
		input.WithinHours = *req.WithinHours
	}
	if req.TopicLimit != nil {
		input.TopicLimit = *req.TopicLimit
	}

	output, err := h.morningLetterUsecase.Execute(ctx.Request().Context(), input)
	if err != nil {
		return ctx.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	// Map domain types to response types
	topics := make([]TopicSummary, len(output.Topics))
	for i, t := range output.Topics {
		refs := make([]ArticleRef, len(t.ArticleRefs))
		for j, r := range t.ArticleRefs {
			refs[j] = ArticleRef{
				ID:          r.ID.String(),
				Title:       r.Title,
				URL:         r.URL,
				PublishedAt: r.PublishedAt,
			}
		}
		topics[i] = TopicSummary{
			Topic:       t.Topic,
			Headline:    t.Headline,
			Summary:     t.Summary,
			Importance:  t.Importance,
			ArticleRefs: refs,
			Keywords:    t.Keywords,
		}
	}

	return ctx.JSON(http.StatusOK, MorningLetterResponse{
		Topics: topics,
		TimeWindow: TimeWindowResponse{
			Since: output.TimeWindow.Since,
			Until: output.TimeWindow.Until,
		},
		ArticlesScanned: output.ArticlesScanned,
		GenerationInfo: GenerationInfo{
			Model:    output.GenerationInfo.Model,
			Fallback: output.GenerationInfo.Fallback,
		},
	})
}
