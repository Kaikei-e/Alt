package domain

import (
	"time"
)

// HealthStatus represents the health status of a component
type HealthStatus struct {
	Status        string                 `json:"status"`
	OverallStatus string                 `json:"overall_status"` // Compatibility field
	Target        string                 `json:"target"`         // Target being checked
	Message       string                 `json:"message"`
	CheckTime     time.Time              `json:"check_time"`
	Details       map[string]interface{} `json:"details"`
	Score         int                    `json:"score"`         // 0-100 health score
	LastHealthy   *time.Time             `json:"last_healthy"`  // When it was last healthy
	Degraded      bool                   `json:"degraded"`      // If partially functional
	Critical      bool                   `json:"critical"`      // If critical failure
	Dependencies  []string               `json:"dependencies"`  // Dependencies checked
	Recommendations []string            `json:"recommendations"` // Health improvement suggestions
}

// ChartHealthStatus represents the health status of a chart
type ChartHealthStatus struct {
	ChartName     string                    `json:"chart_name"`
	Namespace     string                    `json:"namespace"`
	OverallHealth *HealthStatus             `json:"overall_health"`
	OverallStatus string                    `json:"overall_status"`  // String version of overall status
	PodHealth     map[string]*HealthStatus  `json:"pod_health"`
	ServiceHealth map[string]*HealthStatus  `json:"service_health"`
	IngressHealth map[string]*HealthStatus  `json:"ingress_health"`
	VolumeHealth  map[string]*HealthStatus  `json:"volume_health"`
	Resources     []ResourceHealthStatus    `json:"resources"`       // List of resource health statuses
	CheckTime     time.Time                 `json:"check_time"`
	Duration      time.Duration             `json:"duration"`
}

// HealthTarget represents a target for health checking
type HealthTarget struct {
	Type        string            `json:"type"`        // pod, service, deployment, etc.
	Name        string            `json:"name"`
	Namespace   string            `json:"namespace"`
	Selector    map[string]string `json:"selector"`
	Port        int               `json:"port"`
	Path        string            `json:"path"`        // For HTTP health checks
	Timeout     time.Duration     `json:"timeout"`
	MaxRetries  int               `json:"max_retries"`
	RetryDelay  time.Duration     `json:"retry_delay"`
}

// MonitoringOptions represents monitoring configuration
type MonitoringOptions struct {
	// Basic monitoring settings
	Interval          time.Duration  `json:"interval"`           // How often to check
	CheckInterval     time.Duration  `json:"check_interval"`     // Alternative check interval field
	Timeout           time.Duration  `json:"timeout"`            // Per-check timeout
	MaxRetries        int            `json:"max_retries"`        // Retry failed checks
	FailureThreshold  int            `json:"failure_threshold"`  // Failures before marking unhealthy
	
	// What to monitor
	IncludePods       bool           `json:"include_pods"`
	IncludeServices   bool           `json:"include_services"`
	IncludeIngress    bool           `json:"include_ingress"`
	IncludeVolumes    bool           `json:"include_volumes"`
	IncludeMetrics    bool           `json:"include_metrics"`
	
	// Filtering options
	NamespaceFilter   []string       `json:"namespace_filter"`
	LabelSelector     map[string]string `json:"label_selector"`
	ExcludeLabels     map[string]string `json:"exclude_labels"`
	
	// Alert settings
	EnableAlerts      bool           `json:"enable_alerts"`
	AlertChannels     []string       `json:"alert_channels"`
	AlertThresholds   map[string]float64 `json:"alert_thresholds"`
	
	// Output settings
	Format            string         `json:"format"`             // json, yaml, table
	Verbose           bool           `json:"verbose"`
	ShowOnlyFailed    bool           `json:"show_only_failed"`
}

// MonitoringResult represents the result of monitoring operations
type MonitoringResult struct {
	ID                string                  `json:"id"`
	DeploymentID      string                  `json:"deployment_id"`
	Status            MonitoringStatus        `json:"status"`
	StartTime         time.Time               `json:"start_time"`
	EndTime           time.Time               `json:"end_time"`
	Duration          time.Duration           `json:"duration"`
	TotalChecks       int                     `json:"total_checks"`
	HealthyChecks     int                     `json:"healthy_checks"`
	UnhealthyChecks   int                     `json:"unhealthy_checks"`
	FailedChecks      int                     `json:"failed_checks"`
	OverallHealth     string                  `json:"overall_health"`    // healthy, degraded, unhealthy
	HealthScore       int                     `json:"health_score"`      // 0-100
	Events            []*MonitoringEvent      `json:"events"`
	
	// Detailed results
	NamespaceHealth   map[string]*HealthStatus     `json:"namespace_health"`
	ChartHealth       map[string]*ChartHealthStatus `json:"chart_health"`
	ComponentHealth   map[string]*HealthStatus     `json:"component_health"`
	
	// Metrics and insights
	Metrics           *MonitoringMetrics      `json:"metrics"`
	Alerts            []MonitoringAlert       `json:"alerts"`
	Recommendations   []string                `json:"recommendations"`
	Insights          []string                `json:"insights"`
}

// MonitoringEvent represents real-time monitoring events
type MonitoringEvent struct {
	EventID       string                 `json:"event_id"`
	Timestamp     time.Time              `json:"timestamp"`
	EventType     string                 `json:"event_type"`     // health_change, alert, metric_update
	Level         string                 `json:"level"`          // info, warning, error, critical
	Source        string                 `json:"source"`         // What component generated this
	Target        *HealthTarget          `json:"target"`
	PreviousState *HealthStatus          `json:"previous_state"`
	CurrentState  *HealthStatus          `json:"current_state"`
	Message       string                 `json:"message"`
	Severity      string                 `json:"severity"`       // info, warning, error, critical
	Details       string                 `json:"details"`        // Additional details
	DeploymentID  string                 `json:"deployment_id"`  // Associated deployment
	Context       map[string]interface{} `json:"context"`
	Resolved      bool                   `json:"resolved"`
}

// MonitoringMetrics represents metrics collected during monitoring
type MonitoringMetrics struct {
	CollectionTime    time.Time         `json:"collection_time"`
	ResourceUsage     *ResourceUsage    `json:"resource_usage"`
	PerformanceMetrics *PerformanceInfo `json:"performance_metrics"`
	AvailabilityMetrics *AvailabilityInfo `json:"availability_metrics"`
	ThroughputMetrics *ThroughputInfo   `json:"throughput_metrics"`
}

// ResourceUsage represents resource utilization
type ResourceUsage struct {
	CPUUsage      float64  `json:"cpu_usage"`       // Percentage
	MemoryUsage   float64  `json:"memory_usage"`    // Percentage
	DiskUsage     float64  `json:"disk_usage"`      // Percentage
	NetworkIO     float64  `json:"network_io"`      // Bytes per second
	PodCount      int      `json:"pod_count"`
	NodeCount     int      `json:"node_count"`
	NamespaceCount int     `json:"namespace_count"`
}

// PerformanceInfo represents performance metrics
type PerformanceInfo struct {
	AverageResponseTime  time.Duration `json:"average_response_time"`
	P95ResponseTime      time.Duration `json:"p95_response_time"`
	P99ResponseTime      time.Duration `json:"p99_response_time"`
	ErrorRate            float64       `json:"error_rate"`      // Percentage
	RequestRate          float64       `json:"request_rate"`    // Per second
	ConcurrentConnections int          `json:"concurrent_connections"`
}

// AvailabilityInfo represents availability metrics
type AvailabilityInfo struct {
	Uptime              time.Duration `json:"uptime"`
	UptimePercentage    float64       `json:"uptime_percentage"`
	DowntimeEvents      int           `json:"downtime_events"`
	MeanTimeToRestore   time.Duration `json:"mean_time_to_restore"`
	MeanTimeBetweenFailures time.Duration `json:"mean_time_between_failures"`
}

// ThroughputInfo represents throughput metrics
type ThroughputInfo struct {
	RequestsPerSecond   float64 `json:"requests_per_second"`
	BytesPerSecond      float64 `json:"bytes_per_second"`
	TransactionsPerSecond float64 `json:"transactions_per_second"`
	PeakThroughput      float64 `json:"peak_throughput"`
	AverageThroughput   float64 `json:"average_throughput"`
}

// MonitoringAlert represents an alert generated during monitoring
type MonitoringAlert struct {
	AlertID       string                 `json:"alert_id"`
	Timestamp     time.Time              `json:"timestamp"`
	AlertType     string                 `json:"alert_type"`     // threshold, anomaly, pattern
	Severity      string                 `json:"severity"`       // info, warning, critical
	Source        string                 `json:"source"`
	Target        string                 `json:"target"`
	Message       string                 `json:"message"`
	Details       map[string]interface{} `json:"details"`
	Threshold     float64                `json:"threshold"`
	ActualValue   float64                `json:"actual_value"`
	Resolved      bool                   `json:"resolved"`
	ResolvedAt    *time.Time             `json:"resolved_at"`
	Actions       []string               `json:"actions"`        // Suggested actions
}

// HealthStatusType constants for health status
const (
	HealthStatusHealthy   = "healthy"
	HealthStatusDegraded  = "degraded"
	HealthStatusUnhealthy = "unhealthy"
	HealthStatusUnknown   = "unknown"
	HealthStatusStarting  = "starting"
	HealthStatusStopping  = "stopping"
)

// MonitoringEventType constants for event types
const (
	EventTypeHealthChange = "health_change"
	EventTypeAlert        = "alert"
	EventTypeMetricUpdate = "metric_update"
	EventTypeThreshold    = "threshold"
	EventTypeAnomaly      = "anomaly"
)

// MonitoringStatus constants for monitoring status
type MonitoringStatus string

const (
	MonitoringStatusActive    MonitoringStatus = "active"
	MonitoringStatusCompleted MonitoringStatus = "completed"
	MonitoringStatusFailed    MonitoringStatus = "failed"
	MonitoringStatusStopped   MonitoringStatus = "stopped"
	MonitoringStatusPending   MonitoringStatus = "pending"
)

// Additional metrics types needed for MetricsCollectorPort

// NodeMetrics represents metrics for a Kubernetes node
type NodeMetrics struct {
	NodeName        string             `json:"node_name"`
	CollectionTime  time.Time          `json:"collection_time"`
	ResourceUsage   *ResourceUsage     `json:"resource_usage"`
	Conditions      []NodeCondition    `json:"conditions"`
	Capacity        map[string]string  `json:"capacity"`
	Allocatable     map[string]string  `json:"allocatable"`
	PodCount        int                `json:"pod_count"`
	PodCapacity     int                `json:"pod_capacity"`
}

// NodeCondition represents a node condition
type NodeCondition struct {
	Type    string    `json:"type"`
	Status  string    `json:"status"`
	Reason  string    `json:"reason"`
	Message string    `json:"message"`
	LastTransitionTime time.Time `json:"last_transition_time"`
}

// PodMetrics represents metrics for a Kubernetes pod
type PodMetrics struct {
	PodName         string             `json:"pod_name"`
	Namespace       string             `json:"namespace"`
	CollectionTime  time.Time          `json:"collection_time"`
	Phase           string             `json:"phase"`
	ResourceUsage   *ResourceUsage     `json:"resource_usage"`
	ContainerMetrics []ContainerMetrics `json:"container_metrics"`
	RestartCount    int                `json:"restart_count"`
	Age             time.Duration      `json:"age"`
}

// ContainerMetrics represents metrics for a container
type ContainerMetrics struct {
	Name          string         `json:"name"`
	CPUUsage      float64        `json:"cpu_usage"`
	MemoryUsage   float64        `json:"memory_usage"`
	RestartCount  int            `json:"restart_count"`
	State         string         `json:"state"`
	Ready         bool           `json:"ready"`
}

// MetricData represents a single metric data point
type MetricData struct {
	MetricName  string                 `json:"metric_name"`
	Timestamp   time.Time              `json:"timestamp"`
	Value       float64                `json:"value"`
	Labels      map[string]string      `json:"labels"`
	Metadata    map[string]interface{} `json:"metadata"`
	Source      string                 `json:"source"`
	Unit        string                 `json:"unit"`
}

// CustomMetric represents a custom metric
type CustomMetric struct {
	Name        string                 `json:"name"`
	Value       float64                `json:"value"`
	Timestamp   time.Time              `json:"timestamp"`
	Labels      map[string]string      `json:"labels"`
	Description string                 `json:"description"`
	Unit        string                 `json:"unit"`
	Type        string                 `json:"type"`        // counter, gauge, histogram
	Metadata    map[string]interface{} `json:"metadata"`
}

// MetricsQuery represents a query for metrics
type MetricsQuery struct {
	MetricNames []string              `json:"metric_names"`
	Labels      map[string]string     `json:"labels"`
	TimeRange   *TimeRange            `json:"time_range"`
	Aggregation string                `json:"aggregation"`  // avg, sum, max, min
	GroupBy     []string              `json:"group_by"`
	Limit       int                   `json:"limit"`
}

// ResourceHealthStatus represents the health status of a Kubernetes resource
type ResourceHealthStatus struct {
	Name         string            `json:"name"`
	Kind         string            `json:"kind"`
	Namespace    string            `json:"namespace"`
	Status       string            `json:"status"`        // Ready, NotReady, Error, etc.
	Health       string            `json:"health"`        // Healthy, Unhealthy, Unknown
	Ready        bool              `json:"ready"`
	Replicas     int32             `json:"replicas"`
	ReadyReplicas int32            `json:"ready_replicas"`
	UpdatedReplicas int32          `json:"updated_replicas"`
	AvailableReplicas int32        `json:"available_replicas"`
	LastUpdated  time.Time         `json:"last_updated"`
	Conditions   []ResourceCondition `json:"conditions"`
	Events       []ResourceEvent   `json:"events"`
	Metadata     map[string]interface{} `json:"metadata"`
}

// ResourceCondition represents a condition of a Kubernetes resource
type ResourceCondition struct {
	Type               string    `json:"type"`
	Status             string    `json:"status"`
	LastTransitionTime time.Time `json:"last_transition_time"`
	Reason             string    `json:"reason"`
	Message            string    `json:"message"`
}

// ResourceEvent represents an event related to a Kubernetes resource
type ResourceEvent struct {
	Type         string    `json:"type"`        // Normal, Warning
	Reason       string    `json:"reason"`
	Message      string    `json:"message"`
	FirstSeen    time.Time `json:"first_seen"`
	LastSeen     time.Time `json:"last_seen"`
	Count        int32     `json:"count"`
	Source       string    `json:"source"`
}

// PodStatus represents the status of a Kubernetes pod
type PodStatus struct {
	Name      string    `json:"name"`
	Namespace string    `json:"namespace"`
	Phase     string    `json:"phase"`
	Ready     bool      `json:"ready"`
	Status    string    `json:"status"`
	NodeName  string    `json:"node_name"`
	PodIP     string    `json:"pod_ip"`
	StartTime time.Time `json:"start_time"`
}

// MetricsResult represents the result of a metrics query
type MetricsResult struct {
	Query       *MetricsQuery   `json:"query"`
	Data        []MetricData    `json:"data"`
	TotalCount  int             `json:"total_count"`
	Duration    time.Duration   `json:"duration"`
	Timestamp   time.Time       `json:"timestamp"`
}

// MetricAlert represents a metric-based alert
type MetricAlert struct {
	AlertID       string                 `json:"alert_id"`
	Name          string                 `json:"name"`
	MetricName    string                 `json:"metric_name"`
	Condition     string                 `json:"condition"`     // >, <, ==, !=
	Threshold     float64                `json:"threshold"`
	Duration      time.Duration          `json:"duration"`      // Alert duration
	Labels        map[string]string      `json:"labels"`
	Annotations   map[string]string      `json:"annotations"`
	State         string                 `json:"state"`         // pending, firing, resolved
	ActiveSince   *time.Time             `json:"active_since"`
	ResolvedAt    *time.Time             `json:"resolved_at"`
	FireCount     int                    `json:"fire_count"`
}

// MetricsAggregation represents aggregation parameters
type MetricsAggregation struct {
	MetricNames  []string              `json:"metric_names"`
	TimeRange    *TimeRange            `json:"time_range"`
	GroupBy      []string              `json:"group_by"`
	Aggregations []string              `json:"aggregations"`  // avg, sum, max, min, count
	Interval     time.Duration         `json:"interval"`
	Filters      map[string]string     `json:"filters"`
}

// AggregationResult represents the result of metrics aggregation
type AggregationResult struct {
	Aggregation  *MetricsAggregation   `json:"aggregation"`
	Results      []AggregationData     `json:"results"`
	TotalCount   int                   `json:"total_count"`
	Duration     time.Duration         `json:"duration"`
	Timestamp    time.Time             `json:"timestamp"`
}

// AggregationData represents aggregated data points
type AggregationData struct {
	GroupKey     map[string]string     `json:"group_key"`
	Values       map[string]float64    `json:"values"`      // aggregation_type -> value
	DataPoints   []DataPoint           `json:"data_points"`
	Count        int                   `json:"count"`
}

// ReportConfig represents configuration for metrics reports
type ReportConfig struct {
	Name         string                 `json:"name"`
	MetricNames  []string               `json:"metric_names"`
	TimeRange    *TimeRange             `json:"time_range"`
	Format       string                 `json:"format"`       // json, csv, pdf, html
	Recipients   []string               `json:"recipients"`
	Schedule     string                 `json:"schedule"`     // cron expression
	Template     string                 `json:"template"`
	Filters      map[string]string      `json:"filters"`
	Aggregations []string               `json:"aggregations"`
}

// MetricsReport represents a generated metrics report
type MetricsReport struct {
	ReportID     string                 `json:"report_id"`
	Config       *ReportConfig          `json:"config"`
	GeneratedAt  time.Time              `json:"generated_at"`
	Duration     time.Duration          `json:"duration"`
	Data         []MetricData           `json:"data"`
	Summary      map[string]interface{} `json:"summary"`
	Charts       []ChartData            `json:"charts"`
	Content      string                 `json:"content"`     // Rendered content
	Size         int64                  `json:"size"`
}

// ChartData represents data for charts in reports
type ChartData struct {
	Type        string      `json:"type"`        // line, bar, pie, gauge
	Title       string      `json:"title"`
	Data        []DataPoint `json:"data"`
	Labels      []string    `json:"labels"`
	Colors      []string    `json:"colors"`
	Options     map[string]interface{} `json:"options"`
}

// StreamOptions represents options for streaming metrics
type StreamOptions struct {
	MetricNames  []string              `json:"metric_names"`
	Labels       map[string]string     `json:"labels"`
	Interval     time.Duration         `json:"interval"`
	BufferSize   int                   `json:"buffer_size"`
	Filters      map[string]string     `json:"filters"`
}

// Note: TimeRange is already defined in metadata_types.go

// AlertSeverity constants for alert severity levels
const (
	AlertSeverityInfo     = "info"
	AlertSeverityWarning  = "warning"
	AlertSeverityError    = "error"
	AlertSeverityCritical = "critical"
)

// HealthTargetType constants for health target types
const (
	TargetTypePod         = "pod"
	TargetTypeService     = "service"
	TargetTypeDeployment  = "deployment"
	TargetTypeStatefulSet = "statefulset"
	TargetTypeIngress     = "ingress"
	TargetTypeNode        = "node"
	TargetTypeNamespace   = "namespace"
)

// Metric types
const (
	MetricTypeCounter   = "counter"
	MetricTypeGauge     = "gauge"
	MetricTypeHistogram = "histogram"
	MetricTypeSummary   = "summary"
)

// Alert states
const (
	AlertStatePending  = "pending"
	AlertStateFiring   = "firing"
	AlertStateResolved = "resolved"
)

// Report formats
const (
	ReportFormatJSON = "json"
	ReportFormatCSV  = "csv"
	ReportFormatPDF  = "pdf"
	ReportFormatHTML = "html"
)

// Monitoring status constants
const (
	MonitoringStatusInactive  = "inactive"
	MonitoringStatusCancelled = "cancelled"
	MonitoringStatusTimeout   = "timeout"
	MonitoringStatusError     = "error"
)

// Event level constants
const (
	EventLevelInfo     = "info"
	EventLevelWarning  = "warning"
	EventLevelError    = "error"
	EventLevelCritical = "critical"
	EventLevelDebug    = "debug"
)