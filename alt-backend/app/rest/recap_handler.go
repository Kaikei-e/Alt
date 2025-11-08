package rest

import (
	"net/http"

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
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Failed to fetch 7-day recap",
		})
	}

	return c.JSON(http.StatusOK, recap)
}
