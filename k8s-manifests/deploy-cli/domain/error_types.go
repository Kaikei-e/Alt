package domain

import (
	"time"
)

// ErrorPattern represents a pattern for error matching and classification
type ErrorPattern struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Pattern     string            `json:"pattern"`      // Regex pattern
	Regex       string            `json:"regex"`        // Alternative regex field
	Category    ErrorCategory     `json:"category"`
	Severity    ErrorSeverity     `json:"severity"`
	Keywords    []string          `json:"keywords"`
	Context     map[string]string `json:"context"`      // Additional context for matching
	Actions     []RecoveryAction  `json:"actions"`      // Suggested recovery actions
	Created     time.Time         `json:"created"`
	Updated     time.Time         `json:"updated"`
}

// ErrorCategory represents categories of errors
type ErrorCategory string

const (
	ErrorCategoryTimeout     ErrorCategory = "timeout"
	ErrorCategoryPermission  ErrorCategory = "permission"
	ErrorCategoryResource    ErrorCategory = "resource"
	ErrorCategoryNetwork     ErrorCategory = "network"
	ErrorCategoryValidation  ErrorCategory = "validation"
	ErrorCategoryDependency  ErrorCategory = "dependency"
	ErrorCategoryUnknown     ErrorCategory = "unknown"
)

// RecoveryAction represents an action that can be taken to recover from an error
type RecoveryAction struct {
	ID          string            `json:"id"`
	Type        RecoveryType      `json:"type"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Command     string            `json:"command"`      // Command to execute
	Parameters  map[string]string `json:"parameters"`   // Parameters for the action
	Timeout     time.Duration     `json:"timeout"`      // Max time to wait for action
	Retries     int               `json:"retries"`      // Max retries
	RetryDelay  time.Duration     `json:"retry_delay"`  // Delay between retries
	Prerequisites []string        `json:"prerequisites"` // Required conditions
	RiskLevel   RiskLevel         `json:"risk_level"`   // Risk assessment
	Manual      bool              `json:"manual"`       // Requires manual intervention
	Automated   bool              `json:"automated"`    // Can be automated
	Destructive bool              `json:"destructive"`  // May cause data loss
	
	// Additional fields for compatibility
	Priority             string `json:"priority"`              // Priority level
	AutoRetry            bool   `json:"auto_retry"`            // Auto retry flag
	RetryCount           int    `json:"retry_count"`           // Retry count alias
	RequiresConfirmation bool   `json:"requires_confirmation"` // Requires confirmation
}

// RecoveryType represents types of recovery actions
type RecoveryType string

const (
	RecoveryTypeRetry      RecoveryType = "retry"
	RecoveryTypeRestart    RecoveryType = "restart"
	RecoveryTypeRollback   RecoveryType = "rollback"
	RecoveryTypeCleanup    RecoveryType = "cleanup"
	RecoveryTypeScale      RecoveryType = "scale"
	RecoveryTypeUpdate     RecoveryType = "update"
	RecoveryTypeWait       RecoveryType = "wait"
	RecoveryTypeManual     RecoveryType = "manual"
	RecoveryTypeForce      RecoveryType = "force"
)

// RecoveryActionType constants for compatibility
const (
	RecoveryActionTypeCleanup = RecoveryTypeCleanup
	RecoveryActionTypeManual  = RecoveryTypeManual
)

// RiskLevel represents the risk level of a recovery action
type RiskLevel string

const (
	RiskLevelLow      RiskLevel = "low"
	RiskLevelMedium   RiskLevel = "medium"
	RiskLevelHigh     RiskLevel = "high"
	RiskLevelCritical RiskLevel = "critical"
)

// ErrorContext represents the context in which an error occurred
type ErrorContext struct {
	Timestamp     time.Time              `json:"timestamp"`
	Component     string                 `json:"component"`
	Operation     string                 `json:"operation"`
	Environment   string                 `json:"environment"`
	Namespace     string                 `json:"namespace"`
	ChartName     string                 `json:"chart_name"`
	ChartVersion  string                 `json:"chart_version"`
	Release       string                 `json:"release"`
	User          string                 `json:"user"`
	Command       string                 `json:"command"`
	Arguments     []string               `json:"arguments"`
	Metadata      map[string]interface{} `json:"metadata"`
	StackTrace    []string               `json:"stack_trace"`
	Dependencies  []string               `json:"dependencies"`
	Resources     []string               `json:"resources"`
}

// RecoveryResult represents the result of a recovery action
type RecoveryResult struct {
	ActionID      string                 `json:"action_id"`
	ActionType    RecoveryType           `json:"action_type"`
	Action        *RecoveryAction        `json:"action"`         // Action that was executed
	Success       bool                   `json:"success"`
	StartTime     time.Time              `json:"start_time"`
	EndTime       time.Time              `json:"end_time"`
	Duration      time.Duration          `json:"duration"`
	Attempts      int                    `json:"attempts"`
	Message       string                 `json:"message"`
	Output        string                 `json:"output"`
	Error         string                 `json:"error"`
	Context       *ErrorContext          `json:"context"`
	Metadata      map[string]interface{} `json:"metadata"`
	NextActions   []RecoveryAction       `json:"next_actions"`
	Resolved      bool                   `json:"resolved"`
	ManualSteps   []string               `json:"manual_steps"`   // Manual steps required
}

// ErrorEvent represents an error event in the system
type ErrorEvent struct {
	ID            string                 `json:"id"`            // Unique event ID
	EventID       string                 `json:"event_id"`
	Timestamp     time.Time              `json:"timestamp"`
	ErrorType     string                 `json:"error_type"`
	ErrorCode     string                 `json:"error_code"`
	Message       string                 `json:"message"`
	Details       string                 `json:"details"`
	Severity      ErrorSeverity          `json:"severity"`
	Category      ErrorCategory          `json:"category"`
	Context       *ErrorContext          `json:"context"`
	PatternMatch  *ErrorPattern          `json:"pattern_match"`
	RecoveryActions []RecoveryAction     `json:"recovery_actions"`
	Resolved      bool                   `json:"resolved"`
	ResolvedAt    *time.Time             `json:"resolved_at"`
	Resolution    string                 `json:"resolution"`
	Metadata      map[string]interface{} `json:"metadata"`
	Hash          string                 `json:"hash"`          // For deduplication
	Count         int                    `json:"count"`         // How many times occurred
	FirstSeen     time.Time              `json:"first_seen"`
	LastSeen      time.Time              `json:"last_seen"`
	ReleaseName   string                 `json:"release_name"`  // Helm release name
	Namespace     string                 `json:"namespace"`     // Kubernetes namespace
}

// ErrorClassificationAdvanced represents an advanced classification of errors
type ErrorClassificationAdvanced struct {
	ID           string        `json:"id"`
	Name         string        `json:"name"`
	Description  string        `json:"description"`
	Category     ErrorCategory `json:"category"`
	Severity     ErrorSeverity `json:"severity"`
	Patterns     []ErrorPattern `json:"patterns"`
	Actions      []RecoveryAction `json:"actions"`
	Runbook      string        `json:"runbook"`      // Link to runbook
	Examples     []string      `json:"examples"`     // Example error messages
	Tags         []string      `json:"tags"`
	Created      time.Time     `json:"created"`
	Updated      time.Time     `json:"updated"`
}

// ErrorStatistics represents statistics about errors
type ErrorStatistics struct {
	Period        string                    `json:"period"`
	TotalErrors   int                       `json:"total_errors"`
	UniqueErrors  int                       `json:"unique_errors"`
	ResolvedErrors int                      `json:"resolved_errors"`
	BySeverity    map[ErrorSeverity]int     `json:"by_severity"`
	ByCategory    map[ErrorCategory]int     `json:"by_category"`
	ByComponent   map[string]int            `json:"by_component"`
	TopErrors     []ErrorSummaryDetailed    `json:"top_errors"`
	Trends        map[string][]DataPoint    `json:"trends"`
	MTTR          time.Duration             `json:"mttr"`         // Mean Time To Resolution
	MTTD          time.Duration             `json:"mttd"`         // Mean Time To Detection
}

// ErrorSummaryDetailed represents a detailed summary of an error
type ErrorSummaryDetailed struct {
	Pattern     string        `json:"pattern"`
	Message     string        `json:"message"`
	Count       int           `json:"count"`
	Severity    ErrorSeverity `json:"severity"`
	Category    ErrorCategory `json:"category"`
	FirstSeen   time.Time     `json:"first_seen"`
	LastSeen    time.Time     `json:"last_seen"`
	Resolved    int           `json:"resolved"`
	Unresolved  int           `json:"unresolved"`
}

// DataPoint represents a data point for trending
type DataPoint struct {
	Timestamp time.Time `json:"timestamp"`
	Value     float64   `json:"value"`
}

// ErrorHandler represents an error handler configuration
type ErrorHandler struct {
	ID          string           `json:"id"`
	Name        string           `json:"name"`
	Enabled     bool             `json:"enabled"`
	Patterns    []ErrorPattern   `json:"patterns"`
	Actions     []RecoveryAction `json:"actions"`
	Conditions  []string         `json:"conditions"`  // Conditions for triggering
	Cooldown    time.Duration    `json:"cooldown"`    // Cooldown between actions
	MaxRetries  int              `json:"max_retries"`
	Escalation  []string         `json:"escalation"`  // Escalation chain
	Notifications []string       `json:"notifications"` // Who to notify
}

// RecoveryPlan represents a plan for error recovery
type RecoveryPlan struct {
	PlanID      string           `json:"plan_id"`
	Name        string           `json:"name"`
	Description string           `json:"description"`
	ErrorPattern *ErrorPattern   `json:"error_pattern"`
	Steps       []RecoveryStep   `json:"steps"`
	Timeout     time.Duration    `json:"timeout"`
	MaxRetries  int              `json:"max_retries"`
	FailureMode string           `json:"failure_mode"` // What to do if plan fails
	Created     time.Time        `json:"created"`
	Updated     time.Time        `json:"updated"`
}

// RecoveryStep represents a single step in a recovery plan
type RecoveryStep struct {
	StepID      string           `json:"step_id"`
	Name        string           `json:"name"`
	Action      RecoveryAction   `json:"action"`
	Order       int              `json:"order"`
	Parallel    bool             `json:"parallel"`    // Can run in parallel
	Required    bool             `json:"required"`    // Must succeed
	OnFailure   string           `json:"on_failure"`  // What to do on failure
	OnSuccess   string           `json:"on_success"`  // What to do on success
	Conditions  []string         `json:"conditions"`  // Pre-conditions
}

// Constants for error matching and recovery
const (
	// Common error patterns
	ErrorPatternTimeout        = "timeout"
	ErrorPatternPermissionDenied = "permission_denied"
	ErrorPatternResourceExists = "resource_exists"
	ErrorPatternResourceNotFound = "resource_not_found"
	ErrorPatternNetworkError   = "network_error"
	ErrorPatternValidationFailed = "validation_failed"
	ErrorPatternDependencyFailed = "dependency_failed"

	// Recovery action results
	RecoveryStatusPending    = "pending"
	RecoveryStatusRunning    = "running"
	RecoveryStatusSucceeded  = "succeeded"
	RecoveryStatusFailed     = "failed"
	RecoveryStatusSkipped    = "skipped"
	RecoveryStatusTimedOut   = "timed_out"
)

// Additional error severity constants
const (
	ErrorSeverityUnknown = "unknown"
)

// ErrorCategoryGeneric constant 
const (
	ErrorCategoryGeneric      = "generic"
	ErrorCategoryConfiguration = "configuration"
)

// RecoveryActionType constants
const (
	RecoveryActionTypeRetry    = "retry"
	RecoveryActionTypeRollback = "rollback"
	RecoveryActionTypeForce    = "force"
)

// RecoveryPriority constants  
const (
	RecoveryPriorityLow    = "low"
	RecoveryPriorityMedium = "medium"
	RecoveryPriorityHigh   = "high"
)

// Note: ErrorType is already defined in error_classification.go
// Using additional ErrorType constants for enhanced error handling
const (
	ErrorTypeDependency ErrorType = "dependency"
)