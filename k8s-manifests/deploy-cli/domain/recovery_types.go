package domain

import (
	"time"
)

// RecoveryOptions represents options for deployment recovery
type RecoveryOptions struct {
	AutoRollback      bool          `json:"auto_rollback"`
	MaxRetries        int           `json:"max_retries"`
	RetryDelay        time.Duration `json:"retry_delay"`
	CleanupOnFailure  bool          `json:"cleanup_on_failure"`
	PreserveData      bool          `json:"preserve_data"`
	ForceRecovery     bool          `json:"force_recovery"`
	NotifyOnComplete  bool          `json:"notify_on_complete"`
	RecoveryTimeout   time.Duration `json:"recovery_timeout"`
}

// RepairAction represents a repair action to be performed
type RepairAction struct {
	Type        string                 `json:"type"`        // restart, scale, patch, recreate
	Target      string                 `json:"target"`      // deployment, pod, service, etc.
	Resource    string                 `json:"resource"`    // resource name
	Namespace   string                 `json:"namespace"`   // target namespace
	Parameters  map[string]interface{} `json:"parameters"`  // action-specific parameters
	Priority    int                    `json:"priority"`    // execution priority (lower = higher priority)
	Timeout     time.Duration          `json:"timeout"`     // action timeout
	Description string                 `json:"description"` // human-readable description
}

// RepairResult represents the result of a repair operation
type RepairResult struct {
	ActionID    string        `json:"action_id"`
	Action      RepairAction  `json:"action"`
	Success     bool          `json:"success"`
	Error       string        `json:"error,omitempty"`
	Duration    time.Duration `json:"duration"`
	StartTime   time.Time     `json:"start_time"`
	EndTime     time.Time     `json:"end_time"`
	Message     string        `json:"message"`
	Retries     int           `json:"retries"`
	Details     map[string]interface{} `json:"details,omitempty"`
}

// DiagnosisResult represents the result of deployment diagnosis
type DiagnosisResult struct {
	DeploymentID   string                 `json:"deployment_id"`
	Status         string                 `json:"status"` // healthy, degraded, failed, unknown
	Issues         []DiagnosisIssue       `json:"issues"`
	Recommendations []RepairAction        `json:"recommendations"`
	HealthScore    int                    `json:"health_score"` // 0-100
	Timestamp      time.Time              `json:"timestamp"`
	Metadata       map[string]interface{} `json:"metadata,omitempty"`
}

// DiagnosisIssue represents an identified issue
type DiagnosisIssue struct {
	Type        string                 `json:"type"`        // pod_failure, resource_limit, network, etc.
	Severity    string                 `json:"severity"`    // critical, warning, info
	Resource    string                 `json:"resource"`    // affected resource name
	Namespace   string                 `json:"namespace"`   // affected namespace
	Description string                 `json:"description"` // issue description
	Cause       string                 `json:"cause"`       // root cause analysis
	Impact      string                 `json:"impact"`      // impact description
	Details     map[string]interface{} `json:"details,omitempty"`
}

// Note: RecoveryResult already exists in domain/error_types.go
// We use that existing definition instead of creating a duplicate

// ChartRevision represents a chart revision for rollback purposes
type ChartRevision struct {
	Revision     int       `json:"revision"`
	Chart        string    `json:"chart"`
	Status       string    `json:"status"`
	LastDeployed time.Time `json:"last_deployed"`
	Description  string    `json:"description"`
}