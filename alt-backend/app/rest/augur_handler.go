package rest

import (
	"alt/di"
	"alt/usecase/retrieve_context_usecase"
	"net/http"

	"github.com/labstack/echo/v4"
)

type AugurHandler struct {
	retrieveContextUsecase retrieve_context_usecase.RetrieveContextUsecase
}

func NewAugurHandler(retrieveContextUsecase retrieve_context_usecase.RetrieveContextUsecase) *AugurHandler {
	return &AugurHandler{
		retrieveContextUsecase: retrieveContextUsecase,
	}
}

func (h *AugurHandler) RetrieveContext(c echo.Context) error {
	query := c.QueryParam("q")
	if query == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "query parameter 'q' is required"})
	}

	contexts, err := h.retrieveContextUsecase.Execute(c.Request().Context(), query)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"contexts": contexts,
	})
}

func RegisterAugurRoutes(g *echo.Group, container *di.ApplicationComponents) {
	handler := NewAugurHandler(container.RetrieveContextUsecase)
	g.GET("/rag/context", handler.RetrieveContext)
}
