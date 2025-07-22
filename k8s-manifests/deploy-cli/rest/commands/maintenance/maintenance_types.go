// PHASE R3: Maintenance types and shared data structures
package maintenance

import (
	"time"

	"deploy-cli/domain"
)

// CleanupResult represents the result of cleanup operations
type CleanupResult struct {
	Environment domain.Environment
	StartTime   time.Time
	EndTime     time.Time
	Duration    time.Duration
	Success     bool
	DryRun      bool
	Operations  []CleanupOperation
}

// CleanupOperation represents a single cleanup operation result
type CleanupOperation struct {
	Type           string
	StartTime      time.Time
	EndTime        time.Time
	Duration       time.Duration
	Success        bool
	ItemsFound     int
	ItemsCleaned   int
	Message        string
	Error          string
}

// TroubleshootResult represents the result of troubleshooting operations
type TroubleshootResult struct {
	Environment domain.Environment
	StartTime   time.Time
	EndTime     time.Time
	Duration    time.Duration
	Issues      []TroubleshootIssue
	Fixes       []TroubleshootFix
}

// TroubleshootIssue represents a detected troubleshooting issue
type TroubleshootIssue struct {
	ID          string
	Category    string
	Component   string
	Severity    string
	Title       string
	Description string
	Impact      string
	Suggestions []string
	DetectedAt  time.Time
}

// TroubleshootFix represents an applied troubleshooting fix
type TroubleshootFix struct {
	IssueID     string
	Action      string
	Description string
	Success     bool
	Error       string
	AppliedAt   time.Time
	Duration    time.Duration
}

// EmergencyResult represents the result of emergency operations
type EmergencyResult struct {
	Environment domain.Environment
	Operation   string
	StartTime   time.Time
	EndTime     time.Time
	Duration    time.Duration
	Success     bool
	Operations  []EmergencyOperation
}

// EmergencyOperation represents a single emergency operation step
type EmergencyOperation struct {
	Step      string
	StartTime time.Time
	EndTime   time.Time
	Duration  time.Duration
	Success   bool
	Error     string
}

// DiagnoseResult represents the result of diagnostic operations
type DiagnoseResult struct {
	Environment domain.Environment
	Level       string
	StartTime   time.Time
	EndTime     time.Time
	Duration    time.Duration
	Checks      []DiagnosticCheck
	Issues      []DiagnosticIssue
	Fixes       []DiagnosticFix
}

// DiagnosticCheck represents a diagnostic check result
type DiagnosticCheck struct {
	ID          string
	Area        string
	Name        string
	Description string
	Status      string
	Score       int
	Details     map[string]interface{}
	StartTime   time.Time
	EndTime     time.Time
	Duration    time.Duration
}

// DiagnosticIssue represents a detected diagnostic issue
type DiagnosticIssue struct {
	ID             string
	CheckID        string
	Area           string
	Severity       string
	Title          string
	Description    string
	Impact         string
	Recommendation string
	Priority       int
	DetectedAt     time.Time
}

// DiagnosticFix represents an applied diagnostic fix
type DiagnosticFix struct {
	IssueID     string
	Action      string
	Description string
	Success     bool
	Error       string
	AppliedAt   time.Time
	Duration    time.Duration
}

// RepairResult represents the result of repair operations
type RepairResult struct {
	Environment domain.Environment
	StartTime   time.Time
	EndTime     time.Time
	Duration    time.Duration
	Success     bool
	Repairs     []RepairOperation
}

// RepairOperation represents a single repair operation
type RepairOperation struct {
	Type           string
	StartTime      time.Time
	EndTime        time.Time
	Duration       time.Duration
	Success        bool
	Error          string
	ItemsFound     int
	ItemsRepaired  int
	Message        string
}

// Shared option types for maintenance operations

// TroubleshootOptions represents troubleshoot configuration
type TroubleshootOptions struct {
	// Troubleshoot-specific options
	Interactive bool
	Component   string
	Categories  []string
	OutputFile  string
	ExportLogs  bool
	LogWindow   time.Duration
	MaxIssues   int

	// Global maintenance options
	AutoFix bool
	DryRun  bool
	Verbose bool
	Timeout time.Duration
}

// EmergencyOptions represents emergency configuration
type EmergencyOptions struct {
	// Emergency-specific options
	Operation    string
	Confirm      bool
	SafeMode     bool
	Component    string
	BackupBefore string
	NotifyOnCall bool
	IncidentID   string

	// Global maintenance options
	Force   bool
	Verbose bool
	Timeout time.Duration
}

// DiagnoseOptions represents diagnose configuration
type DiagnoseOptions struct {
	// Diagnose-specific options
	Level              string
	Areas              []string
	Format             string
	OutputFile         string
	RawData            bool
	Comprehensive      bool
	TrendWindow        time.Duration
	MaxRecommendations int
	IncludeLogs        bool
	ExcludeAreas       []string

	// Global maintenance options
	AutoFix bool
	DryRun  bool
	Verbose bool
	Timeout time.Duration
}

// RepairOptions represents repair configuration
type RepairOptions struct {
	// Repair-specific options
	Types             []string
	Interactive       bool
	Aggressive        bool
	ValidationTimeout time.Duration
	MaxRetries        int
	RetryDelay        time.Duration
	Parallel          bool
	MaxParallel       int
	ExcludeTypes      []string
	SeverityThreshold string

	// Global maintenance options
	AutoFix bool
	DryRun  bool
	Force   bool
	Verbose bool
	Timeout time.Duration
}

// Severity levels for issues and operations
const (
	SeverityLow      = "low"
	SeverityMedium   = "medium"
	SeverityHigh     = "high"
	SeverityCritical = "critical"
)

// Operation statuses
const (
	StatusPending   = "pending"
	StatusRunning   = "running"
	StatusCompleted = "completed"
	StatusFailed    = "failed"
	StatusSkipped   = "skipped"
)

// Diagnostic check statuses
const (
	CheckStatusPass = "pass"
	CheckStatusFail = "fail"
	CheckStatusWarn = "warn"
	CheckStatusInfo = "info"
)

// Issue categories for troubleshooting and diagnostics
const (
	CategoryConnectivity  = "connectivity"
	CategoryResources     = "resources"
	CategoryConfiguration = "configuration"
	CategoryPerformance   = "performance"
	CategorySecurity      = "security"
	CategoryCompliance    = "compliance"
	CategoryStorage       = "storage"
	CategoryNetwork       = "network"
)

// Repair types
const (
	RepairTypePods         = "pods"
	RepairTypeServices     = "services"
	RepairTypeStorage      = "storage"
	RepairTypeStatefulSets = "statefulsets"
	RepairTypeHelm         = "helm"
	RepairTypeNetwork      = "network"
	RepairTypeConfig       = "config"
)

// Emergency operation types
const (
	EmergencyReset     = "reset"
	EmergencyRollback  = "rollback"
	EmergencyIsolate   = "isolate"
	EmergencyRestore   = "restore"
	EmergencyDrain     = "drain"
	EmergencyScaleZero = "scale-zero"
)

// Diagnostic levels
const (
	DiagnosticLevelMinimal       = "minimal"
	DiagnosticLevelBasic         = "basic"
	DiagnosticLevelStandard      = "standard"
	DiagnosticLevelComprehensive = "comprehensive"
)

// Output formats for diagnostics
const (
	OutputFormatConsole = "console"
	OutputFormatJSON    = "json"
	OutputFormatHTML    = "html"
	OutputFormatCSV     = "csv"
	OutputFormatXML     = "xml"
)