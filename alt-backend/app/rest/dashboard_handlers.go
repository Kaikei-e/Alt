package rest

import (
	"alt/config"
	"alt/di"
	"alt/driver/recap_job_driver"
	"alt/gateway/dashboard_gateway"
	dashboard_usecase "alt/usecase/dashboard"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
)

func registerDashboardRoutes(v1 *echo.Group, container *di.ApplicationComponents, cfg *config.Config) {
	dashboardGateway := dashboard_gateway.NewDashboardGateway()

	v1.GET("/dashboard/metrics", handleGetMetrics(dashboardGateway))
	v1.GET("/dashboard/overview", handleGetOverview(dashboardGateway))
	v1.GET("/dashboard/logs", handleGetLogs(dashboardGateway))
	v1.GET("/dashboard/jobs", handleGetJobs(dashboardGateway))

	recapDriver := recap_job_driver.NewRecapJobGateway(cfg.Recap.WorkerURL)
	recapUsecase := dashboard_usecase.NewGetRecapJobsUsecase(recapDriver)
	v1.GET("/dashboard/recap_jobs", handleGetRecapJobs(recapUsecase))
}

func handleGetMetrics(gateway *dashboard_gateway.DashboardGateway) echo.HandlerFunc {
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

		data, err := gateway.GetMetrics(c.Request().Context(), metricType, windowSeconds, limit)
		if err != nil {
			return HandleError(c, err, "GetMetrics")
		}

		c.Response().Header().Set("Content-Type", "application/json")
		return c.Blob(http.StatusOK, "application/json", data)
	}
}

func handleGetOverview(gateway *dashboard_gateway.DashboardGateway) echo.HandlerFunc {
	return func(c echo.Context) error {
		windowSeconds, _ := strconv.ParseInt(c.QueryParam("window"), 10, 64)
		if windowSeconds <= 0 {
			windowSeconds = 14400 // デフォルト4時間
		}
		limit, _ := strconv.ParseInt(c.QueryParam("limit"), 10, 64)
		if limit <= 0 {
			limit = 200
		}

		data, err := gateway.GetOverview(c.Request().Context(), windowSeconds, limit)
		if err != nil {
			return HandleError(c, err, "GetOverview")
		}

		c.Response().Header().Set("Content-Type", "application/json")
		return c.Blob(http.StatusOK, "application/json", data)
	}
}

func handleGetLogs(gateway *dashboard_gateway.DashboardGateway) echo.HandlerFunc {
	return func(c echo.Context) error {
		windowSeconds, _ := strconv.ParseInt(c.QueryParam("window"), 10, 64)
		if windowSeconds <= 0 {
			windowSeconds = 14400 // デフォルト4時間
		}
		limit, _ := strconv.ParseInt(c.QueryParam("limit"), 10, 64)
		if limit <= 0 {
			limit = 2000
		}

		data, err := gateway.GetLogs(c.Request().Context(), windowSeconds, limit)
		if err != nil {
			return HandleError(c, err, "GetLogs")
		}

		c.Response().Header().Set("Content-Type", "application/json")
		return c.Blob(http.StatusOK, "application/json", data)
	}
}

func handleGetJobs(gateway *dashboard_gateway.DashboardGateway) echo.HandlerFunc {
	return func(c echo.Context) error {
		windowSeconds, _ := strconv.ParseInt(c.QueryParam("window"), 10, 64)
		if windowSeconds <= 0 {
			windowSeconds = 14400 // デフォルト4時間
		}
		limit, _ := strconv.ParseInt(c.QueryParam("limit"), 10, 64)
		if limit <= 0 {
			limit = 200
		}

		data, err := gateway.GetJobs(c.Request().Context(), windowSeconds, limit)
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
