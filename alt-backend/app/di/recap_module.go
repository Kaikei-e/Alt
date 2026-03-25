package di

import (
	"alt/driver/recap_job_driver"
	"alt/gateway/dashboard_gateway"
	"alt/gateway/recap_articles_gateway"
	"alt/gateway/recap_gateway"
	dashboard_usecase "alt/usecase/dashboard"
	"alt/usecase/recap_articles_usecase"
	"alt/usecase/recap_usecase"
)

// RecapModule holds all recap-domain components.
type RecapModule struct {
	RecapArticlesUsecase    *recap_articles_usecase.RecapArticlesUsecase
	RecapUsecase            *recap_usecase.RecapUsecase
	GetRecapJobsUsecase     dashboard_usecase.GetRecapJobsUsecase
	DashboardMetricsUsecase *dashboard_usecase.DashboardMetricsUsecase
}

func newRecapModule(infra *InfraModule) *RecapModule {
	cfg := infra.Config

	// Recap articles
	recapArticlesGw := recap_articles_gateway.NewGateway(infra.AltDBRepository)
	recapUsecaseCfg := recap_articles_usecase.Config{
		DefaultPageSize: cfg.Recap.DefaultPageSize,
		MaxPageSize:     cfg.Recap.MaxPageSize,
		MaxRangeDays:    cfg.Recap.MaxRangeDays,
	}
	recapArticlesUC := recap_articles_usecase.NewRecapArticlesUsecase(recapArticlesGw, recapUsecaseCfg)

	// Recap 7-day summary
	recapGw := recap_gateway.NewRecapGateway(infra.SearchIndexerDriver)
	recapUC := recap_usecase.NewRecapUsecase(recapGw)

	// Dashboard recap jobs
	recapJobDriver := recap_job_driver.NewRecapJobGateway(cfg.Recap.WorkerURL)
	getRecapJobsUC := dashboard_usecase.NewGetRecapJobsUsecase(recapJobDriver)

	// Dashboard metrics
	dashboardGw := dashboard_gateway.NewDashboardGateway()
	dashboardMetricsUC := dashboard_usecase.NewDashboardMetricsUsecase(dashboardGw)

	return &RecapModule{
		RecapArticlesUsecase:    recapArticlesUC,
		RecapUsecase:            recapUC,
		GetRecapJobsUsecase:     getRecapJobsUC,
		DashboardMetricsUsecase: dashboardMetricsUC,
	}
}
