// PHASE R3: Monitoring types and data structures
package monitoring

import (
	"time"

	"deploy-cli/domain"
)

// Dashboard-related types

// DashboardState represents the current state of the monitoring dashboard
type DashboardState struct {
	Environment     domain.Environment
	RefreshInterval time.Duration
	Filter          string
	CompactMode     bool
	ShowMetrics     bool
	ShowLogs        bool
	Interactive     bool
	LastUpdate      time.Time
}

// ClusterOverview provides cluster-wide overview information
type ClusterOverview struct {
	Environment   domain.Environment
	NodesCount    int
	NodesReady    int
	PodsTotal     int
	PodsRunning   int
	PodsPending   int
	PodsFailed    int
	ServicesTotal int
	LastUpdate    time.Time
}

// Services monitoring types

// ServicesMonitoring represents the state of services monitoring
type ServicesMonitoring struct {
	Services    []string
	Environment domain.Environment
	Options     *ServicesOptions
	LastUpdate  time.Time
}

// ServiceStatus represents the status of a single service
type ServiceStatus struct {
	Name      string
	Status    string
	Pods      string
	Age       string
	Ready     bool
	LastSeen  time.Time
	Error     string
	Namespace string
	
	// Extended information for detailed view
	CPU     float64
	Memory  float64
	Disk    float64
	Network float64
}

// Metrics-related types

// MetricsCollection represents a metrics collection session
type MetricsCollection struct {
	Environment domain.Environment
	StartTime   time.Time
	Duration    time.Duration
	Interval    time.Duration
	Focus       []string
	Analyze     bool
	OutputPath  string
}

// MetricsOptions represents metrics collection configuration
type MetricsOptions struct {
	Duration   time.Duration
	Interval   time.Duration
	Focus      []string
	Analyze    bool
	OutputPath string
}

// MetricsSnapshot represents a point-in-time metrics snapshot
type MetricsSnapshot struct {
	Timestamp   time.Time
	CPUUsage    float64
	MemoryUsage float64
	DiskUsage   float64
	NetworkIO   float64
	
	// Service-specific metrics
	ServiceMetrics map[string]ServiceMetrics
}

// ServiceMetrics represents metrics for a specific service
type ServiceMetrics struct {
	Name           string
	CPUUsage       float64
	MemoryUsage    float64
	NetworkIn      float64
	NetworkOut     float64
	RequestRate    float64
	ResponseTime   float64
	ErrorRate      float64
	InstanceCount  int
	LastUpdate     time.Time
}

// MetricsSample represents a single metrics data point
type MetricsSample struct {
	Timestamp time.Time
	Values    map[string]float64
}

// Logs-related types

// LogsOptions represents log monitoring configuration
type LogsOptions struct {
	Services  []string
	Follow    bool
	Lines     int
	Since     time.Duration
	Level     string
	Format    string
	Tail      bool
}

// LogEntry represents a single log entry
type LogEntry struct {
	Timestamp time.Time
	Service   string
	Pod       string
	Level     string
	Message   string
	Source    string
}

// LogStream represents a stream of log entries
type LogStream struct {
	Service   string
	Entries   chan LogEntry
	Errors    chan error
	Done      chan struct{}
}

// Report-related types

// ReportOptions represents monitoring report configuration
type ReportOptions struct {
	Environment   domain.Environment
	TimeRange     time.Duration
	Services      []string
	IncludeMetrics bool
	IncludeLogs   bool
	Format        string
	OutputPath    string
}

// MonitoringReport represents a comprehensive monitoring report
type MonitoringReport struct {
	Environment    domain.Environment
	GeneratedAt    time.Time
	TimeRange      time.Duration
	
	// Summary information
	Summary        ReportSummary
	
	// Detailed sections
	Services       []ServiceReport
	Metrics        MetricsReport
	Incidents      []IncidentReport
	Recommendations []string
}

// ReportSummary provides high-level report summary
type ReportSummary struct {
	TotalServices      int
	HealthyServices    int
	UnhealthyServices  int
	AverageUptime      float64
	TotalIncidents     int
	CriticalIncidents  int
}

// ServiceReport represents detailed service information in a report
type ServiceReport struct {
	Name            string
	Status          string
	Uptime          float64
	AverageMetrics  ServiceMetrics
	Incidents       []IncidentReport
	Recommendations []string
}

// MetricsReport represents metrics analysis in a report
type MetricsReport struct {
	TimeRange         time.Duration
	SamplesCollected  int
	AverageMetrics    MetricsSnapshot
	PeakMetrics       MetricsSnapshot
	TrendAnalysis     TrendAnalysis
}

// TrendAnalysis represents trend analysis results
type TrendAnalysis struct {
	CPUTrend     string // "increasing", "decreasing", "stable"
	MemoryTrend  string
	DiskTrend    string
	NetworkTrend string
	Predictions  map[string]float64 // Predicted values for next period
}

// Alerts-related types

// AlertsOptions represents alert management configuration
type AlertsOptions struct {
	Environment domain.Environment
	Rules       []AlertRule
	Webhooks    []string
	Channels    []string
}

// AlertRule represents a monitoring alert rule
type AlertRule struct {
	Name        string
	Service     string
	Metric      string
	Condition   string // "gt", "lt", "eq"
	Threshold   float64
	Duration    time.Duration
	Severity    string // "critical", "warning", "info"
	Enabled     bool
}

// Alert represents an active or historical alert
type Alert struct {
	ID          string
	Rule        AlertRule
	TriggeredAt time.Time
	ResolvedAt  time.Time
	Status      string // "active", "resolved", "silenced"
	Value       float64
	Message     string
}

// IncidentReport represents a service incident
type IncidentReport struct {
	ID          string
	Service     string
	StartTime   time.Time
	EndTime     time.Time
	Duration    time.Duration
	Severity    string
	Description string
	Impact      string
	Resolution  string
}

// Health check types

// HealthCheckResult represents the result of a health check
type HealthCheckResult struct {
	Service     string
	Endpoint    string
	Status      string
	ResponseTime time.Duration
	Error       error
	CheckedAt   time.Time
}

// HealthCheckSummary represents aggregated health check results
type HealthCheckSummary struct {
	TotalChecks   int
	HealthyChecks int
	UnhealthyChecks int
	AverageResponseTime time.Duration
	LastUpdate  time.Time
}