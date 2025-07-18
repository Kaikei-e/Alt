package domain

import (
	"fmt"
	"time"
)

// DeploymentOptions holds the deployment configuration options
type DeploymentOptions struct {
	Environment       Environment
	DryRun            bool
	DoRestart         bool
	ForceUpdate       bool
	TargetNamespace   string
	ImagePrefix       string
	TagBase           string
	ChartsDir         string
	Timeout           time.Duration
	DeploymentStrategy DeploymentStrategy
	StrategyName      string // Override strategy selection
}

// NewDeploymentOptions creates a new deployment options with defaults
func NewDeploymentOptions() *DeploymentOptions {
	return &DeploymentOptions{
		DryRun:      false,
		DoRestart:   false,
		ForceUpdate: false,
		ChartsDir:   "../charts",
		Timeout:     300 * time.Second,
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
	ChartName     string
	Namespace     string
	Status        DeploymentStatus
	Error         error
	Duration      time.Duration
	StartTime     time.Time
	Message       string
}

// DeploymentStatus represents the status of a deployment
type DeploymentStatus string

const (
	DeploymentStatusSuccess DeploymentStatus = "success"
	DeploymentStatusFailed  DeploymentStatus = "failed"
	DeploymentStatusSkipped DeploymentStatus = "skipped"
)

// String returns the string representation of the deployment status
func (s DeploymentStatus) String() string {
	return string(s)
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
	Name           string
	Namespace      string
	Replicas       int32
	ReadyReplicas  int32
	UpdatedReplicas int32
	AvailableReplicas int32
	Status         string
	CreationTime   time.Time
}

// StatefulSet represents a Kubernetes StatefulSet
type StatefulSet struct {
	Name           string
	Namespace      string
	Replicas       int32
	ReadyReplicas  int32
	UpdatedReplicas int32
	CurrentReplicas int32
	Status         string
	CreationTime   time.Time
}

// Pod represents a Kubernetes Pod
type Pod struct {
	Name           string
	Namespace      string
	Status         string
	RestartCount   int32
	CreationTime   time.Time
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