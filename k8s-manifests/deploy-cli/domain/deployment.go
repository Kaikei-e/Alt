package domain

import (
	"fmt"
	"time"
)

// DeploymentOptions holds the deployment configuration options
type DeploymentOptions struct {
	Environment             Environment
	DryRun                  bool
	DoRestart               bool
	ForceUpdate             bool
	TargetNamespace         string
	ImagePrefix             string
	TagBase                 string
	ChartsDir               string
	Timeout                 time.Duration
	DeploymentStrategy      DeploymentStrategy
	StrategyName            string // Override strategy selection
	AutoFixSecrets          bool          // Enable automatic secret error recovery (Phase 4.3)
	AutoCreateNamespaces    bool          // Enable automatic namespace creation if not exists
	AutoFixStorage          bool          // Enable automatic StorageClass configuration
	SkipStatefulSetRecovery bool          // Skip StatefulSet recovery for emergency deployments
	SkipHealthChecks        bool          // Skip all health checks for emergency deployment
	ForceUnlock             bool          // Force cleanup of Helm lock conflicts before deployment
	LockWaitTimeout         time.Duration // Maximum time to wait for Helm lock release
	MaxLockRetries          int           // Maximum number of lock cleanup retry attempts
}

// NewDeploymentOptions creates a new deployment options with defaults
func NewDeploymentOptions() *DeploymentOptions {
	return &DeploymentOptions{
		DryRun:          false,
		DoRestart:       false,
		ForceUpdate:     false,
		ChartsDir:       "../charts",
		Timeout:         300 * time.Second,
		ForceUnlock:     false,
		LockWaitTimeout: 5 * time.Minute,
		MaxLockRetries:  5,
	}
}

// Validate validates the deployment options
func (o *DeploymentOptions) Validate() error {
	if !o.Environment.IsValid() {
		return fmt.Errorf("invalid environment: %s", o.Environment)
	}

	if o.ImagePrefix == "" {
		return fmt.Errorf("IMAGE_PREFIX is required")
	}

	return nil
}

// HelmReleaseStatus represents the status of a Helm release
type HelmReleaseStatus struct {
	Name        string `json:"name"`
	Namespace   string `json:"namespace"`
	Version     int    `json:"version"`
	Status      string `json:"status"`
	Description string `json:"description"`
	LastUpdated string `json:"last_updated"`
	Exists      bool   `json:"exists"`
}

// GetNamespace returns the target namespace, using override if provided
func (o *DeploymentOptions) GetNamespace(chartName string) string {
	if o.TargetNamespace != "" {
		return o.TargetNamespace
	}
	return DetermineNamespace(chartName, o.Environment)
}

// ShouldOverrideImage returns true if image should be overridden
func (o *DeploymentOptions) ShouldOverrideImage() bool {
	return o.TagBase != "" || o.ForceUpdate
}

// GetImageTag returns the image tag for the given chart
func (o *DeploymentOptions) GetImageTag(chartName string) string {
	if o.TagBase != "" {
		// If TagBase is explicitly provided, use it with chart name
		return fmt.Sprintf("%s-%s", chartName, o.TagBase)
	}

	if o.ForceUpdate {
		// For force updates, generate a unique tag to ensure pod updates
		timestamp := time.Now().Unix()
		return fmt.Sprintf("%s-force-%d", o.Environment.String(), timestamp)
	}

	// Default fallback: use environment name
	return o.Environment.String()
}

// HasDeploymentStrategy returns true if a deployment strategy is set
func (o *DeploymentOptions) HasDeploymentStrategy() bool {
	return o.DeploymentStrategy != nil
}

// GetDeploymentStrategy returns the deployment strategy
func (o *DeploymentOptions) GetDeploymentStrategy() DeploymentStrategy {
	return o.DeploymentStrategy
}

// SetDeploymentStrategy sets the deployment strategy
func (o *DeploymentOptions) SetDeploymentStrategy(strategy DeploymentStrategy) {
	o.DeploymentStrategy = strategy
}

// GetLayerConfigurations returns the layer configurations from the deployment strategy
func (o *DeploymentOptions) GetLayerConfigurations() []LayerConfiguration {
	if o.DeploymentStrategy == nil {
		return nil
	}
	return o.DeploymentStrategy.GetLayerConfigurations(o.ChartsDir)
}

// GetStrategyTimeout returns the timeout from the deployment strategy or default
func (o *DeploymentOptions) GetStrategyTimeout() time.Duration {
	if o.DeploymentStrategy != nil {
		return o.DeploymentStrategy.GetGlobalTimeout()
	}
	return o.Timeout
}

// GetStrategyName returns the strategy name if set, otherwise returns environment-based name
func (o *DeploymentOptions) GetStrategyName() string {
	if o.StrategyName != "" {
		return o.StrategyName
	}
	if o.DeploymentStrategy != nil {
		return o.DeploymentStrategy.GetName()
	}
	return o.Environment.String()
}

// DeploymentResult represents the result of a deployment operation
type DeploymentResult struct {
	ChartName string
	Namespace string
	Status    DeploymentStatus
	Error     error
	Duration  time.Duration
	StartTime time.Time
	Message   string
}

// DeploymentStatus represents the status of a deployment
type DeploymentStatus string

const (
	DeploymentStatusSuccess    DeploymentStatus = "success"
	DeploymentStatusFailed     DeploymentStatus = "failed"
	DeploymentStatusSkipped    DeploymentStatus = "skipped"
	DeploymentStatusInProgress DeploymentStatus = "in_progress"
	DeploymentStatusCompleted  DeploymentStatus = "completed"
	DeploymentStatusCancelled  DeploymentStatus = "cancelled"
)

// String returns the string representation of the deployment status
func (s DeploymentStatus) String() string {
	return string(s)
}

// DeploymentStatusInfo represents the current status of a deployment
type DeploymentStatusInfo struct {
	ID               string           `json:"id"`
	Environment      Environment      `json:"environment"`
	Status           DeploymentStatus `json:"status"`
	StartTime        time.Time        `json:"start_time"`
	EndTime          time.Time        `json:"end_time,omitempty"`
	Duration         time.Duration    `json:"duration"`
	Phase            string           `json:"phase"`
	CurrentPhase     string           `json:"current_phase"`
	CurrentChart     string           `json:"current_chart"`
	CompletedCharts  int              `json:"completed_charts"`
	TotalCharts      int              `json:"total_charts"`
	SuccessfulCharts int              `json:"successful_charts"`
	FailedCharts     int              `json:"failed_charts"`
	SkippedCharts    int              `json:"skipped_charts"`
	ProgressPercent  float64          `json:"progress_percent"`
	LastUpdated      time.Time        `json:"last_updated"`
	Error            string           `json:"error,omitempty"`
}

// DeploymentReport represents a comprehensive deployment report
type DeploymentReport struct {
	DeploymentID string                 `json:"deployment_id"`
	Status       *DeploymentStatusInfo  `json:"status"`
	Metrics      map[string]interface{} `json:"metrics"`
	GeneratedAt  time.Time              `json:"generated_at"`
}

// DeploymentProgress represents the progress of a deployment
type DeploymentProgress struct {
	TotalCharts     int
	CompletedCharts int
	CurrentChart    string
	CurrentPhase    string
	Results         []DeploymentResult
}

// NewDeploymentProgress creates a new deployment progress
func NewDeploymentProgress(totalCharts int) *DeploymentProgress {
	return &DeploymentProgress{
		TotalCharts: totalCharts,
		Results:     make([]DeploymentResult, 0, totalCharts),
	}
}

// AddResult adds a deployment result to the progress
func (p *DeploymentProgress) AddResult(result DeploymentResult) {
	p.Results = append(p.Results, result)
	p.CompletedCharts++
}

// GetSuccessCount returns the number of successful deployments
func (p *DeploymentProgress) GetSuccessCount() int {
	count := 0
	for _, result := range p.Results {
		if result.Status == DeploymentStatusSuccess {
			count++
		}
	}
	return count
}

// GetFailedCount returns the number of failed deployments
func (p *DeploymentProgress) GetFailedCount() int {
	count := 0
	for _, result := range p.Results {
		if result.Status == DeploymentStatusFailed {
			count++
		}
	}
	return count
}

// GetSkippedCount returns the number of skipped deployments
func (p *DeploymentProgress) GetSkippedCount() int {
	count := 0
	for _, result := range p.Results {
		if result.Status == DeploymentStatusSkipped {
			count++
		}
	}
	return count
}

// IsComplete returns true if all charts have been processed
func (p *DeploymentProgress) IsComplete() bool {
	return p.CompletedCharts >= p.TotalCharts
}

// Deployment represents a Kubernetes Deployment
type Deployment struct {
	Name              string
	Namespace         string
	Replicas          int32
	ReadyReplicas     int32
	UpdatedReplicas   int32
	AvailableReplicas int32
	Status            string
	CreationTime      time.Time
}

// StatefulSet represents a Kubernetes StatefulSet
type StatefulSet struct {
	Name            string
	Namespace       string
	Replicas        int32
	ReadyReplicas   int32
	UpdatedReplicas int32
	CurrentReplicas int32
	Status          string
	CreationTime    time.Time
}

// Pod represents a Kubernetes Pod
type Pod struct {
	Name         string
	Namespace    string
	Status       string
	RestartCount int32
	CreationTime time.Time
}

// HelmReleaseInfo represents information about a Helm release
type HelmReleaseInfo struct {
	Name       string
	Namespace  string
	Revision   int
	Status     string
	Chart      string
	AppVersion string
	Updated    time.Time
}

// DeploymentCheckpoint represents a snapshot of deployment state for rollback
type DeploymentCheckpoint struct {
	ID          string
	Timestamp   time.Time
	Environment Environment
	Releases    []HelmReleaseInfo
	Namespaces  []string
}
