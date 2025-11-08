package rest

import (
	"errors"
	"fmt"
	"net/http"

	"alt/domain"
	"alt/usecase/recap_usecase"

	"github.com/labstack/echo/v4"
)

type RecapHandler struct {
	recapUsecase *recap_usecase.RecapUsecase
}

func NewRecapHandler(recapUsecase *recap_usecase.RecapUsecase) *RecapHandler {
	return &RecapHandler{
		recapUsecase: recapUsecase,
	}
}

func (h *RecapHandler) GetSevenDayRecap(c echo.Context) error {
	ctx := c.Request().Context()

	recap, err := h.recapUsecase.GetSevenDayRecap(ctx)
	if err != nil {
		if errors.Is(err, domain.ErrRecapNotFound) {
			return c.JSON(http.StatusNotFound, map[string]string{
				"error": "No 7-day recap available yet",
			})
		}
		return handleError(c, fmt.Errorf("failed to fetch 7-day recap: %w", err), "recap_summary")
	}

	return c.JSON(http.StatusOK, recap)
}
