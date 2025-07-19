package domain

import (
	"time"
)

// DeploymentMonitor defines the interface for deployment monitoring
type DeploymentMonitor interface {
	// StartDeployment starts monitoring a deployment
	StartDeployment(deploymentID string, options *DeploymentOptions) error

	// UpdateLayerProgress updates the progress of a layer
	UpdateLayerProgress(deploymentID, layerName string, progress LayerProgress) error

	// UpdateChartProgress updates the progress of a chart
	UpdateChartProgress(deploymentID, layerName, chartName string, progress ChartProgress) error

	// CompleteDeployment marks a deployment as complete
	CompleteDeployment(deploymentID string, result DeploymentResult) error

	// GetDeploymentMetrics returns metrics for a deployment
	GetDeploymentMetrics(deploymentID string) (*DeploymentMetrics, error)

	// GetLayerMetrics returns metrics for a specific layer
	GetLayerMetrics(deploymentID, layerName string) (*LayerMetrics, error)
}

// DeploymentMetrics represents comprehensive deployment metrics
type DeploymentMetrics struct {
	DeploymentID       string              `json:"deployment_id"`
	StartTime          time.Time           `json:"start_time"`
	EndTime            *time.Time          `json:"end_time,omitempty"`
	Duration           time.Duration       `json:"duration"`
	Status             DeploymentStatus    `json:"status"`
	Environment        Environment         `json:"environment"`
	Strategy           string              `json:"strategy"`
	TotalLayers        int                 `json:"total_layers"`
	CompletedLayers    int                 `json:"completed_layers"`
	TotalCharts        int                 `json:"total_charts"`
	CompletedCharts    int                 `json:"completed_charts"`
	FailedCharts       int                 `json:"failed_charts"`
	SkippedCharts      int                 `json:"skipped_charts"`
	LayerMetrics       []LayerMetrics      `json:"layer_metrics"`
	PerformanceMetrics *PerformanceMetrics `json:"performance_metrics"`
	ErrorSummary       []ErrorSummary      `json:"error_summary"`
}

// LayerMetrics represents metrics for a deployment layer
type LayerMetrics struct {
	LayerName          string              `json:"layer_name"`
	StartTime          time.Time           `json:"start_time"`
	EndTime            *time.Time          `json:"end_time,omitempty"`
	Duration           time.Duration       `json:"duration"`
	Status             LayerStatus         `json:"status"`
	TotalCharts        int                 `json:"total_charts"`
	CompletedCharts    int                 `json:"completed_charts"`
	FailedCharts       int                 `json:"failed_charts"`
	SkippedCharts      int                 `json:"skipped_charts"`
	ChartMetrics       []ChartMetrics      `json:"chart_metrics"`
	HealthCheckMetrics *HealthCheckMetrics `json:"health_check_metrics"`
	DependencyMetrics  *DependencyMetrics  `json:"dependency_metrics"`
}

// ChartMetrics represents metrics for a chart deployment
type ChartMetrics struct {
	ChartName         string             `json:"chart_name"`
	Namespace         string             `json:"namespace"`
	StartTime         time.Time          `json:"start_time"`
	EndTime           *time.Time         `json:"end_time,omitempty"`
	Duration          time.Duration      `json:"duration"`
	Status            DeploymentStatus   `json:"status"`
	Retries           int                `json:"retries"`
	ResourceMetrics   *ResourceMetrics   `json:"resource_metrics"`
	HealthCheckResult *HealthCheckResult `json:"health_check_result"`
	ErrorDetails      []ErrorDetail      `json:"error_details"`
}

// PerformanceMetrics represents performance-related metrics
type PerformanceMetrics struct {
	AverageLayerTime        time.Duration `json:"average_layer_time"`
	AverageChartTime        time.Duration `json:"average_chart_time"`
	TotalHealthCheckTime    time.Duration `json:"total_health_check_time"`
	TotalDependencyWaitTime time.Duration `json:"total_dependency_wait_time"`
	ParallelismEfficiency   float64       `json:"parallelism_efficiency"`
	ResourceUtilization     float64       `json:"resource_utilization"`
	OptimizationSuggestions []string      `json:"optimization_suggestions"`
}

// HealthCheckMetrics represents health check metrics
type HealthCheckMetrics struct {
	StartTime          time.Time           `json:"start_time"`
	EndTime            *time.Time          `json:"end_time,omitempty"`
	Duration           time.Duration       `json:"duration"`
	TotalChecks        int                 `json:"total_checks"`
	SuccessfulChecks   int                 `json:"successful_checks"`
	FailedChecks       int                 `json:"failed_checks"`
	AverageCheckTime   time.Duration       `json:"average_check_time"`
	HealthCheckResults []HealthCheckResult `json:"health_check_results"`
}

// DependencyMetrics represents dependency-related metrics
type DependencyMetrics struct {
	TotalDependencies     int           `json:"total_dependencies"`
	ResolvedDependencies  int           `json:"resolved_dependencies"`
	FailedDependencies    int           `json:"failed_dependencies"`
	AverageDependencyTime time.Duration `json:"average_dependency_time"`
	DependencyChain       []string      `json:"dependency_chain"`
	CircularDependencies  []string      `json:"circular_dependencies"`
	CriticalPath          []string      `json:"critical_path"`
}

// ResourceMetrics represents resource utilization metrics
type ResourceMetrics struct {
	CPUUsage     float64 `json:"cpu_usage"`
	MemoryUsage  float64 `json:"memory_usage"`
	NetworkIO    float64 `json:"network_io"`
	DiskIO       float64 `json:"disk_io"`
	PodCount     int     `json:"pod_count"`
	ServiceCount int     `json:"service_count"`
}

// HealthCheckResult represents the result of a health check
type HealthCheckResult struct {
	CheckType  string                 `json:"check_type"`
	Target     string                 `json:"target"`
	Status     string                 `json:"status"`
	StartTime  time.Time              `json:"start_time"`
	Duration   time.Duration          `json:"duration"`
	Message    string                 `json:"message"`
	Details    map[string]interface{} `json:"details"`
	Retries    int                    `json:"retries"`
	MaxRetries int                    `json:"max_retries"`
}

// ErrorSummary represents a summary of errors during deployment
type ErrorSummary struct {
	ErrorType           string    `json:"error_type"`
	ErrorMessage        string    `json:"error_message"`
	Count               int       `json:"count"`
	FirstSeen           time.Time `json:"first_seen"`
	LastSeen            time.Time `json:"last_seen"`
	AffectedComponents  []string  `json:"affected_components"`
	SuggestedResolution string    `json:"suggested_resolution"`
}

// ErrorDetail represents detailed error information
type ErrorDetail struct {
	Timestamp   time.Time              `json:"timestamp"`
	ErrorType   string                 `json:"error_type"`
	Message     string                 `json:"message"`
	StackTrace  string                 `json:"stack_trace,omitempty"`
	Context     map[string]interface{} `json:"context"`
	Severity    ErrorSeverity          `json:"severity"`
	Recoverable bool                   `json:"recoverable"`
}

// LayerProgress represents the progress of a layer
type LayerProgress struct {
	LayerName              string        `json:"layer_name"`
	Status                 LayerStatus   `json:"status"`
	StartTime              time.Time     `json:"start_time"`
	CurrentChart           string        `json:"current_chart"`
	CompletedCharts        int           `json:"completed_charts"`
	TotalCharts            int           `json:"total_charts"`
	ProgressPercent        float64       `json:"progress_percent"`
	EstimatedTimeRemaining time.Duration `json:"estimated_time_remaining"`
	Message                string        `json:"message"`
}

// ChartProgress represents the progress of a chart
type ChartProgress struct {
	ChartName       string           `json:"chart_name"`
	Status          DeploymentStatus `json:"status"`
	StartTime       time.Time        `json:"start_time"`
	CurrentPhase    string           `json:"current_phase"`
	ProgressPercent float64          `json:"progress_percent"`
	Message         string           `json:"message"`
	Retries         int              `json:"retries"`
}

// LayerStatus represents the status of a layer
type LayerStatus string

const (
	LayerStatusPending    LayerStatus = "pending"
	LayerStatusInProgress LayerStatus = "in_progress"
	LayerStatusCompleted  LayerStatus = "completed"
	LayerStatusFailed     LayerStatus = "failed"
	LayerStatusSkipped    LayerStatus = "skipped"
)

// String returns the string representation of the layer status
func (s LayerStatus) String() string {
	return string(s)
}

// ErrorSeverity represents the severity of an error
type ErrorSeverity string

const (
	ErrorSeverityLow      ErrorSeverity = "low"
	ErrorSeverityMedium   ErrorSeverity = "medium"
	ErrorSeverityHigh     ErrorSeverity = "high"
	ErrorSeverityCritical ErrorSeverity = "critical"
)

// String returns the string representation of the error severity
func (s ErrorSeverity) String() string {
	return string(s)
}

// DeploymentAlert represents an alert triggered during deployment
type DeploymentAlert struct {
	AlertID    string                 `json:"alert_id"`
	Timestamp  time.Time              `json:"timestamp"`
	Severity   ErrorSeverity          `json:"severity"`
	Type       string                 `json:"type"`
	Message    string                 `json:"message"`
	Component  string                 `json:"component"`
	Context    map[string]interface{} `json:"context"`
	Resolved   bool                   `json:"resolved"`
	Resolution string                 `json:"resolution,omitempty"`
	ResolvedAt *time.Time             `json:"resolved_at,omitempty"`
}

// OptimizationSuggestion represents a suggestion for deployment optimization
type OptimizationSuggestion struct {
	Type        string    `json:"type"`
	Component   string    `json:"component"`
	Description string    `json:"description"`
	Impact      string    `json:"impact"`
	Effort      string    `json:"effort"`
	Priority    string    `json:"priority"`
	CreatedAt   time.Time `json:"created_at"`
}

// DeploymentInsight represents insights derived from deployment metrics
type DeploymentInsight struct {
	InsightType string                   `json:"insight_type"`
	Title       string                   `json:"title"`
	Description string                   `json:"description"`
	Metrics     map[string]interface{}   `json:"metrics"`
	Suggestions []OptimizationSuggestion `json:"suggestions"`
	CreatedAt   time.Time                `json:"created_at"`
}
