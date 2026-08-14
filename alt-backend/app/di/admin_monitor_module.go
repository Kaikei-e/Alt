package di

import (
	"alt/config"
	"alt/orchestrator/driver/prometheus_client"
	"alt/orchestrator/gateway/admin_metrics_gateway"
	"alt/orchestrator/port/admin_metrics_port"
	"alt/orchestrator/usecase/admin_metrics_usecase"
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

// newAdminMonitorModule emits the loud enabled/disabled/misconfigured signal
// CLAUDE.md rule 8 / .claude/rules/di-wiring.md require, and never rewrites the
// operator's flag.
//
// ADMIN_MONITOR_ENABLED=true with a driver that cannot be built used to log a
// Warn and set Enabled=false, which made connect/v2/server.go announce
// "AdminMonitorService disabled (config.AdminMonitor.Enabled=false)" about a
// config value that said the opposite. An operator reading that line has no way
// to tell a bad ADMIN_MONITOR_PROMETHEUS_URL from a flag they forgot to set,
// and the admin UI's monitoring RPCs answer 404 either way. Enabled but
// unbuildable is missing required config, so it exits non-zero (rule 9).
func newAdminMonitorModule(cfg *config.Config, logger *slog.Logger) *AdminMonitorModule {
	m := &AdminMonitorModule{Enabled: cfg.AdminMonitor.Enabled}
	if !m.Enabled {
		logger.Warn("admin_monitor_disabled", "reason", "ADMIN_MONITOR_ENABLED=false")
		return m
	}
	client, err := prometheus_client.New(prometheus_client.Config{
		URL:     cfg.AdminMonitor.PrometheusURL,
		Timeout: cfg.AdminMonitor.QueryTimeout,
	})
	if err != nil {
		logger.Error("admin_monitor_misconfigured",
			"reason", "ADMIN_MONITOR_ENABLED=true but the prometheus client could not be built",
			"prometheus_url", cfg.AdminMonitor.PrometheusURL,
			"err", err)
		panic("ADMIN_MONITOR_ENABLED=true requires a usable ADMIN_MONITOR_PROMETHEUS_URL — " +
			"refusing to start with the admin monitoring RPCs silently 404: " + err.Error())
	}
	gw := admin_metrics_gateway.New(admin_metrics_gateway.Config{
		Client:         client,
		CacheTTL:       cfg.AdminMonitor.CacheTTL,
		RateLimit:      float64(cfg.AdminMonitor.RateLimitRPS),
		RateLimitBurst: cfg.AdminMonitor.RateLimitBurst,
	})
	m.Port = gw
	m.Facade = admin_metrics_usecase.NewFacade(gw, cfg.AdminMonitor.StreamInterval)
	logger.Info("admin_monitor_enabled", "prometheus_url", cfg.AdminMonitor.PrometheusURL)
	return m
}
