package rag_http

import (
	"net/http"

	"rag-orchestrator/internal/adapter/rag_http/openapi"

	"github.com/labstack/echo/v4"
)

type Handler struct {
}

func NewHandler() *Handler {
	return &Handler{}
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
	return ctx.JSON(http.StatusNotImplemented, map[string]string{"status": "not implemented"})
}
