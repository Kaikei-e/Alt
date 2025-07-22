package domain

import (
	"time"
)

// ChartStatus represents the status of a Helm chart
type ChartStatus struct {
	Name          string            `json:"name"`
	Namespace     string            `json:"namespace"`
	Version       string            `json:"version"`
	AppVersion    string            `json:"app_version"`
	Status        string            `json:"status"`        // deployed, pending, failed, etc.
	Revision      int               `json:"revision"`
	LastDeployed  time.Time         `json:"last_deployed"`
	Description   string            `json:"description"`
	Notes         string            `json:"notes"`
	Values        map[string]interface{} `json:"values"`
	Resources     []ResourceInfo    `json:"resources"`
	Dependencies  []string          `json:"dependencies"`
	Hooks         []HookInfo        `json:"hooks"`
	TestStatus    string            `json:"test_status"`
}

// ResourceInfo represents information about a Kubernetes resource
type ResourceInfo struct {
	Kind       string `json:"kind"`
	Name       string `json:"name"`
	Namespace  string `json:"namespace"`
	Status     string `json:"status"`
	Ready      string `json:"ready"`
	Age        string `json:"age"`
}

// HookInfo represents information about a Helm hook
type HookInfo struct {
	Name      string    `json:"name"`
	Kind      string    `json:"kind"`
	Hook      string    `json:"hook"`
	Weight    int       `json:"weight"`
	Status    string    `json:"status"`
	StartTime time.Time `json:"start_time"`
	EndTime   *time.Time `json:"end_time"`
}

// TemplateOptions represents options for Helm template operations
type TemplateOptions struct {
	// Basic options
	Name           string            `json:"name"`
	ReleaseName    string            `json:"release_name"`  // Alternative field for release name
	Namespace      string            `json:"namespace"`
	Values         map[string]interface{} `json:"values"`
	ValueFiles     []string          `json:"value_files"`
	StringValues   map[string]string `json:"string_values"`
	FileValues     map[string]string `json:"file_values"`
	
	// Template specific options
	ShowOnly       []string          `json:"show_only"`
	IncludeCrds    bool              `json:"include_crds"`
	SkipCrds       bool              `json:"skip_crds"`
	ValidateSchema bool              `json:"validate_schema"`
	
	// Output options
	OutputDir      string            `json:"output_dir"`
	Debug          bool              `json:"debug"`
	
	// Advanced options
	APIVersions    []string          `json:"api_versions"`
	KubeVersion    string            `json:"kube_version"`
	IsUpgrade      bool              `json:"is_upgrade"`
	PostRenderer   string            `json:"post_renderer"`
}

// TemplateResult represents the result of a Helm template operation
type TemplateResult struct {
	Success      bool              `json:"success"`
	Manifest     string            `json:"manifest"`
	Resources    []ResourceInfo    `json:"resources"`
	Notes        string            `json:"notes"`
	Values       map[string]interface{} `json:"values"`
	Duration     time.Duration     `json:"duration"`
	Errors       []string          `json:"errors"`
	Warnings     []string          `json:"warnings"`
	Files        map[string]string `json:"files"`    // filename -> content
	Hooks        []HookInfo        `json:"hooks"`
}

// ValidationResult represents the result of validation operations
type ValidationResult struct {
	Valid        bool              `json:"valid"`
	Duration     time.Duration     `json:"duration"`
	Errors       []ValidationError `json:"errors"`
	Warnings     []ValidationWarning `json:"warnings"`
	Summary      string            `json:"summary"`
	Details      map[string]interface{} `json:"details"`
}

// ValidationError represents a validation error
type ValidationError struct {
	Code        string `json:"code"`
	Type        string `json:"type"`
	Message     string `json:"message"`
	File        string `json:"file"`
	Line        int    `json:"line"`
	Column      int    `json:"column"`
	Severity    string `json:"severity"`
	Suggestion  string `json:"suggestion"`
	Details     string `json:"details"`
}

// ValidationWarning represents a validation warning
type ValidationWarning struct {
	Code        string `json:"code"`
	Type        string `json:"type"`        // Warning type
	Message     string `json:"message"`
	File        string `json:"file"`
	Line        int    `json:"line"`
	Column      int    `json:"column"`
	Severity    string `json:"severity"`    // Warning severity
	Suggestion  string `json:"suggestion"`
	Details     string `json:"details"`     // Additional warning details
}

// LintResult represents the result of Helm lint operations
type LintResult struct {
	Success      bool              `json:"success"`
	Duration     time.Duration     `json:"duration"`
	Messages     []LintMessage     `json:"messages"`
	ErrorCount   int               `json:"error_count"`
	WarningCount int               `json:"warning_count"`
	InfoCount    int               `json:"info_count"`
	Summary      string            `json:"summary"`
}

// LintMessage represents a lint message
type LintMessage struct {
	Severity string `json:"severity"`  // ERROR, WARNING, INFO
	Message  string `json:"message"`
	File     string `json:"file"`
	Line     int    `json:"line"`
}

// DryRunResult represents the result of a dry-run operation
type DryRunResult struct {
	Success      bool              `json:"success"`
	Duration     time.Duration     `json:"duration"`
	Resources    []ResourceInfo    `json:"resources"`
	Hooks        []HookInfo        `json:"hooks"`
	Notes        string            `json:"notes"`
	Manifest     string            `json:"manifest"`
	Changes      []ChangeInfo      `json:"changes"`
	Conflicts    []ConflictInfo    `json:"conflicts"`
	Errors       []string          `json:"errors"`
	Warnings     []string          `json:"warnings"`
}

// ChangeInfo represents information about changes that would be made
type ChangeInfo struct {
	Action    string `json:"action"`    // create, update, delete
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Diff      string `json:"diff"`
}

// ConflictInfo represents information about conflicts
type ConflictInfo struct {
	Resource     string `json:"resource"`
	ConflictType string `json:"conflict_type"`
	Message      string `json:"message"`
	Resolution   string `json:"resolution"`
}

// InstallOptions represents options for Helm install operations
type InstallOptions struct {
	// Basic options
	Name           string            `json:"name"`
	Namespace      string            `json:"namespace"`
	CreateNamespace bool             `json:"create_namespace"`
	Values         map[string]interface{} `json:"values"`
	ValueFiles     []string          `json:"value_files"`
	
	// Installation behavior
	Wait            bool             `json:"wait"`
	WaitForJobs     bool             `json:"wait_for_jobs"`
	Timeout         time.Duration    `json:"timeout"`
	Atomic          bool             `json:"atomic"`
	SkipCrds        bool             `json:"skip_crds"`
	ReplaceCrds     bool             `json:"replace_crds"`
	
	// Advanced options
	Force           bool             `json:"force"`
	ResetValues     bool             `json:"reset_values"`
	ReuseValues     bool             `json:"reuse_values"`
	Description     string           `json:"description"`
	DependencyUpdate bool            `json:"dependency_update"`
	DisableHooks    bool             `json:"disable_hooks"`
	
	// Output options
	DryRun          bool             `json:"dry_run"`
	Debug           bool             `json:"debug"`
}

// UpgradeOptions represents options for Helm upgrade operations
type UpgradeOptions struct {
	// Inherits all InstallOptions
	InstallOptions
	
	// Upgrade-specific options
	Install         bool             `json:"install"`         // Install if not exists
	RecreatePods    bool             `json:"recreate_pods"`
	MaxHistory      int              `json:"max_history"`
	CleanupOnFail   bool             `json:"cleanup_on_fail"`
}

// UninstallOptions represents options for Helm uninstall operations
type UninstallOptions struct {
	DryRun        bool          `json:"dry_run"`
	KeepHistory   bool          `json:"keep_history"`
	Timeout       time.Duration `json:"timeout"`
	Wait          bool          `json:"wait"`
	DisableHooks  bool          `json:"disable_hooks"`
}

// RollbackOptions represents options for Helm rollback operations
type RollbackOptions struct {
	Revision      int           `json:"revision"`
	DryRun        bool          `json:"dry_run"`
	Force         bool          `json:"force"`
	RecreatesPods bool          `json:"recreates_pods"`
	Timeout       time.Duration `json:"timeout"`
	Wait          bool          `json:"wait"`
	DisableHooks  bool          `json:"disable_hooks"`
	CleanupOnFail bool          `json:"cleanup_on_fail"`
}

// Constants for Helm chart statuses
const (
	ChartStatusDeployed      = "deployed"
	ChartStatusUninstalled   = "uninstalled"
	ChartStatusSuperseded    = "superseded"
	ChartStatusFailed        = "failed"
	ChartStatusUninstalling  = "uninstalling"
	ChartStatusPendingInstall = "pending-install"
	ChartStatusPendingUpgrade = "pending-upgrade"
	ChartStatusPendingRollback = "pending-rollback"
)

// Constants for hook phases
const (
	HookPhasePreInstall   = "pre-install"
	HookPhasePostInstall  = "post-install"
	HookPhasePreDelete    = "pre-delete"
	HookPhasePostDelete   = "post-delete"
	HookPhasePreUpgrade   = "pre-upgrade"
	HookPhasePostUpgrade  = "post-upgrade"
	HookPhasePreRollback  = "pre-rollback"
	HookPhasePostRollback = "post-rollback"
	HookPhaseTest         = "test"
)

// Constants for hook deletion policies
const (
	HookDeletePolicyBeforeHookCreation = "before-hook-creation"
	HookDeletePolicyHookSucceeded      = "hook-succeeded"
	HookDeletePolicyHookFailed         = "hook-failed"
)

// Additional missing types for Helm operations

// ReleaseStatus represents the status of a release
type ReleaseStatus string

const (
	ReleaseStatusActive    ReleaseStatus = "active"
	ReleaseStatusFailed    ReleaseStatus = "failed"
	ReleaseStatusSuperseded ReleaseStatus = "superseded"
	ReleaseStatusDeleted   ReleaseStatus = "deleted"
	ReleaseStatusPending   ReleaseStatus = "pending"
)

// TestOptions represents options for Helm test operations
type TestOptions struct {
	Timeout      time.Duration `json:"timeout"`
	Cleanup      bool          `json:"cleanup"`
	Parallel     bool          `json:"parallel"`
	MaxParallel  int           `json:"max_parallel"`
	Filter       []string      `json:"filter"`
	ExcludeFilter []string     `json:"exclude_filter"`
	Debug        bool          `json:"debug"`
	Verbose      bool          `json:"verbose"`
}

// TestResult represents the result of Helm test operations
type TestResult struct {
	Success      bool              `json:"success"`
	Duration     time.Duration     `json:"duration"`
	TestResults  []TestCaseResult  `json:"test_results"`
	Summary      string            `json:"summary"`
	Errors       []string          `json:"errors"`
	Warnings     []string          `json:"warnings"`
}

// TestCaseResult represents the result of a single test case
type TestCaseResult struct {
	Name        string        `json:"name"`
	Success     bool          `json:"success"`
	Duration    time.Duration `json:"duration"`
	Output      string        `json:"output"`
	Error       string        `json:"error"`
	Phase       string        `json:"phase"`
	StartTime   time.Time     `json:"start_time"`
	EndTime     time.Time     `json:"end_time"`
}

// ComplianceRules represents compliance validation rules
type ComplianceRules struct {
	Rules        []ComplianceRule     `json:"rules"`
	Profiles     []string             `json:"profiles"`     // security, pci-dss, sox, etc.
	Severity     string               `json:"severity"`     // minimum severity to check
	FailOnError  bool                 `json:"fail_on_error"`
	SkipRules    []string             `json:"skip_rules"`
	RequiredLabels []string           `json:"required_labels"` // Required labels for compliance
	SecurityRequirements *SecurityRequirements `json:"security_requirements"` // Security requirements
	ResourceRequirements *ResourceRequirements `json:"resource_requirements"` // Resource requirements
}

// ComplianceRule represents a single compliance rule
type ComplianceRule struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Severity    string            `json:"severity"`
	Category    string            `json:"category"`
	Pattern     string            `json:"pattern"`
	Remediation string            `json:"remediation"`
	Metadata    map[string]string `json:"metadata"`
}

// ComplianceViolation represents a compliance violation
type ComplianceViolation struct {
	RuleID      string `json:"rule_id"`
	Rule        string `json:"rule"`         // Rule name/identifier
	Severity    string `json:"severity"`
	Message     string `json:"message"`
	Resource    string `json:"resource"`
	Remediation string `json:"remediation"`
	Details     string `json:"details"`      // Additional violation details
}

// ComplianceResult represents compliance validation results
type ComplianceResult struct {
	Success      bool                    `json:"success"`
	Duration     time.Duration           `json:"duration"`
	Results      []ComplianceRuleResult  `json:"results"`
	PassedRules  int                     `json:"passed_rules"`
	FailedRules  int                     `json:"failed_rules"`
	SkippedRules int                     `json:"skipped_rules"`
	Summary      string                  `json:"summary"`
	Compliant    bool                    `json:"compliant"`    // Overall compliance status
	Violations   []ComplianceViolation   `json:"violations"`   // List of violations
	Score        int                     `json:"score"`        // Compliance score 0-100
	RulesChecked int                     `json:"rules_checked"` // Total number of rules checked
}

// ComplianceRuleResult represents the result of a single compliance rule
type ComplianceRuleResult struct {
	Rule        ComplianceRule `json:"rule"`
	Passed      bool           `json:"passed"`
	Message     string         `json:"message"`
	Details     string         `json:"details"`
	Remediation string         `json:"remediation"`
}

// SecurityValidationResult represents security validation results
type SecurityValidationResult struct {
	Success          bool                     `json:"success"`
	Duration         time.Duration            `json:"duration"`
	Vulnerabilities  []SecurityVulnerability  `json:"vulnerabilities"`
	Misconfigurations []SecurityMisconfiguration `json:"misconfigurations"`
	Score            int                      `json:"score"`  // 0-100
	Recommendations  []string                 `json:"recommendations"`
	Secure           bool                     `json:"secure"`      // Overall security status
	RiskScore        int                      `json:"risk_score"`  // Risk score 0-100
}

// SecurityVulnerability represents a security vulnerability
type SecurityVulnerability struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Type        string    `json:"type"`        // Vulnerability type
	Severity    string    `json:"severity"`
	CVSS        float64   `json:"cvss"`
	CVE         string    `json:"cve"`
	Component   string    `json:"component"`
	Version     string    `json:"version"`
	FixVersion  string    `json:"fix_version"`
	References  []string  `json:"references"`
	Impact      string    `json:"impact"`      // Impact description
	Mitigation  string    `json:"mitigation"`  // Mitigation steps
}

// SecurityMisconfiguration represents a security misconfiguration
type SecurityMisconfiguration struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Severity    string   `json:"severity"`
	Resource    string   `json:"resource"`
	Path        string   `json:"path"`
	Current     string   `json:"current"`
	Expected    string   `json:"expected"`
	References  []string `json:"references"`
}

// DependencyIssue represents a dependency issue
type DependencyIssue struct {
	Type        string `json:"type"`
	Severity    string `json:"severity"`    // Issue severity
	Dependency  string `json:"dependency"`
	Version     string `json:"version"`
	Message     string `json:"message"`
	Resolution  string `json:"resolution"`
	Details     string `json:"details"`     // Additional issue details
	Remediation string `json:"remediation"` // Remediation steps
}

// DependencyValidationResult represents dependency validation results
type DependencyValidationResult struct {
	Success        bool                      `json:"success"`
	Duration       time.Duration             `json:"duration"`
	Dependencies   []*DependencyInfo         `json:"dependencies"` // Changed to match what gateway expects
	Conflicts      []DependencyConflict      `json:"conflicts"`
	MissingDeps    []string                  `json:"missing_dependencies"`
	Recommendations []string                 `json:"recommendations"`
	Valid          bool                      `json:"valid"`       // Overall validation status
	Issues         []DependencyIssue         `json:"issues"`      // List of dependency issues
}

// DependencyCheck represents a dependency validation check
type DependencyCheck struct {
	Name          string `json:"name"`
	Version       string `json:"version"`
	Required      string `json:"required"`
	Available     bool   `json:"available"`
	Compatible    bool   `json:"compatible"`
	Message       string `json:"message"`
}

// DependencyConflict represents a dependency conflict
type DependencyConflict struct {
	Dependency1   string `json:"dependency1"`
	Version1      string `json:"version1"`
	Dependency2   string `json:"dependency2"`
	Version2      string `json:"version2"`
	ConflictType  string `json:"conflict_type"`
	Resolution    string `json:"resolution"`
}

// HelmDeploymentRequest represents a Helm deployment request
type HelmDeploymentRequest struct {
	ChartName       string                     `json:"chart_name"`
	ChartPath       string                     `json:"chart_path"`
	ReleaseName     string                     `json:"release_name"`
	Namespace       string                     `json:"namespace"`
	Values          map[string]interface{}     `json:"values"`
	Wait            bool                       `json:"wait"`
	Timeout         time.Duration              `json:"timeout"`
	CreateNamespace bool                       `json:"create_namespace"`
	DryRun          bool                       `json:"dry_run"`
	Force           bool                       `json:"force"`
	DisableHooks    bool                       `json:"disable_hooks"`
	SkipCRDs        bool                       `json:"skip_crds"`
	Chart           HelmChart                  `json:"chart"`
	Options         *InstallOptions            `json:"options"`
	Context         map[string]string          `json:"context"`
}

// HelmUndeploymentRequest represents a Helm undeployment request
type HelmUndeploymentRequest struct {
	ReleaseName     string                     `json:"release_name"`
	Namespace       string                     `json:"namespace"`
	KeepHistory     bool                       `json:"keep_history"`
	DryRun          bool                       `json:"dry_run"`
	Wait            bool                       `json:"wait"`
	Timeout         time.Duration              `json:"timeout"`
	DisableHooks    bool                       `json:"disable_hooks"`
	UndeployTimeout time.Duration              `json:"undeploy_timeout"`
	Options         *UninstallOptions          `json:"options"`
	Context         map[string]string          `json:"context"`
}

// Constants for lint severity levels
const (
	LintSeverityError   = "ERROR"
	LintSeverityWarning = "WARNING"
	LintSeverityInfo    = "INFO"
)

// Constants for validation error types
const (
	ValidationErrorTypeMetadata      = "metadata"
	ValidationErrorTypeSchema        = "schema"
	ValidationErrorTypeConfiguration = "configuration"
	ValidationErrorTypeSecurity      = "security"
	ValidationErrorTypeResource      = "resource"
	ValidationErrorTypeVersion       = "version"
	ValidationErrorTypeDependency    = "dependency"
	ValidationErrorTypeMissingField  = "missing_field"
)

// Constants for validation severities
const (
	ValidationSeverityError   = "error"
	ValidationSeverityWarning = "warning"
	ValidationSeverityInfo    = "info"
)

// Constants for validation warning types
const (
	ValidationWarningTypeNaming        = "naming"
	ValidationWarningTypeStructure     = "structure"
	ValidationWarningTypeBestPractices = "best_practices"
	ValidationWarningTypeValues        = "values"
	ValidationWarningTypeMissingTemplates = "missing_templates"
	ValidationWarningTypeDeprecated    = "deprecated"
)

// Constants for compliance severity levels
const (
	ComplianceSeverityLow      = "low"
	ComplianceSeverityMedium   = "medium"
	ComplianceSeverityHigh     = "high"
	ComplianceSeverityCritical = "critical"
)

// Constants for security severity levels
const (
	SecuritySeverityLow      = "low"
	SecuritySeverityMedium   = "medium"
	SecuritySeverityHigh     = "high"
	SecuritySeverityCritical = "critical"
)

// Constants for security vulnerability types
const (
	SecurityVulnerabilityTypePrivileged     = "privileged"
	SecurityVulnerabilityTypeExposed        = "exposed"
	SecurityVulnerabilityTypeInsecure       = "insecure"
	SecurityVulnerabilityTypeRootUser       = "root_user"
	SecurityVulnerabilityTypeExposedSecrets = "exposed_secrets"
)

// Constants for dependency issue types
const (
	DependencyIssueTypeResolution     = "resolution"
	DependencyIssueTypeConflict       = "conflict"
	DependencyIssueTypeMissing        = "missing"
	DependencyIssueTypeVersion        = "version"
	DependencyIssueTypeValidation     = "validation"
	DependencyIssueTypeVersionConflict = "version_conflict"
)

// Constants for dependency issue severity
const (
	DependencyIssueSeverityLow      = "low"
	DependencyIssueSeverityMedium   = "medium"
	DependencyIssueSeverityHigh     = "high"
	DependencyIssueSeverityError    = "error"
	DependencyIssueSeverityCritical = "critical"
	DependencyIssueSeverityWarning  = "warning"
)

// HelmUpgradeRequest represents a Helm upgrade request
type HelmUpgradeRequest struct {
	ChartName       string                     `json:"chart_name"`
	ChartPath       string                     `json:"chart_path"`
	ReleaseName     string                     `json:"release_name"`
	Namespace       string                     `json:"namespace"`
	Values          map[string]interface{}     `json:"values"`
	Wait            bool                       `json:"wait"`
	Timeout         time.Duration              `json:"timeout"`
	CreateNamespace bool                       `json:"create_namespace"`
	DryRun          bool                       `json:"dry_run"`
	Force           bool                       `json:"force"`
	DisableHooks    bool                       `json:"disable_hooks"`
	SkipCRDs        bool                       `json:"skip_crds"`
	ResetValues     bool                       `json:"reset_values"`
	ReuseValues     bool                       `json:"reuse_values"`
	Install         bool                       `json:"install"`         // Install if not exists
	Chart           HelmChart                  `json:"chart"`
	Options         *UpgradeOptions            `json:"options"`
	Context         map[string]string          `json:"context"`
}

// HelmRollbackRequest represents a Helm rollback request
type HelmRollbackRequest struct {
	ReleaseName       string                     `json:"release_name"`
	Namespace         string                     `json:"namespace"`
	Revision          int                        `json:"revision"`
	DryRun            bool                       `json:"dry_run"`
	Force             bool                       `json:"force"`
	DisableHooks      bool                       `json:"disable_hooks"`
	Wait              bool                       `json:"wait"`
	Timeout           time.Duration              `json:"timeout"`
	RecreateResources bool                       `json:"recreate_resources"`
	Options           *RollbackOptions           `json:"options"`
	Context           map[string]string          `json:"context"`
}

// HelmMetadataRequest represents a Helm metadata request
type HelmMetadataRequest struct {
	ChartName string                     `json:"chart_name"`
	ChartPath string                     `json:"chart_path"`
	Context   map[string]string          `json:"context"`
}

// HelmChart represents a Helm chart for API requests (different from domain.Chart)
type HelmChart struct {
	Name        string            `json:"name"`
	Version     string            `json:"version"`
	Path        string            `json:"path"`
	Repository  string            `json:"repository"`
	Metadata    *ChartMetadata    `json:"metadata"`
	Values      map[string]interface{} `json:"values"`
}

// SecurityRequirements represents security requirements for validation
type SecurityRequirements struct {
	EnforceSecurityPolicies bool     `json:"enforce_security_policies"`
	RequiredSecurityContext bool     `json:"required_security_context"`
	ProhibitedCapabilities  []string `json:"prohibited_capabilities"`
	RequiredLabels          []string `json:"required_labels"`
	RequiredAnnotations     []string `json:"required_annotations"`
	AllowPrivileged         bool     `json:"allow_privileged"`
	RequireNonRootUser      bool     `json:"require_non_root_user"`
	RequireNonRoot          bool     `json:"require_non_root"`          // Alternative field name
	RequireReadOnlyRootFilesystem bool `json:"require_readonly_root_filesystem"` // Additional security requirement
}

// ResourceRequirements represents resource requirements for validation
type ResourceRequirements struct {
	RequireResourceLimits   bool   `json:"require_resource_limits"`
	RequireResourceRequests bool   `json:"require_resource_requests"`
	MaxCPU                  string `json:"max_cpu"`
	MaxMemory               string `json:"max_memory"`
	MinCPU                  string `json:"min_cpu"`
	MinMemory               string `json:"min_memory"`
}

// DependencyInfo represents dependency information
type DependencyInfo struct {
	Name         string            `json:"name"`
	Version      string            `json:"version"`
	Repository   string            `json:"repository"`
	Alias        string            `json:"alias"`
	Condition    string            `json:"condition"`
	Tags         []string          `json:"tags"`
	ImportValues []interface{}     `json:"import_values"`
	Enabled      bool              `json:"enabled"`
	Optional     bool              `json:"optional"`
	Status       string            `json:"status"`
	Metadata     map[string]string `json:"metadata"`
}

// HelmMetadataUpdateRequest represents a Helm metadata update request
type HelmMetadataUpdateRequest struct {
	ChartName string                     `json:"chart_name"`
	ChartPath string                     `json:"chart_path"`
	Fields    map[string]interface{}     `json:"fields"`
	Context   map[string]string          `json:"context"`
}

// HelmDependencyRequest represents a Helm dependency request
type HelmDependencyRequest struct {
	ChartName string                     `json:"chart_name"`
	ChartPath string                     `json:"chart_path"`
	Context   map[string]string          `json:"context"`
}

// HelmDependencyUpdateRequest represents a Helm dependency update request
type HelmDependencyUpdateRequest struct {
	ChartName    string                     `json:"chart_name"`
	ChartPath    string                     `json:"chart_path"`
	Dependencies []*ChartDependency         `json:"dependencies"`
	Context      map[string]string          `json:"context"`
}

// HelmValuesRequest represents a Helm values request
type HelmValuesRequest struct {
	ChartName string                     `json:"chart_name"`
	ChartPath string                     `json:"chart_path"`
	Context   map[string]string          `json:"context"`
}

// HelmTemplateRequest represents a Helm template request
type HelmTemplateRequest struct {
	ChartName       string                     `json:"chart_name"`
	ChartPath       string                     `json:"chart_path"`
	ReleaseName     string                     `json:"release_name"`
	Namespace       string                     `json:"namespace"`
	Values          map[string]interface{}     `json:"values"`
	Options         *TemplateOptions           `json:"options"`
	Context         map[string]string          `json:"context"`
}

// HelmListRequest represents a Helm list request
type HelmListRequest struct {
	Namespace       string                     `json:"namespace"`
	AllNamespaces   bool                       `json:"all_namespaces"`
	Options         *ReleaseListOptions        `json:"options"`
	Context         map[string]string          `json:"context"`
}

// HelmHistoryRequest represents a Helm history request
type HelmHistoryRequest struct {
	ReleaseName     string                     `json:"release_name"`
	Namespace       string                     `json:"namespace"`
	MaxRevisions    int                        `json:"max_revisions"`
	Context         map[string]string          `json:"context"`
}