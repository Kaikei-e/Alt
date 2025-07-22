package domain

import (
	"time"
)

// DeploymentStrategy defines the interface for environment-specific deployment strategies
type DeploymentStrategy interface {
	// GetName returns the strategy name
	GetName() string

	// GetEnvironment returns the target environment
	GetEnvironment() Environment

	// GetLayerConfigurations returns environment-specific layer configurations
	GetLayerConfigurations(chartsDir string) []LayerConfiguration

	// GetGlobalTimeout returns the overall deployment timeout
	GetGlobalTimeout() time.Duration

	// AllowsParallelDeployment returns whether parallel deployment is allowed
	AllowsParallelDeployment() bool

	// GetHealthCheckRetries returns the number of health check retries
	GetHealthCheckRetries() int

	// RequiresZeroDowntime returns whether zero-downtime deployment is required
	RequiresZeroDowntime() bool
}

// LayerConfiguration represents environment-specific layer deployment settings
type LayerConfiguration struct {
	Name                    string
	Charts                  []Chart
	RequiresHealthCheck     bool
	HealthCheckTimeout      time.Duration
	WaitBetweenCharts       time.Duration
	LayerCompletionTimeout  time.Duration
	AllowParallelDeployment bool
	SkipInEnvironment       []Environment
	CriticalLayer           bool
}

// DevelopmentStrategy implements fast deployment for development environment
type DevelopmentStrategy struct{}

func (d *DevelopmentStrategy) GetName() string {
	return "development"
}

func (d *DevelopmentStrategy) GetEnvironment() Environment {
	return Development
}

func (d *DevelopmentStrategy) GetLayerConfigurations(chartsDir string) []LayerConfiguration {
	return []LayerConfiguration{
		{
			Name: "Essential Storage",
			Charts: []Chart{
				{Name: "postgres", Type: InfrastructureChart, Path: chartsDir + "/postgres", WaitReady: true},
				{Name: "clickhouse", Type: InfrastructureChart, Path: chartsDir + "/clickhouse", WaitReady: true},
				{Name: "meilisearch", Type: InfrastructureChart, Path: chartsDir + "/meilisearch", WaitReady: true},
			},
			RequiresHealthCheck:     true,
			HealthCheckTimeout:      5 * time.Minute, // Reduced for development
			WaitBetweenCharts:       10 * time.Second,
			LayerCompletionTimeout:  8 * time.Minute,
			AllowParallelDeployment: true,
			CriticalLayer:           true,
		},
		{
			Name: "Core Services",
			Charts: []Chart{
				{Name: "alt-backend", Type: ApplicationChart, Path: chartsDir + "/alt-backend", WaitReady: true},
				{Name: "auth-service", Type: ApplicationChart, Path: chartsDir + "/auth-service", WaitReady: true},
			},
			RequiresHealthCheck:     true,
			HealthCheckTimeout:      3 * time.Minute,
			WaitBetweenCharts:       5 * time.Second,
			LayerCompletionTimeout:  5 * time.Minute,
			AllowParallelDeployment: true,
			CriticalLayer:           true,
		},
		{
			Name: "Frontend & Optional Services",
			Charts: []Chart{
				{Name: "alt-frontend", Type: ApplicationChart, Path: chartsDir + "/alt-frontend", WaitReady: true},
				{Name: "nginx", Type: InfrastructureChart, Path: chartsDir + "/nginx", WaitReady: false},
			},
			RequiresHealthCheck:     false,
			HealthCheckTimeout:      2 * time.Minute,
			WaitBetweenCharts:       5 * time.Second,
			LayerCompletionTimeout:  4 * time.Minute,
			AllowParallelDeployment: true,
			CriticalLayer:           false,
		},
	}
}

func (d *DevelopmentStrategy) GetGlobalTimeout() time.Duration {
	return 20 * time.Minute // Reduced overall timeout
}

func (d *DevelopmentStrategy) AllowsParallelDeployment() bool {
	return true
}

func (d *DevelopmentStrategy) GetHealthCheckRetries() int {
	return 3 // Fewer retries for faster feedback
}

func (d *DevelopmentStrategy) RequiresZeroDowntime() bool {
	return false
}

// StagingStrategy implements comprehensive validation for staging environment
type StagingStrategy struct{}

func (s *StagingStrategy) GetName() string {
	return "staging"
}

func (s *StagingStrategy) GetEnvironment() Environment {
	return Staging
}

func (s *StagingStrategy) GetLayerConfigurations(chartsDir string) []LayerConfiguration {
	return []LayerConfiguration{
		{
			Name: "Storage & Persistent Infrastructure",
			Charts: []Chart{
				{Name: "postgres", Type: InfrastructureChart, Path: chartsDir + "/postgres", WaitReady: true},
				{Name: "auth-postgres", Type: InfrastructureChart, Path: chartsDir + "/auth-postgres", WaitReady: true},
				{Name: "kratos-postgres", Type: InfrastructureChart, Path: chartsDir + "/kratos-postgres", WaitReady: true},
				{Name: "clickhouse", Type: InfrastructureChart, Path: chartsDir + "/clickhouse", WaitReady: true},
				{Name: "meilisearch", Type: InfrastructureChart, Path: chartsDir + "/meilisearch", WaitReady: true},
			},
			RequiresHealthCheck:     true,
			HealthCheckTimeout:      12 * time.Minute,
			WaitBetweenCharts:       25 * time.Second,
			LayerCompletionTimeout:  18 * time.Minute,
			AllowParallelDeployment: false,
			CriticalLayer:           true,
		},
		{
			Name: "Configuration & Secrets",
			Charts: []Chart{
				{Name: "common-secrets", Type: InfrastructureChart, Path: chartsDir + "/common-secrets", WaitReady: false, MultiNamespace: true, TargetNamespaces: []string{"alt-apps"}},
				{Name: "common-config", Type: InfrastructureChart, Path: chartsDir + "/common-config", WaitReady: false},
				{Name: "common-ssl", Type: InfrastructureChart, Path: chartsDir + "/common-ssl", WaitReady: false, MultiNamespace: true, TargetNamespaces: []string{"alt-apps", "alt-database", "alt-ingress", "alt-search", "alt-auth"}},
			},
			RequiresHealthCheck:     true,
			HealthCheckTimeout:      3 * time.Minute,
			WaitBetweenCharts:       10 * time.Second,
			LayerCompletionTimeout:  6 * time.Minute,
			AllowParallelDeployment: false,
			CriticalLayer:           true,
		},
		{
			Name: "Core Services",
			Charts: []Chart{
				{Name: "alt-backend", Type: ApplicationChart, Path: chartsDir + "/alt-backend", WaitReady: true},
				{Name: "auth-service", Type: ApplicationChart, Path: chartsDir + "/auth-service", WaitReady: true},
				{Name: "kratos", Type: ApplicationChart, Path: chartsDir + "/kratos", WaitReady: true},
			},
			RequiresHealthCheck:     true,
			HealthCheckTimeout:      8 * time.Minute,
			WaitBetweenCharts:       15 * time.Second,
			LayerCompletionTimeout:  12 * time.Minute,
			AllowParallelDeployment: false,
			CriticalLayer:           true,
		},
		{
			Name: "Network & Ingress",
			Charts: []Chart{
				{Name: "nginx", Type: InfrastructureChart, Path: chartsDir + "/nginx", WaitReady: true},
				{Name: "nginx-external", Type: InfrastructureChart, Path: chartsDir + "/nginx-external", WaitReady: true},
			},
			RequiresHealthCheck:     true,
			HealthCheckTimeout:      5 * time.Minute,
			WaitBetweenCharts:       10 * time.Second,
			LayerCompletionTimeout:  8 * time.Minute,
			AllowParallelDeployment: false,
			CriticalLayer:           false,
		},
		{
			Name: "Frontend Applications",
			Charts: []Chart{
				{Name: "alt-frontend", Type: ApplicationChart, Path: chartsDir + "/alt-frontend", WaitReady: true},
			},
			RequiresHealthCheck:     true,
			HealthCheckTimeout:      8 * time.Minute,
			WaitBetweenCharts:       10 * time.Second,
			LayerCompletionTimeout:  10 * time.Minute,
			AllowParallelDeployment: false,
			CriticalLayer:           false,
		},
		{
			Name: "Data Processing Services",
			Charts: []Chart{
				{Name: "pre-processor", Type: ApplicationChart, Path: chartsDir + "/pre-processor", WaitReady: true},
				{Name: "search-indexer", Type: ApplicationChart, Path: chartsDir + "/search-indexer", WaitReady: true},
				{Name: "tag-generator", Type: ApplicationChart, Path: chartsDir + "/tag-generator", WaitReady: true},
				{Name: "news-creator", Type: ApplicationChart, Path: chartsDir + "/news-creator", WaitReady: true},
			},
			RequiresHealthCheck:     true,
			HealthCheckTimeout:      10 * time.Minute,
			WaitBetweenCharts:       20 * time.Second,
			LayerCompletionTimeout:  15 * time.Minute,
			AllowParallelDeployment: false,
			CriticalLayer:           false,
		},
		{
			Name: "Operations & Monitoring",
			Charts: []Chart{
				{Name: "migrate", Type: OperationalChart, Path: chartsDir + "/migrate", WaitReady: true},
				{Name: "backup", Type: OperationalChart, Path: chartsDir + "/backup", WaitReady: true},
				{Name: "monitoring", Type: OperationalChart, Path: chartsDir + "/monitoring", WaitReady: true},
			},
			RequiresHealthCheck:     true,
			HealthCheckTimeout:      5 * time.Minute,
			WaitBetweenCharts:       10 * time.Second,
			LayerCompletionTimeout:  10 * time.Minute,
			AllowParallelDeployment: false,
			CriticalLayer:           false,
		},
	}
}

func (s *StagingStrategy) GetGlobalTimeout() time.Duration {
	return 60 * time.Minute // Extended timeout for comprehensive validation
}

func (s *StagingStrategy) AllowsParallelDeployment() bool {
	return false
}

func (s *StagingStrategy) GetHealthCheckRetries() int {
	return 5 // More retries for comprehensive validation
}

func (s *StagingStrategy) RequiresZeroDowntime() bool {
	return false
}

// ProductionStrategy implements conservative, reliable deployment for production
type ProductionStrategy struct{}

func (p *ProductionStrategy) GetName() string {
	return "production"
}

func (p *ProductionStrategy) GetEnvironment() Environment {
	return Production
}

func (p *ProductionStrategy) GetLayerConfigurations(chartsDir string) []LayerConfiguration {
	return []LayerConfiguration{
		{
			Name: "Core Database Layer",
			Charts: []Chart{
				{Name: "postgres", Type: InfrastructureChart, Path: chartsDir + "/postgres", WaitReady: true},
			},
			RequiresHealthCheck:     true,
			HealthCheckTimeout:      5 * time.Minute,
			WaitBetweenCharts:       20 * time.Second,
			LayerCompletionTimeout:  8 * time.Minute, // Single chart, shorter timeout
			AllowParallelDeployment: false,
			CriticalLayer:           true,
		},
		{
			Name: "Authentication Database Layer",
			Charts: []Chart{
				{Name: "auth-postgres", Type: InfrastructureChart, Path: chartsDir + "/auth-postgres", WaitReady: true},
				{Name: "kratos-postgres", Type: InfrastructureChart, Path: chartsDir + "/kratos-postgres", WaitReady: true},
			},
			RequiresHealthCheck:     true,
			HealthCheckTimeout:      4 * time.Minute,
			WaitBetweenCharts:       20 * time.Second,
			LayerCompletionTimeout:  10 * time.Minute, // Two charts, parallel possible
			AllowParallelDeployment: true, // IMPROVEMENT: Enable parallel for independent auth DBs
			CriticalLayer:           true,
		},
		{
			Name: "Analytics & Search Database Layer",
			Charts: []Chart{
				{Name: "clickhouse", Type: InfrastructureChart, Path: chartsDir + "/clickhouse", WaitReady: true},
				{Name: "meilisearch", Type: InfrastructureChart, Path: chartsDir + "/meilisearch", WaitReady: true},
			},
			RequiresHealthCheck:     true,
			HealthCheckTimeout:      4 * time.Minute,
			WaitBetweenCharts:       20 * time.Second,
			LayerCompletionTimeout:  10 * time.Minute, // Two charts, parallel possible
			AllowParallelDeployment: true, // IMPROVEMENT: Enable parallel for independent storage
			CriticalLayer:           true,
		},
		{
			Name: "Configuration & Secrets",
			Charts: []Chart{
				{Name: "common-secrets", Type: InfrastructureChart, Path: chartsDir + "/common-secrets", WaitReady: false, MultiNamespace: true, TargetNamespaces: []string{"alt-apps"}},
				{Name: "common-config", Type: InfrastructureChart, Path: chartsDir + "/common-config", WaitReady: false},
				{Name: "common-ssl", Type: InfrastructureChart, Path: chartsDir + "/common-ssl", WaitReady: false, MultiNamespace: true, TargetNamespaces: []string{"alt-apps", "alt-database", "alt-ingress", "alt-search", "alt-auth"}},
			},
			RequiresHealthCheck:     true,
			HealthCheckTimeout:      3 * time.Minute,
			WaitBetweenCharts:       10 * time.Second,
			LayerCompletionTimeout:  8 * time.Minute,
			AllowParallelDeployment: false,
			CriticalLayer:           true,
		},
		{
			Name: "Core Services",
			Charts: []Chart{
				{Name: "alt-backend", Type: ApplicationChart, Path: chartsDir + "/alt-backend", WaitReady: true},
				{Name: "auth-service", Type: ApplicationChart, Path: chartsDir + "/auth-service", WaitReady: true},
				{Name: "kratos", Type: ApplicationChart, Path: chartsDir + "/kratos", WaitReady: true},
			},
			RequiresHealthCheck:     true,
			HealthCheckTimeout:      6 * time.Minute, // Reduced for faster failure detection
			WaitBetweenCharts:       20 * time.Second, // Reduced for efficiency
			LayerCompletionTimeout:  12 * time.Minute, // Reduced from 15m to 12m
			AllowParallelDeployment: true, // IMPROVEMENT: Enable parallel for independent services
			CriticalLayer:           true,
		},
		{
			Name: "Network & Ingress",
			Charts: []Chart{
				{Name: "nginx", Type: InfrastructureChart, Path: chartsDir + "/nginx", WaitReady: true},
				{Name: "nginx-external", Type: InfrastructureChart, Path: chartsDir + "/nginx-external", WaitReady: true},
			},
			RequiresHealthCheck:     true,
			HealthCheckTimeout:      5 * time.Minute, // Reduced for faster detection
			WaitBetweenCharts:       15 * time.Second,
			LayerCompletionTimeout:  8 * time.Minute, // Reduced from 12m to 8m
			AllowParallelDeployment: true, // IMPROVEMENT: nginx services can be parallel
			CriticalLayer:           false,
		},
		{
			Name: "Frontend Applications",
			Charts: []Chart{
				{Name: "alt-frontend", Type: ApplicationChart, Path: chartsDir + "/alt-frontend", WaitReady: true},
			},
			RequiresHealthCheck:     true,
			HealthCheckTimeout:      10 * time.Minute,
			WaitBetweenCharts:       15 * time.Second,
			LayerCompletionTimeout:  15 * time.Minute,
			AllowParallelDeployment: false,
			CriticalLayer:           false,
		},
		{
			Name: "Data Processing Services",
			Charts: []Chart{
				{Name: "pre-processor", Type: ApplicationChart, Path: chartsDir + "/pre-processor", WaitReady: true},
				{Name: "search-indexer", Type: ApplicationChart, Path: chartsDir + "/search-indexer", WaitReady: true},
				{Name: "tag-generator", Type: ApplicationChart, Path: chartsDir + "/tag-generator", WaitReady: true},
				{Name: "news-creator", Type: ApplicationChart, Path: chartsDir + "/news-creator", WaitReady: true},
				{Name: "rask-log-aggregator", Type: ApplicationChart, Path: chartsDir + "/rask-log-aggregator", WaitReady: true},
			},
			RequiresHealthCheck:     true,
			HealthCheckTimeout:      6 * time.Minute, // Reduced for efficiency
			WaitBetweenCharts:       20 * time.Second, // Reduced from 30s
			LayerCompletionTimeout:  12 * time.Minute, // Reduced from 15m to 12m
			AllowParallelDeployment: true, // IMPROVEMENT: Independent processing services
			CriticalLayer:           false,
		},
		{
			Name: "Operations & Monitoring",
			Charts: []Chart{
				{Name: "migrate", Type: OperationalChart, Path: chartsDir + "/migrate", WaitReady: true},
				{Name: "backup", Type: OperationalChart, Path: chartsDir + "/backup", WaitReady: true},
				{Name: "monitoring", Type: OperationalChart, Path: chartsDir + "/monitoring", WaitReady: true},
			},
			RequiresHealthCheck:     true,
			HealthCheckTimeout:      8 * time.Minute,
			WaitBetweenCharts:       15 * time.Second,
			LayerCompletionTimeout:  15 * time.Minute,
			AllowParallelDeployment: false,
			CriticalLayer:           false,
		},
	}
}

func (p *ProductionStrategy) GetGlobalTimeout() time.Duration {
	return 60 * time.Minute // EMERGENCY FIX: Reduced from 90m to 60m
}

func (p *ProductionStrategy) AllowsParallelDeployment() bool {
	return false
}

func (p *ProductionStrategy) GetHealthCheckRetries() int {
	return 8 // Maximum retries for production reliability
}

func (p *ProductionStrategy) RequiresZeroDowntime() bool {
	return true
}

// DisasterRecoveryStrategy implements emergency deployment for disaster recovery
type DisasterRecoveryStrategy struct{}

func (d *DisasterRecoveryStrategy) GetName() string {
	return "disaster-recovery"
}

func (d *DisasterRecoveryStrategy) GetEnvironment() Environment {
	return Production // Usually used in production context
}

func (d *DisasterRecoveryStrategy) GetLayerConfigurations(chartsDir string) []LayerConfiguration {
	return []LayerConfiguration{
		{
			Name: "Critical Storage",
			Charts: []Chart{
				{Name: "postgres", Type: InfrastructureChart, Path: chartsDir + "/postgres", WaitReady: true},
				{Name: "auth-postgres", Type: InfrastructureChart, Path: chartsDir + "/auth-postgres", WaitReady: true},
			},
			RequiresHealthCheck:     true,
			HealthCheckTimeout:      8 * time.Minute,
			WaitBetweenCharts:       20 * time.Second,
			LayerCompletionTimeout:  10 * time.Minute,
			AllowParallelDeployment: true, // Speed up for emergency
			CriticalLayer:           true,
		},
		{
			Name: "Essential Services",
			Charts: []Chart{
				{Name: "alt-backend", Type: ApplicationChart, Path: chartsDir + "/alt-backend", WaitReady: true},
				{Name: "auth-service", Type: ApplicationChart, Path: chartsDir + "/auth-service", WaitReady: true},
			},
			RequiresHealthCheck:     true,
			HealthCheckTimeout:      5 * time.Minute,
			WaitBetweenCharts:       10 * time.Second,
			LayerCompletionTimeout:  8 * time.Minute,
			AllowParallelDeployment: true,
			CriticalLayer:           true,
		},
		{
			Name: "Basic Frontend",
			Charts: []Chart{
				{Name: "alt-frontend", Type: ApplicationChart, Path: chartsDir + "/alt-frontend", WaitReady: true},
				{Name: "nginx", Type: InfrastructureChart, Path: chartsDir + "/nginx", WaitReady: false},
			},
			RequiresHealthCheck:     false,
			HealthCheckTimeout:      3 * time.Minute,
			WaitBetweenCharts:       5 * time.Second,
			LayerCompletionTimeout:  5 * time.Minute,
			AllowParallelDeployment: true,
			CriticalLayer:           false,
		},
	}
}

func (d *DisasterRecoveryStrategy) GetGlobalTimeout() time.Duration {
	return 25 * time.Minute // Fast recovery timeout
}

func (d *DisasterRecoveryStrategy) AllowsParallelDeployment() bool {
	return true
}

func (d *DisasterRecoveryStrategy) GetHealthCheckRetries() int {
	return 3 // Minimal retries for speed
}

func (d *DisasterRecoveryStrategy) RequiresZeroDowntime() bool {
	return false
}
