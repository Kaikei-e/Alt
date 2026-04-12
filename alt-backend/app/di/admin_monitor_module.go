package di

import (
	"alt/config"
	"alt/driver/prometheus_client"
	"alt/gateway/admin_metrics_gateway"
	"alt/port/admin_metrics_port"
	"alt/usecase/admin_metrics_usecase"
	"log/slog"
)

// AdminMonitorModule wires the Prometheus-backed observability pipeline:
// prometheus_client driver -> admin_metrics_gateway -> Facade (usecases).
// The module is only populated when config.AdminMonitor.Enabled is true; when
// disabled the Facade is nil and the Connect handler is skipped in server.go.
type AdminMonitorModule struct {
	Enabled bool
	Port    admin_metrics_port.AdminMetricsPort
	Facade  *admin_metrics_usecase.Facade
}

func newAdminMonitorModule(cfg *config.Config, logger *slog.Logger) *AdminMonitorModule {
	m := &AdminMonitorModule{Enabled: cfg.AdminMonitor.Enabled}
	if !m.Enabled {
		return m
	}
	client, err := prometheus_client.New(prometheus_client.Config{
		URL:     cfg.AdminMonitor.PrometheusURL,
		Timeout: cfg.AdminMonitor.QueryTimeout,
	})
	if err != nil {
		logger.Warn("admin_monitor disabled: prometheus_client init failed", "err", err)
		m.Enabled = false
		return m
	}
	gw := admin_metrics_gateway.New(admin_metrics_gateway.Config{
		Client:         client,
		CacheTTL:       cfg.AdminMonitor.CacheTTL,
		RateLimit:      float64(cfg.AdminMonitor.RateLimitRPS),
		RateLimitBurst: cfg.AdminMonitor.RateLimitBurst,
	})
	m.Port = gw
	m.Facade = admin_metrics_usecase.NewFacade(gw, cfg.AdminMonitor.StreamInterval)
	return m
}
