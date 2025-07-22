package domain

import (
	"fmt"
	"path/filepath"
)

// ChartType represents the type of chart
type ChartType string

const (
	InfrastructureChart ChartType = "infrastructure"
	ApplicationChart    ChartType = "application"
	OperationalChart    ChartType = "operational"
	// Specialized chart types for dependency resolution
	ConfigChart      ChartType = "config"
	FrontendChart    ChartType = "frontend"
	ServiceChart     ChartType = "service"
	SecurityChart    ChartType = "security"
	IngressChart     ChartType = "ingress"
	MonitoringChart  ChartType = "monitoring"
)

// Chart represents a Helm chart configuration
type Chart struct {
	Name             string
	Type             ChartType
	Path             string
	Version          string
	ValuesPath       string
	Values           map[string]interface{} // Chart values
	WaitReady        bool
	MultiNamespace   bool     // Deploy to multiple namespaces
	TargetNamespaces []string // List of target namespaces for multi-namespace deployment
}

// HasTemplates checks if the chart has templates
func (c *Chart) HasTemplates() bool {
	// TODO: Implement template check logic
	// This is a placeholder - should check if chart has template files
	return true
}


// ChartConfig holds the chart deployment configuration
type ChartConfig struct {
	InfrastructureCharts []Chart
	ApplicationCharts    []Chart
	OperationalCharts    []Chart
}

// NewChartConfig creates a new chart configuration with default charts
func NewChartConfig(chartsDir string) *ChartConfig {
	return &ChartConfig{
		InfrastructureCharts: []Chart{
			// Database Layer (StatefulSets)
			{Name: "postgres", Type: InfrastructureChart, Path: filepath.Join(chartsDir, "postgres"), WaitReady: false},
			{Name: "auth-postgres", Type: InfrastructureChart, Path: filepath.Join(chartsDir, "auth-postgres"), WaitReady: false},
			{Name: "kratos-postgres", Type: InfrastructureChart, Path: filepath.Join(chartsDir, "kratos-postgres"), WaitReady: false},
			{Name: "clickhouse", Type: InfrastructureChart, Path: filepath.Join(chartsDir, "clickhouse"), WaitReady: false},
			{Name: "meilisearch", Type: InfrastructureChart, Path: filepath.Join(chartsDir, "meilisearch"), WaitReady: false},
			// Config/Secret Layer
			{Name: "common-secrets", Type: InfrastructureChart, Path: filepath.Join(chartsDir, "common-secrets"), WaitReady: false, MultiNamespace: true, TargetNamespaces: []string{"alt-apps", "alt-auth"}},
			{Name: "common-config", Type: InfrastructureChart, Path: filepath.Join(chartsDir, "common-config"), WaitReady: false},
			{Name: "common-ssl", Type: InfrastructureChart, Path: filepath.Join(chartsDir, "common-ssl"), WaitReady: false, MultiNamespace: true, TargetNamespaces: []string{"alt-apps", "alt-database", "alt-ingress", "alt-search", "alt-auth"}},
			// Network Layer
			{Name: "nginx", Type: InfrastructureChart, Path: filepath.Join(chartsDir, "nginx"), WaitReady: false},
			{Name: "nginx-external", Type: InfrastructureChart, Path: filepath.Join(chartsDir, "nginx-external"), WaitReady: false},
		},
		ApplicationCharts: []Chart{
			// Core Application Layer
			{Name: "alt-backend", Type: ApplicationChart, Path: filepath.Join(chartsDir, "alt-backend"), WaitReady: true},
			{Name: "auth-service", Type: ApplicationChart, Path: filepath.Join(chartsDir, "auth-service"), WaitReady: true},
			{Name: "kratos", Type: ApplicationChart, Path: filepath.Join(chartsDir, "kratos"), WaitReady: true},
			// Frontend Layer
			{Name: "alt-frontend", Type: ApplicationChart, Path: filepath.Join(chartsDir, "alt-frontend"), WaitReady: true},
			// Processor Layer
			{Name: "pre-processor", Type: ApplicationChart, Path: filepath.Join(chartsDir, "pre-processor"), WaitReady: true},
			{Name: "search-indexer", Type: ApplicationChart, Path: filepath.Join(chartsDir, "search-indexer"), WaitReady: true},
			{Name: "tag-generator", Type: ApplicationChart, Path: filepath.Join(chartsDir, "tag-generator"), WaitReady: true},
			{Name: "news-creator", Type: ApplicationChart, Path: filepath.Join(chartsDir, "news-creator"), WaitReady: true},
			{Name: "rask-log-aggregator", Type: ApplicationChart, Path: filepath.Join(chartsDir, "rask-log-aggregator"), WaitReady: true},
		},
		OperationalCharts: []Chart{
			// Operational Layer
			{Name: "migrate", Type: OperationalChart, Path: filepath.Join(chartsDir, "migrate"), WaitReady: true},
			{Name: "backup", Type: OperationalChart, Path: filepath.Join(chartsDir, "backup"), WaitReady: true},
			{Name: "monitoring", Type: OperationalChart, Path: filepath.Join(chartsDir, "monitoring"), WaitReady: false},
		},
	}
}

// AllCharts returns all charts in deployment order
func (c *ChartConfig) AllCharts() []Chart {
	var charts []Chart
	charts = append(charts, c.InfrastructureCharts...)
	charts = append(charts, c.ApplicationCharts...)
	charts = append(charts, c.OperationalCharts...)
	return charts
}

// GetChart returns a chart by name
func (c *ChartConfig) GetChart(name string) (*Chart, error) {
	for _, chart := range c.AllCharts() {
		if chart.Name == name {
			return &chart, nil
		}
	}
	return nil, fmt.Errorf("chart not found: %s", name)
}

// ValuesFile returns the values file path for the chart in the given environment
func (c *Chart) ValuesFile(env Environment) string {
	envValues := filepath.Join(c.Path, fmt.Sprintf("values-%s.yaml", env))
	return envValues
}

// DefaultValuesFile returns the default values file path
func (c *Chart) DefaultValuesFile() string {
	return filepath.Join(c.Path, "values.yaml")
}

// ShouldWaitForReadiness returns true if the chart should wait for readiness
func (c *Chart) ShouldWaitForReadiness() bool {
	return c.WaitReady
}

// ShouldWaitForReadinessWithOptions returns true if the chart should wait for readiness based on deployment options
func (c *Chart) ShouldWaitForReadinessWithOptions(options *DeploymentOptions) bool {
	// Don't wait for readiness during force updates to prevent hanging
	if options.ForceUpdate {
		return false
	}

	// Don't wait for readiness during dry run
	if options.DryRun {
		return false
	}

	return c.WaitReady
}

// SupportsImageOverride returns true if the chart supports image override
func (c *Chart) SupportsImageOverride() bool {
	applicationCharts := map[string]bool{
		"alt-backend":         true,
		"auth-service":        true,
		"pre-processor":       true,
		"search-indexer":      true,
		"tag-generator":       true,
		"news-creator":        true,
		"rask-log-aggregator": true,
		"alt-frontend":        true,
	}
	return applicationCharts[c.Name]
}
