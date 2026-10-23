package rest

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	echomiddleware "github.com/labstack/echo/v4/middleware"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"alt/di"
	"alt/domain"
	middleware_custom "alt/middleware"
	dashboard_usecase "alt/orchestrator/usecase/dashboard"
	"alt/utils/logger"
)

type stubDashboardMetricsPort struct {
	metricsPayload  []byte
	overviewPayload []byte
	logsPayload     []byte
	jobsPayload     []byte
	err             error
}

func (s *stubDashboardMetricsPort) GetMetrics(_ context.Context, _ string, _, _ int64) ([]byte, error) {
	return s.metricsPayload, s.err
}

func (s *stubDashboardMetricsPort) GetOverview(_ context.Context, _, _ int64) ([]byte, error) {
	return s.overviewPayload, s.err
}

func (s *stubDashboardMetricsPort) GetLogs(_ context.Context, _, _ int64) ([]byte, error) {
	return s.logsPayload, s.err
}

func (s *stubDashboardMetricsPort) GetJobs(_ context.Context, _, _ int64) ([]byte, error) {
	return s.jobsPayload, s.err
}

type stubGetRecapJobsUsecase struct {
	jobs []domain.RecapJob
	err  error
}

func (s *stubGetRecapJobsUsecase) Execute(_ context.Context, _, _ int64) ([]domain.RecapJob, error) {
	return s.jobs, s.err
}

var dashboardRoutesTable = []struct {
	name   string
	target string
}{
	{name: "GET /v1/dashboard/metrics", target: "/v1/dashboard/metrics?type=cpu&window=3600&limit=100"},
	{name: "GET /v1/dashboard/overview", target: "/v1/dashboard/overview?window=3600&limit=50"},
	{name: "GET /v1/dashboard/logs", target: "/v1/dashboard/logs?window=3600&limit=50"},
	{name: "GET /v1/dashboard/jobs", target: "/v1/dashboard/jobs?window=3600&limit=50"},
	{name: "GET /v1/dashboard/recap_jobs", target: "/v1/dashboard/recap_jobs?window=3600&limit=50"},
}

func TestDashboardRoutes_Unauthenticated_Returns401(t *testing.T) {
	logger.InitLogger()

	for _, rt := range dashboardRoutesTable {
		t.Run(rt.name, func(t *testing.T) {
			e := echo.New()
			e.Use(echomiddleware.Recover())
			v1 := e.Group("/v1")
			registerDashboardRoutes(v1, &di.ApplicationComponents{}, testAuthCfg())

			req := httptest.NewRequest(http.MethodGet, rt.target, nil)
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)

			assert.Equal(t, http.StatusUnauthorized, rec.Code)
		})
	}
}

func TestDashboardRoutes_NonAdminUserContext_Returns403(t *testing.T) {
	logger.InitLogger()
	userToken := signRESTTestToken(t, domain.UserRoleUser)

	for _, rt := range dashboardRoutesTable {
		t.Run(rt.name+" via JWT", func(t *testing.T) {
			e := echo.New()
			e.Use(echomiddleware.Recover())
			v1 := e.Group("/v1")
			registerDashboardRoutes(v1, &di.ApplicationComponents{}, testAuthCfg())

			req := httptest.NewRequest(http.MethodGet, rt.target, nil)
			req.Header.Set("X-Alt-Backend-Token", userToken)
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)

			assert.Equal(t, http.StatusForbidden, rec.Code)
		})

		t.Run(rt.name+" via domain.UserContext", func(t *testing.T) {
			e := echo.New()
			authMW := middleware_custom.NewAuthMiddleware(logger.Logger, testAuthCfg())
			dashboard := e.Group("/v1/dashboard", injectUserContextMiddleware(domain.UserRoleUser), authMW.RequireAdmin())
			dashboard.GET("/metrics", handleGetMetrics(nil))
			dashboard.GET("/overview", handleGetOverview(nil))
			dashboard.GET("/logs", handleGetLogs(nil))
			dashboard.GET("/jobs", handleGetJobs(nil))
			dashboard.GET("/recap_jobs", handleGetRecapJobs(nil))

			req := httptest.NewRequest(http.MethodGet, rt.target, nil)
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)

			assert.Equal(t, http.StatusForbidden, rec.Code)
		})
	}
}

func TestDashboardRoutes_AdminRole_SucceedsAcrossAll5Routes(t *testing.T) {
	logger.InitLogger()
	stage := "completed"
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)

	metricsPort := &stubDashboardMetricsPort{
		metricsPayload:  []byte(`{"series":[{"name":"cpu"}]}`),
		overviewPayload: []byte(`{"active_jobs":3}`),
		logsPayload:     []byte(`{"logs":[{"level":"ERROR"}]}`),
		jobsPayload:     []byte(`{"jobs":[{"id":"j1"}]}`),
	}
	recapJobsUC := &stubGetRecapJobsUsecase{
		jobs: []domain.RecapJob{
			{
				JobID:     "recap-job-1",
				Status:    "succeeded",
				LastStage: &stage,
				KickedAt:  now,
				UpdatedAt: now,
			},
		},
	}
	container := &di.ApplicationComponents{
		DashboardMetricsUsecase: dashboard_usecase.NewDashboardMetricsUsecase(metricsPort),
		GetRecapJobsUsecase:     recapJobsUC,
	}

	e := echo.New()
	v1 := e.Group("/v1")
	registerDashboardRoutes(v1, container, testAuthCfg())
	adminToken := signRESTTestToken(t, domain.UserRoleAdmin)

	t.Run("GET /v1/dashboard/metrics", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v1/dashboard/metrics?type=cpu&window=3600&limit=100", nil)
		req.Header.Set("X-Alt-Backend-Token", adminToken)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		require.Equal(t, http.StatusOK, rec.Code)
		assert.JSONEq(t, `{"series":[{"name":"cpu"}]}`, rec.Body.String())
	})

	t.Run("GET /v1/dashboard/overview", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v1/dashboard/overview?window=3600&limit=50", nil)
		req.Header.Set("X-Alt-Backend-Token", adminToken)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		require.Equal(t, http.StatusOK, rec.Code)
		assert.JSONEq(t, `{"active_jobs":3}`, rec.Body.String())
	})

	t.Run("GET /v1/dashboard/logs", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v1/dashboard/logs?window=3600&limit=50", nil)
		req.Header.Set("X-Alt-Backend-Token", adminToken)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		require.Equal(t, http.StatusOK, rec.Code)
		assert.JSONEq(t, `{"logs":[{"level":"ERROR"}]}`, rec.Body.String())
	})

	t.Run("GET /v1/dashboard/jobs", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v1/dashboard/jobs?window=3600&limit=50", nil)
		req.Header.Set("X-Alt-Backend-Token", adminToken)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		require.Equal(t, http.StatusOK, rec.Code)
		assert.JSONEq(t, `{"jobs":[{"id":"j1"}]}`, rec.Body.String())
	})

	t.Run("GET /v1/dashboard/recap_jobs", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v1/dashboard/recap_jobs?window=3600&limit=50", nil)
		req.Header.Set("X-Alt-Backend-Token", adminToken)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		require.Equal(t, http.StatusOK, rec.Code)
		var got []domain.RecapJob
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
		require.Len(t, got, 1)
		assert.Equal(t, "recap-job-1", got[0].JobID)
		assert.Equal(t, "succeeded", got[0].Status)
	})
}
