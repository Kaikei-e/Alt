package rest

import (
	"alt/config"
	"alt/di"
	middleware_custom "alt/middleware"
	dashboard_usecase "alt/usecase/dashboard"
	"alt/utils/logger"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
)

// L-002: clamp helpers protect dashboard usecases from query-string DoS.
// Even an authenticated admin should not be able to ask for `limit=999999999`
// — defense in depth in case an admin token is compromised.
const (
	dashboardMetricsLimitDefault  = int64(500)
	dashboardOverviewLimitDefault = int64(200)
	dashboardLogsLimitDefault     = int64(500) // was 2000 before L-002
	dashboardJobsLimitDefault     = int64(200)
	dashboardRecapLimitDefault    = int64(200)
	dashboardLimitMax             = int64(1000)
	dashboardWindowDefault        = int64(14400)            // 4 hours
	dashboardWindowMax            = int64(7 * 24 * 60 * 60) // 7 days
)

// clampLimit returns a value in [1, ceiling]; non-positive values return def.
func clampLimit(value, ceiling, def int64) int64 {
	if value <= 0 {
		return def
	}
	if value > ceiling {
		return ceiling
	}
	return value
}

// clampWindowSeconds bounds the time window so log/metrics queries cannot
// scan an attacker-defined arbitrarily long range.
func clampWindowSeconds(value, def, ceiling int64) int64 {
	if value <= 0 {
		return def
	}
	if value > ceiling {
		return ceiling
	}
	return value
}

// registerDashboardRoutes wires the admin-only dashboard endpoints.
// Access requires a valid JWT (RequireAuth) AND the admin role (RequireAdmin).
// Dashboard data includes operational logs and metrics and must not be exposed
// to regular users or unauthenticated traffic.
func registerDashboardRoutes(v1 *echo.Group, container *di.ApplicationComponents, cfg *config.Config) {
	authMiddleware := middleware_custom.NewAuthMiddleware(logger.Logger, cfg)
	dashboard := v1.Group("/dashboard", authMiddleware.RequireAuth(), authMiddleware.RequireAdmin())
	dashboard.GET("/metrics", handleGetMetrics(container.DashboardMetricsUsecase))
	dashboard.GET("/overview", handleGetOverview(container.DashboardMetricsUsecase))
	dashboard.GET("/logs", handleGetLogs(container.DashboardMetricsUsecase))
	dashboard.GET("/jobs", handleGetJobs(container.DashboardMetricsUsecase))
	dashboard.GET("/recap_jobs", handleGetRecapJobs(container.GetRecapJobsUsecase))
}

func handleGetMetrics(usecase *dashboard_usecase.DashboardMetricsUsecase) echo.HandlerFunc {
	return func(c echo.Context) error {
		metricType := c.QueryParam("type")
		rawWindow, _ := strconv.ParseInt(c.QueryParam("window"), 10, 64)
		windowSeconds := clampWindowSeconds(rawWindow, dashboardWindowDefault, dashboardWindowMax)
		rawLimit, _ := strconv.ParseInt(c.QueryParam("limit"), 10, 64)
		limit := clampLimit(rawLimit, dashboardLimitMax, dashboardMetricsLimitDefault)

		data, err := usecase.GetMetrics(c.Request().Context(), metricType, windowSeconds, limit)
		if err != nil {
			return HandleError(c, err, "GetMetrics")
		}

		c.Response().Header().Set("Content-Type", "application/json")
		return c.Blob(http.StatusOK, "application/json", data)
	}
}

func handleGetOverview(usecase *dashboard_usecase.DashboardMetricsUsecase) echo.HandlerFunc {
	return func(c echo.Context) error {
		rawWindow, _ := strconv.ParseInt(c.QueryParam("window"), 10, 64)
		windowSeconds := clampWindowSeconds(rawWindow, dashboardWindowDefault, dashboardWindowMax)
		rawLimit, _ := strconv.ParseInt(c.QueryParam("limit"), 10, 64)
		limit := clampLimit(rawLimit, dashboardLimitMax, dashboardOverviewLimitDefault)

		data, err := usecase.GetOverview(c.Request().Context(), windowSeconds, limit)
		if err != nil {
			return HandleError(c, err, "GetOverview")
		}

		c.Response().Header().Set("Content-Type", "application/json")
		return c.Blob(http.StatusOK, "application/json", data)
	}
}

func handleGetLogs(usecase *dashboard_usecase.DashboardMetricsUsecase) echo.HandlerFunc {
	return func(c echo.Context) error {
		rawWindow, _ := strconv.ParseInt(c.QueryParam("window"), 10, 64)
		windowSeconds := clampWindowSeconds(rawWindow, dashboardWindowDefault, dashboardWindowMax)
		// L-002: previously defaulted to 2000 with no ceiling; bound it.
		rawLimit, _ := strconv.ParseInt(c.QueryParam("limit"), 10, 64)
		limit := clampLimit(rawLimit, dashboardLimitMax, dashboardLogsLimitDefault)

		data, err := usecase.GetLogs(c.Request().Context(), windowSeconds, limit)
		if err != nil {
			return HandleError(c, err, "GetLogs")
		}

		c.Response().Header().Set("Content-Type", "application/json")
		return c.Blob(http.StatusOK, "application/json", data)
	}
}

func handleGetJobs(usecase *dashboard_usecase.DashboardMetricsUsecase) echo.HandlerFunc {
	return func(c echo.Context) error {
		rawWindow, _ := strconv.ParseInt(c.QueryParam("window"), 10, 64)
		windowSeconds := clampWindowSeconds(rawWindow, dashboardWindowDefault, dashboardWindowMax)
		rawLimit, _ := strconv.ParseInt(c.QueryParam("limit"), 10, 64)
		limit := clampLimit(rawLimit, dashboardLimitMax, dashboardJobsLimitDefault)

		data, err := usecase.GetJobs(c.Request().Context(), windowSeconds, limit)
		if err != nil {
			return HandleError(c, err, "GetJobs")
		}

		c.Response().Header().Set("Content-Type", "application/json")
		return c.Blob(http.StatusOK, "application/json", data)
	}
}

func handleGetRecapJobs(usecase dashboard_usecase.GetRecapJobsUsecase) echo.HandlerFunc {
	return func(c echo.Context) error {
		rawWindow, _ := strconv.ParseInt(c.QueryParam("window"), 10, 64)
		windowSeconds := clampWindowSeconds(rawWindow, dashboardWindowDefault, dashboardWindowMax)
		rawLimit, _ := strconv.ParseInt(c.QueryParam("limit"), 10, 64)
		limit := clampLimit(rawLimit, dashboardLimitMax, dashboardRecapLimitDefault)

		jobs, err := usecase.Execute(c.Request().Context(), windowSeconds, limit)
		if err != nil {
			return HandleError(c, err, "GetRecapJobs")
		}

		c.Response().Header().Set("Content-Type", "application/json")
		return c.JSON(http.StatusOK, jobs)
	}
}
