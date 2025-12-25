package rag_http

import (
	"net/http"
	"rag-orchestrator/internal/adapter/rag_http/openapi"
	"rag-orchestrator/internal/domain"
	"rag-orchestrator/internal/usecase"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

type Handler struct {
	retrieveUsecase usecase.RetrieveContextUsecase
	answerUsecase   usecase.AnswerWithRAGUsecase
	jobRepo         domain.RagJobRepository
}

func NewHandler(
	retrieveUsecase usecase.RetrieveContextUsecase,
	answerUsecase usecase.AnswerWithRAGUsecase,
	jobRepo domain.RagJobRepository,
) *Handler {
	return &Handler{
		retrieveUsecase: retrieveUsecase,
		answerUsecase:   answerUsecase,
		jobRepo:         jobRepo,
	}
}

// Ensure Handler implements ServerInterface
var _ openapi.ServerInterface = (*Handler)(nil)

// Delete or tombstone an article from the index
// (POST /internal/rag/index/delete)
func (h *Handler) DeleteIndex(ctx echo.Context) error {
	return ctx.JSON(http.StatusNotImplemented, map[string]string{"status": "not implemented"})
}

// Upsert an article to the RAG index
// (POST /internal/rag/index/upsert)
func (h *Handler) UpsertIndex(ctx echo.Context) error {
	return ctx.JSON(http.StatusNotImplemented, map[string]string{"status": "not implemented"})
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
