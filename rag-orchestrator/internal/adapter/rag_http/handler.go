package rag_http

import (
	"net/http"
	"rag-orchestrator/internal/adapter/rag_http/openapi"
	"rag-orchestrator/internal/usecase"
	"time"

	"github.com/labstack/echo/v4"
)

type Handler struct {
	retrieveUsecase usecase.RetrieveContextUsecase
}

func NewHandler(retrieveUsecase usecase.RetrieveContextUsecase) *Handler {
	return &Handler{
		retrieveUsecase: retrieveUsecase,
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
	return ctx.JSON(http.StatusNotImplemented, map[string]string{"status": "not implemented"})
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
