package domain

import (
	"time"
)

// ChartMetadata represents metadata about a Helm chart
type ChartMetadata struct {
	// Basic chart information
	Name         string            `json:"name"`
	Path         string            `json:"path"`
	Directory    string            `json:"directory"`      // Chart directory path
	Version      string            `json:"version"`
	AppVersion   string            `json:"app_version"`
	Description  string            `json:"description"`
	Type         string            `json:"type"`
	Keywords     []string          `json:"keywords"`
	Home         string            `json:"home"`
	Sources      []string          `json:"sources"`
	Icon         string            `json:"icon"`
	
	// Chart maintainers and annotations
	Maintainers  []Maintainer      `json:"maintainers"`
	Annotations  map[string]string `json:"annotations"`
	
	// Dependencies
	Dependencies []ChartDependency `json:"dependencies"`
	
	// API and Kubernetes version constraints
	APIVersion   string            `json:"api_version"`
	KubeVersion  string            `json:"kube_version"`
	
	// Timestamps
	Created      time.Time         `json:"created"`
	Updated      time.Time         `json:"updated"`
	
	// File information
	Size         int64             `json:"size"`
	Digest       string            `json:"digest"`
	URLs         []string          `json:"urls"`
	
	// Validation and quality
	Validated    bool              `json:"validated"`
	Quality      QualityMetrics    `json:"quality"`
	
	// Custom metadata
	CustomFields map[string]interface{} `json:"custom_fields"`
}

// Maintainer represents a chart maintainer
type Maintainer struct {
	Name  string `json:"name"`
	Email string `json:"email"`
	URL   string `json:"url"`
}

// ChartDependency represents a chart dependency
type ChartDependency struct {
	Name         string            `json:"name"`
	Version      string            `json:"version"`
	Repository   string            `json:"repository"`
	Alias        string            `json:"alias"`
	Condition    string            `json:"condition"`
	Tags         []string          `json:"tags"`
	ImportValues []interface{}     `json:"import_values"`
	Enabled      bool              `json:"enabled"`
	Optional     bool              `json:"optional"`
}

// QualityMetrics represents quality metrics for a chart
type QualityMetrics struct {
	Score          int       `json:"score"`           // 0-100 quality score
	LintScore      int       `json:"lint_score"`
	SecurityScore  int       `json:"security_score"`
	DocumentationScore int   `json:"documentation_score"`
	TestCoverage   float64   `json:"test_coverage"`
	LastScanned    time.Time `json:"last_scanned"`
	Issues         []QualityIssue `json:"issues"`
}

// QualityIssue represents a quality issue
type QualityIssue struct {
	Type        string `json:"type"`        // lint, security, documentation
	Severity    string `json:"severity"`    // low, medium, high, critical
	Message     string `json:"message"`
	File        string `json:"file"`
	Line        int    `json:"line"`
	Suggestion  string `json:"suggestion"`
}

// MetadataUpdates represents updates to chart metadata
type MetadataUpdates struct {
	ChartName    string                 `json:"chart_name"`
	Version      string                 `json:"version"`
	Updates      map[string]interface{} `json:"updates"`
	Fields       map[string]interface{} `json:"fields"`     // Alternative field name for updates
	UpdatedBy    string                 `json:"updated_by"`
	UpdatedAt    time.Time              `json:"updated_at"`
	Reason       string                 `json:"reason"`
	Validated    bool                   `json:"validated"`
}

// ChartSummary represents a summary of chart information
type ChartSummary struct {
	Name          string            `json:"name"`
	Path          string            `json:"path"`
	Generated     time.Time         `json:"generated"`
	LatestVersion string            `json:"latest_version"`
	Description   string            `json:"description"`
	Created       time.Time         `json:"created"`
	Updated       time.Time         `json:"updated"`
	Downloads     int64             `json:"downloads"`
	Stars         int               `json:"stars"`
	Deprecated    bool              `json:"deprecated"`
	Verified      bool              `json:"verified"`
	Official      bool              `json:"official"`
	Categories    []string          `json:"categories"`
	Tags          []string          `json:"tags"`
	Metadata      *ChartMetadata    `json:"metadata"`
	Dependencies  []*ChartDependency `json:"dependencies"`
	Values        map[string]interface{} `json:"values"`
	TemplateCount int               `json:"template_count"`
	ResourceTypes []string          `json:"resource_types"`
	ComplexityScore int             `json:"complexity_score"`
	Recommendations []string        `json:"recommendations"`
}

// ReleaseListOptions represents options for listing Helm releases
type ReleaseListOptions struct {
	// Filtering options
	Namespace     string            `json:"namespace"`
	AllNamespaces bool              `json:"all_namespaces"`
	Filter        string            `json:"filter"`        // Regex filter for names
	Selector      string            `json:"selector"`      // Label selector
	
	// Status filtering
	StatusFilter  []string          `json:"status_filter"` // deployed, failed, etc.
	
	// Output options
	MaxReleases   int               `json:"max_releases"`
	Offset        int               `json:"offset"`
	SortBy        string            `json:"sort_by"`       // name, date, status
	SortOrder     string            `json:"sort_order"`    // asc, desc
	
	// Additional data
	IncludeTest   bool              `json:"include_test"`
	Short         bool              `json:"short"`         // Abbreviated output
	Date          bool              `json:"date"`          // Include dates
	TimeFormat    string            `json:"time_format"`   // Time format for output
	
	// Time filtering
	TimeRange     *TimeRange        `json:"time_range"`
}

// TimeRange represents a time range for filtering
type TimeRange struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

// ReleaseInfo represents information about a Helm release
type ReleaseInfo struct {
	// Basic release information
	Name          string            `json:"name"`
	Namespace     string            `json:"namespace"`
	Version       int               `json:"version"`
	Revision      int               `json:"revision"`  // Added for compatibility with helm_release_manager.go
	Status        string            `json:"status"`
	Chart         string            `json:"chart"`
	ChartVersion  string            `json:"chart_version"`
	AppVersion    string            `json:"app_version"`
	
	// Timestamps
	FirstDeployed time.Time         `json:"first_deployed"`
	LastDeployed  time.Time         `json:"last_deployed"`
	Updated       time.Time         `json:"updated"`
	
	// Configuration
	Values        map[string]interface{} `json:"values"`
	ConfigHash    string            `json:"config_hash"`
	
	// Resources and manifest
	Manifest      string            `json:"manifest"`
	Resources     []ResourceInfo    `json:"resources"`
	Hooks         []HookInfo        `json:"hooks"`
	Notes         string            `json:"notes"`
	
	// Metadata
	Description   string            `json:"description"`
	Labels        map[string]string `json:"labels"`
	Annotations   map[string]string `json:"annotations"`
	
	// History and revisions
	History       []ReleaseRevision `json:"history"`
	
	// Health and status
	Health        *HealthStatus     `json:"health"`
	TestStatus    string            `json:"test_status"`
	
	// Size and complexity
	Size          int64             `json:"size"`
	ResourceCount int               `json:"resource_count"`
	
	// Custom metadata
	CustomFields  map[string]interface{} `json:"custom_fields"`
}

// ReleaseRevision represents a single revision in release history
type ReleaseRevision struct {
	Revision      int               `json:"revision"`
	Updated       time.Time         `json:"updated"`
	Status        string            `json:"status"`
	Chart         string            `json:"chart"`
	ChartVersion  string            `json:"chart_version"`
	AppVersion    string            `json:"app_version"`
	Description   string            `json:"description"`
	Values        map[string]interface{} `json:"values"`
	Size          int64             `json:"size"`
	Duration      time.Duration     `json:"duration"`
}

// ReleaseMetrics represents metrics for a release
type ReleaseMetrics struct {
	Name              string                 `json:"name"`
	Namespace         string                 `json:"namespace"`
	CollectionTime    time.Time              `json:"collection_time"`
	
	// Deployment metrics
	DeploymentCount   int                    `json:"deployment_count"`
	SuccessfulDeploys int                    `json:"successful_deploys"`
	FailedDeploys     int                    `json:"failed_deploys"`
	RollbackCount     int                    `json:"rollback_count"`
	
	// Performance metrics
	AverageDeployTime time.Duration          `json:"average_deploy_time"`
	LastDeployTime    time.Duration          `json:"last_deploy_time"`
	UptimePercentage  float64                `json:"uptime_percentage"`
	
	// Resource metrics
	ResourceUsage     *ResourceUsage         `json:"resource_usage"`
	
	// Health metrics
	HealthChecks      int                    `json:"health_checks"`
	HealthyChecks     int                    `json:"healthy_checks"`
	UnhealthyChecks   int                    `json:"unhealthy_checks"`
	
	// Error metrics
	ErrorCount        int                    `json:"error_count"`
	WarningCount      int                    `json:"warning_count"`
	
	// History and trends
	DeploymentHistory []DeploymentMetric     `json:"deployment_history"`
	Trends           map[string][]DataPoint `json:"trends"`
}

// DeploymentMetric represents metrics for a single deployment
type DeploymentMetric struct {
	Revision      int           `json:"revision"`
	Timestamp     time.Time     `json:"timestamp"`
	Duration      time.Duration `json:"duration"`
	Status        string        `json:"status"`
	ResourceCount int           `json:"resource_count"`
	Size          int64         `json:"size"`
}

// ChartRepository represents a Helm chart repository
type ChartRepository struct {
	Name         string            `json:"name"`
	URL          string            `json:"url"`
	Username     string            `json:"username"`
	Password     string            `json:"password"`
	CertFile     string            `json:"cert_file"`
	KeyFile      string            `json:"key_file"`
	CAFile       string            `json:"ca_file"`
	Insecure     bool              `json:"insecure"`
	
	// Metadata
	Description  string            `json:"description"`
	Official     bool              `json:"official"`
	Verified     bool              `json:"verified"`
	
	// Status
	LastSync     time.Time         `json:"last_sync"`
	Status       string            `json:"status"`
	ChartCount   int               `json:"chart_count"`
	
	// Configuration
	SyncInterval time.Duration     `json:"sync_interval"`
	Timeout      time.Duration     `json:"timeout"`
	
	// Custom fields
	Annotations  map[string]string `json:"annotations"`
	Labels       map[string]string `json:"labels"`
}

// RepositoryIndex represents the index of a chart repository
type RepositoryIndex struct {
	APIVersion string                          `json:"api_version"`
	Generated  time.Time                       `json:"generated"`
	Entries    map[string][]*ChartMetadata     `json:"entries"`
	PublicKeys []string                        `json:"public_keys"`
	Annotations map[string]string              `json:"annotations"`
}

// Constants for chart and release metadata
const (
	// Chart types
	ChartTypeApplication = "application"
	ChartTypeLibrary     = "library"
	
	// Release statuses (avoiding duplicates with helm_types.go)
	ReleaseStatusDeployed      = "deployed"
	ReleaseStatusUninstalled   = "uninstalled"
	ReleaseStatusUninstalling  = "uninstalling"
	ReleaseStatusPendingInstall = "pending-install"
	ReleaseStatusPendingUpgrade = "pending-upgrade"
	ReleaseStatusPendingRollback = "pending-rollback"
	
	// Sort options
	SortByName         = "name"
	SortByDate         = "date"
	SortByStatus       = "status"
	SortByRevision     = "revision"
	SortByNamespace    = "namespace"
	
	SortOrderAsc       = "asc"
	SortOrderDesc      = "desc"
	
	// Quality issue types
	QualityIssueLint          = "lint"
	QualityIssueSecurity      = "security"
	QualityIssueDocumentation = "documentation"
	QualityIssuePerformance   = "performance"
	
	// Quality issue severities
	QualityIssuesSeverityLow      = "low"
	QualityIssuesSeverityMedium   = "medium"
	QualityIssuesSeverityHigh     = "high"
	QualityIssuesSeverityCritical = "critical"
)