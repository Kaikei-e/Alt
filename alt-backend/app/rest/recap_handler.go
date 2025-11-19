package rest

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"alt/domain"
	"alt/utils/logger"

	"github.com/labstack/echo/v4"
)

const clusterDraftHeader = "X-Genre-Draft-Id"

type recapService interface {
	GetSevenDayRecap(ctx context.Context) (*domain.RecapSummary, error)
}

type clusterDraftProvider interface {
	LoadDraft(draftID string) (*domain.ClusterDraft, error)
}

type RecapHandler struct {
	recapService         recapService
	clusterDraftProvider clusterDraftProvider
}

func NewRecapHandler(recapService recapService, provider clusterDraftProvider) *RecapHandler {
	return &RecapHandler{
		recapService:         recapService,
		clusterDraftProvider: provider,
	}
}

func (h *RecapHandler) GetSevenDayRecap(c echo.Context) error {
	ctx := c.Request().Context()

	recap, err := h.recapService.GetSevenDayRecap(ctx)
	if err != nil {
		if errors.Is(err, domain.ErrRecapNotFound) {
			return c.JSON(http.StatusNotFound, map[string]string{
				"error": "No 7-day recap available yet",
			})
		}
		return handleError(c, fmt.Errorf("failed to fetch 7-day recap: %w", err), "recap_summary")
	}

	h.attachClusterDraft(c, recap)
	return c.JSON(http.StatusOK, recap)
}

func (h *RecapHandler) attachClusterDraft(c echo.Context, recap *domain.RecapSummary) {
	if recap == nil || h.clusterDraftProvider == nil {
		return
	}

	draftID := c.Request().Header.Get(clusterDraftHeader)
	if draftID == "" {
		return
	}

	draft, err := h.clusterDraftProvider.LoadDraft(draftID)
	if err != nil {
		logger.Logger.Warn("cluster draft loader failed", "error", err, "draft_id", draftID)
		return
	}

	if draft != nil {
		recap.ClusterDraft = draft
	}
}
