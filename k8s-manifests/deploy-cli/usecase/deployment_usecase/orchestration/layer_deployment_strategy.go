// PHASE R1: Layer deployment strategy implementation
package orchestration

import (
	"fmt"
	"time"

	"deploy-cli/domain"
	"deploy-cli/port/logger_port"
)

// LayerManager manages layer configurations and strategies
type LayerManager struct {
	logger logger_port.LoggerPort
}

// LayerManagerPort defines the interface for layer management
type LayerManagerPort interface {
	GetLayerConfigurations(chartConfig *domain.ChartConfig, chartsDir string) []domain.LayerConfiguration
	GetDefaultLayerConfigurations(chartConfig *domain.ChartConfig, chartsDir string) []domain.LayerConfiguration
	CreateCustomLayerConfiguration(layers []domain.LayerDefinition) []domain.LayerConfiguration
	ValidateLayerConfiguration(config []domain.LayerConfiguration) error
}

// NewLayerManager creates a new layer manager
func NewLayerManager(logger logger_port.LoggerPort) *LayerManager {
	return &LayerManager{
		logger: logger,
	}
}

// GetLayerConfigurations gets layer configurations based on chart config
func (l *LayerManager) GetLayerConfigurations(chartConfig *domain.ChartConfig, chartsDir string) []domain.LayerConfiguration {
	l.logger.DebugWithContext("determining layer configurations", map[string]interface{}{
		"charts_dir": chartsDir,
	})

	// For now, return default configurations
	// Future enhancement: support custom layer definitions from config files
	return l.GetDefaultLayerConfigurations(chartConfig, chartsDir)
}

// GetDefaultLayerConfigurations returns the default layer configurations (PHASE 2 enhanced)
func (l *LayerManager) GetDefaultLayerConfigurations(chartConfig *domain.ChartConfig, chartsDir string) []domain.LayerConfiguration {
	l.logger.DebugWithContext("creating default layer configurations", map[string]interface{}{
		"charts_dir": chartsDir,
	})

	return []domain.LayerConfiguration{
		{
			Name: "Storage & Persistent Infrastructure",
			Charts: []domain.Chart{
				{Name: "postgres", Type: domain.InfrastructureChart, Path: chartsDir + "/postgres", WaitReady: true},
				{Name: "auth-postgres", Type: domain.InfrastructureChart, Path: chartsDir + "/auth-postgres", WaitReady: true},
				{Name: "kratos-postgres", Type: domain.InfrastructureChart, Path: chartsDir + "/kratos-postgres", WaitReady: true},
				{Name: "clickhouse", Type: domain.InfrastructureChart, Path: chartsDir + "/clickhouse", WaitReady: true},
			},
			RequiresHealthCheck:     true,
			HealthCheckTimeout:      20 * time.Minute, // Phase 2: Extended from 15 minutes
			WaitBetweenCharts:       60 * time.Second, // Phase 2: Extended from 30 seconds
			LayerCompletionTimeout:  30 * time.Minute, // Phase 2: Extended from 20 minutes
			AllowParallelDeployment: false, // StatefulSets should be sequential
			CriticalLayer:           true,
		},
		{
			Name: "Core Services & Dependencies",
			Charts: []domain.Chart{
				{Name: "meilisearch", Type: domain.ServiceChart, Path: chartsDir + "/meilisearch", WaitReady: true},
				{Name: "common-secrets", Type: domain.ConfigChart, Path: chartsDir + "/common-secrets"},
				{Name: "common-config", Type: domain.ConfigChart, Path: chartsDir + "/common-config"},
			},
			RequiresHealthCheck:     true,
			HealthCheckTimeout:      10 * time.Minute,
			WaitBetweenCharts:       30 * time.Second,
			LayerCompletionTimeout:  15 * time.Minute,
			AllowParallelDeployment: true, // Config resources can be parallel
			CriticalLayer:           true,
		},
		{
			Name: "Application Services",
			Charts: []domain.Chart{
				{Name: "alt-backend", Type: domain.ApplicationChart, Path: chartsDir + "/alt-backend", WaitReady: true},
				{Name: "pre-processor", Type: domain.ApplicationChart, Path: chartsDir + "/pre-processor", WaitReady: true},
				{Name: "search-indexer", Type: domain.ApplicationChart, Path: chartsDir + "/search-indexer", WaitReady: true},
				{Name: "tag-generator", Type: domain.ApplicationChart, Path: chartsDir + "/tag-generator", WaitReady: true},
				{Name: "news-creator", Type: domain.ApplicationChart, Path: chartsDir + "/news-creator", WaitReady: true},
			},
			RequiresHealthCheck:     true,
			HealthCheckTimeout:      8 * time.Minute,
			WaitBetweenCharts:       20 * time.Second,
			LayerCompletionTimeout:  15 * time.Minute,
			AllowParallelDeployment: true, // Application services can be parallel
			CriticalLayer:           false, // Non-critical for basic functionality
		},
		{
			Name: "Frontend & External Interfaces",
			Charts: []domain.Chart{
				{Name: "alt-frontend", Type: domain.FrontendChart, Path: chartsDir + "/alt-frontend", WaitReady: true},
				{Name: "nginx-external", Type: domain.IngressChart, Path: chartsDir + "/nginx-external", WaitReady: true},
			},
			RequiresHealthCheck:     true,
			HealthCheckTimeout:      5 * time.Minute,
			WaitBetweenCharts:       15 * time.Second,
			LayerCompletionTimeout:  10 * time.Minute,
			AllowParallelDeployment: true,
			CriticalLayer:           false,
		},
		{
			Name: "Authentication & Security",
			Charts: []domain.Chart{
				{Name: "auth-service", Type: domain.SecurityChart, Path: chartsDir + "/auth-service", WaitReady: true},
				{Name: "kratos", Type: domain.SecurityChart, Path: chartsDir + "/kratos", WaitReady: true},
			},
			RequiresHealthCheck:     true,
			HealthCheckTimeout:      8 * time.Minute,
			WaitBetweenCharts:       30 * time.Second,
			LayerCompletionTimeout:  12 * time.Minute,
			AllowParallelDeployment: false, // Security services should be sequential
			CriticalLayer:           true, // Security is critical
		},
		{
			Name: "Monitoring & Observability",
			Charts: []domain.Chart{
				{Name: "rask-log-forwarder", Type: domain.MonitoringChart, Path: chartsDir + "/rask-log-forwarder"},
				{Name: "rask-log-aggregator", Type: domain.MonitoringChart, Path: chartsDir + "/rask-log-aggregator"},
			},
			RequiresHealthCheck:     false, // Monitoring can be deployed without strict health checks
			HealthCheckTimeout:      3 * time.Minute,
			WaitBetweenCharts:       10 * time.Second,
			LayerCompletionTimeout:  8 * time.Minute,
			AllowParallelDeployment: true,
			CriticalLayer:           false, // Non-critical for core functionality
		},
	}
}

// CreateCustomLayerConfiguration creates layer configuration from custom definitions
func (l *LayerManager) CreateCustomLayerConfiguration(layers []domain.LayerDefinition) []domain.LayerConfiguration {
	l.logger.InfoWithContext("creating custom layer configuration", map[string]interface{}{
		"layer_count": len(layers),
	})

	configs := make([]domain.LayerConfiguration, len(layers))
	
	for i, layerDef := range layers {
		config := domain.LayerConfiguration{
			Name:                    layerDef.Name,
			Charts:                  layerDef.Charts,
			RequiresHealthCheck:     layerDef.WaitReady,
			HealthCheckTimeout:      layerDef.Timeout,
			WaitBetweenCharts:       time.Duration(0),
			LayerCompletionTimeout:  layerDef.Timeout,
			AllowParallelDeployment: layerDef.Parallel,
			CriticalLayer:           false,
		}

		// Apply defaults if not specified
		if config.HealthCheckTimeout == 0 {
			config.HealthCheckTimeout = 5 * time.Minute
		}
		if config.LayerCompletionTimeout == 0 {
			config.LayerCompletionTimeout = 10 * time.Minute
		}
		if config.WaitBetweenCharts == 0 {
			config.WaitBetweenCharts = 15 * time.Second
		}

		configs[i] = config

		l.logger.DebugWithContext("custom layer configuration created", map[string]interface{}{
			"layer":            config.Name,
			"chart_count":      len(config.Charts),
			"critical":         config.CriticalLayer,
			"parallel":         config.AllowParallelDeployment,
		})
	}

	return configs
}

// ValidateLayerConfiguration validates a layer configuration
func (l *LayerManager) ValidateLayerConfiguration(config []domain.LayerConfiguration) error {
	l.logger.DebugWithContext("validating layer configuration", map[string]interface{}{
		"layer_count": len(config),
	})

	if len(config) == 0 {
		return fmt.Errorf("layer configuration cannot be empty")
	}

	// Track chart names to ensure no duplicates across layers
	chartNames := make(map[string]string)

	for i, layer := range config {
		// Validate layer name
		if layer.Name == "" {
			return fmt.Errorf("layer %d has empty name", i)
		}

		// Validate charts
		if len(layer.Charts) == 0 {
			return fmt.Errorf("layer %s has no charts", layer.Name)
		}

		// Check for duplicate charts
		for _, chart := range layer.Charts {
			if existingLayer, exists := chartNames[chart.Name]; exists {
				return fmt.Errorf("chart %s appears in multiple layers: %s and %s", chart.Name, existingLayer, layer.Name)
			}
			chartNames[chart.Name] = layer.Name
		}

		// Validate timeouts
		if layer.HealthCheckTimeout < 0 {
			return fmt.Errorf("layer %s has negative health check timeout", layer.Name)
		}
		if layer.LayerCompletionTimeout < 0 {
			return fmt.Errorf("layer %s has negative completion timeout", layer.Name)
		}


		l.logger.DebugWithContext("layer configuration validated", map[string]interface{}{
			"layer":       layer.Name,
			"chart_count": len(layer.Charts),
			"critical":    layer.CriticalLayer,
		})
	}

	l.logger.InfoWithContext("layer configuration validation completed", map[string]interface{}{
		"layer_count": len(config),
		"total_charts": len(chartNames),
	})

	return nil
}