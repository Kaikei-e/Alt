package di

import (
	"alt/orchestrator/driver/recap_job_driver"
	"alt/orchestrator/gateway/dashboard_gateway"
	"alt/orchestrator/gateway/recap_gateway"
	dashboard_usecase "alt/orchestrator/usecase/dashboard"
	"alt/orchestrator/usecase/recap_usecase"
)

// RecapModule holds cmd/backend's recap-domain components.
//
// RecapArticlesUsecase is not among them: it backs
// BackendInternalService/ListRecapArticles, which cmd/datahub serves. Building
// it here as well would give the backend a second, independently configured
// reader of the same table that no backend handler ever calls.
type RecapModule struct {
	RecapUsecase            *recap_usecase.RecapUsecase
	GetRecapJobsUsecase     dashboard_usecase.GetRecapJobsUsecase
	DashboardMetricsUsecase *dashboard_usecase.DashboardMetricsUsecase
}

func newRecapModule(infra *InfraModule) *RecapModule {
	cfg := infra.Config

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
		RecapUsecase:            recapUC,
		GetRecapJobsUsecase:     getRecapJobsUC,
		DashboardMetricsUsecase: dashboardMetricsUC,
	}
}
