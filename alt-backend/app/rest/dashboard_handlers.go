package rest

import (
	"alt/di"
	dashboard_usecase "alt/usecase/dashboard"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
)

func registerDashboardRoutes(v1 *echo.Group, container *di.ApplicationComponents) {
	v1.GET("/dashboard/metrics", handleGetMetrics(container.DashboardMetricsUsecase))
	v1.GET("/dashboard/overview", handleGetOverview(container.DashboardMetricsUsecase))
	v1.GET("/dashboard/logs", handleGetLogs(container.DashboardMetricsUsecase))
	v1.GET("/dashboard/jobs", handleGetJobs(container.DashboardMetricsUsecase))
	v1.GET("/dashboard/recap_jobs", handleGetRecapJobs(container.GetRecapJobsUsecase))
}

func handleGetMetrics(usecase *dashboard_usecase.DashboardMetricsUsecase) echo.HandlerFunc {
	return func(c echo.Context) error {
		metricType := c.QueryParam("type")
		windowSeconds, _ := strconv.ParseInt(c.QueryParam("window"), 10, 64)
		if windowSeconds <= 0 {
			windowSeconds = 14400 // デフォルト4時間
		}
		limit, _ := strconv.ParseInt(c.QueryParam("limit"), 10, 64)
		if limit <= 0 {
			limit = 500
		}

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
		windowSeconds, _ := strconv.ParseInt(c.QueryParam("window"), 10, 64)
		if windowSeconds <= 0 {
			windowSeconds = 14400 // デフォルト4時間
		}
		limit, _ := strconv.ParseInt(c.QueryParam("limit"), 10, 64)
		if limit <= 0 {
			limit = 200
		}

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
		windowSeconds, _ := strconv.ParseInt(c.QueryParam("window"), 10, 64)
		if windowSeconds <= 0 {
			windowSeconds = 14400 // デフォルト4時間
		}
		limit, _ := strconv.ParseInt(c.QueryParam("limit"), 10, 64)
		if limit <= 0 {
			limit = 2000
		}

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
		windowSeconds, _ := strconv.ParseInt(c.QueryParam("window"), 10, 64)
		if windowSeconds <= 0 {
			windowSeconds = 14400 // デフォルト4時間
		}
		limit, _ := strconv.ParseInt(c.QueryParam("limit"), 10, 64)
		if limit <= 0 {
			limit = 200
		}

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
		windowSeconds, _ := strconv.ParseInt(c.QueryParam("window"), 10, 64)
		if windowSeconds <= 0 {
			windowSeconds = 14400 // Default 4 hours
		}
		limit, _ := strconv.ParseInt(c.QueryParam("limit"), 10, 64)
		if limit <= 0 {
			limit = 200
		}

		jobs, err := usecase.Execute(c.Request().Context(), windowSeconds, limit)
		if err != nil {
			return HandleError(c, err, "GetRecapJobs")
		}

		c.Response().Header().Set("Content-Type", "application/json")
		return c.JSON(http.StatusOK, jobs)
	}
}
