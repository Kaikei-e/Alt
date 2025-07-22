package helm_port

import (
	"context"
	"time"
	
	"deploy-cli/domain"
)

// HelmPort defines the interface for Helm operations
type HelmPort interface {
	// Template renders chart templates locally
	Template(ctx context.Context, releaseName, chartPath string, options HelmTemplateOptions) (string, error)

	// Lint validates chart templates and values
	Lint(ctx context.Context, chartPath string, options HelmLintOptions) (*HelmLintResult, error)

	// UpgradeInstall installs or upgrades a Helm release
	UpgradeInstall(ctx context.Context, releaseName, chartPath string, options HelmUpgradeOptions) error

	// Status returns the status of a Helm release
	Status(ctx context.Context, releaseName, namespace string) (HelmStatus, error)

	// List returns list of Helm releases
	List(ctx context.Context, namespace string) ([]HelmRelease, error)

	// Uninstall removes a Helm release
	Uninstall(ctx context.Context, releaseName, namespace string) error

	// Rollback rolls back a Helm release to a specific revision
	Rollback(ctx context.Context, releaseName, namespace string, revision int) error

	// History returns the history of a Helm release
	History(ctx context.Context, releaseName, namespace string) ([]HelmRevision, error)

	// DetectPendingOperation checks for pending Helm operations
	DetectPendingOperation(ctx context.Context, releaseName, namespace string) (*HelmOperation, error)

	// CleanupStuckOperations cleans up stuck Helm operations
	CleanupStuckOperations(ctx context.Context, releaseName, namespace string) error

	// CleanupStuckOperationsWithThreshold cleans up stuck Helm operations with age threshold
	CleanupStuckOperationsWithThreshold(ctx context.Context, releaseName, namespace string, minAge time.Duration) error

	// RetryWithBackoff retries an operation with exponential backoff
	RetryWithBackoff(ctx context.Context, operation func() error, maxRetries int, baseDelay time.Duration) error

	// InstallChart installs a chart with the given request
	InstallChart(ctx context.Context, request *domain.HelmDeploymentRequest) error

	// UninstallChart uninstalls a chart with the given request  
	UninstallChart(ctx context.Context, request *domain.HelmUndeploymentRequest) error

	// UpgradeChart upgrades a chart with the given request
	UpgradeChart(ctx context.Context, request *domain.HelmUpgradeRequest) error

	// GetChartMetadata gets chart metadata
	GetChartMetadata(ctx context.Context, request *domain.HelmMetadataRequest) (*domain.ChartMetadata, error)

	// UpdateChartMetadata updates chart metadata
	UpdateChartMetadata(ctx context.Context, request *domain.HelmMetadataUpdateRequest) error

	// GetChartDependencies gets chart dependencies
	GetChartDependencies(ctx context.Context, request *domain.HelmDependencyRequest) ([]*domain.DependencyInfo, error)

	// UpdateChartDependencies updates chart dependencies
	UpdateChartDependencies(ctx context.Context, request *domain.HelmDependencyUpdateRequest) error

	// RollbackChart rolls back a chart with the given request
	RollbackChart(ctx context.Context, request *domain.HelmRollbackRequest) error

	// GetReleaseStatus gets release status
	GetReleaseStatus(ctx context.Context, releaseName, namespace string) (*domain.ReleaseInfo, error)

	// GetChartValues gets chart values
	GetChartValues(ctx context.Context, request *domain.HelmValuesRequest) (map[string]interface{}, error)

	// ListReleases lists releases with the given options
	ListReleases(ctx context.Context, options *domain.ReleaseListOptions) ([]*domain.ReleaseInfo, error)

	// GetReleaseHistory gets release history
	GetReleaseHistory(ctx context.Context, request *domain.HelmHistoryRequest) ([]*domain.ReleaseRevision, error)

	// GetPVCStatus gets PVC status for charts  
	GetPVCStatus(ctx context.Context, chartName, namespace string) (*PVCStatus, error)

	// GetReleasedPVs gets released persistent volumes
	GetReleasedPVs(ctx context.Context, namespace string) ([]*PersistentVolume, error)

	// ClearPVClaimRef clears PV claim reference
	ClearPVClaimRef(ctx context.Context, pvName string) error
}

// HelmTemplateOptions holds options for helm template command
type HelmTemplateOptions struct {
	ValuesFile     string
	Namespace      string
	ImageOverrides map[string]string
	SetValues      map[string]string
}

// HelmLintOptions holds options for helm lint command
type HelmLintOptions struct {
	ValuesFile string
	Strict     bool // Enable strict mode for more rigorous validation
	Namespace  string
}

// HelmLintResult holds the result of helm lint operation
type HelmLintResult struct {
	Success  bool
	Warnings []HelmLintMessage
	Errors   []HelmLintMessage
	Output   string
}

// HelmLintMessage represents a single lint warning or error
type HelmLintMessage struct {
	Severity string // "WARNING", "ERROR"
	Path     string // File path where issue was found
	Message  string // Description of the issue
}

// HelmUpgradeOptions holds options for helm upgrade command
type HelmUpgradeOptions struct {
	ValuesFile      string
	Namespace       string
	CreateNamespace bool
	Wait            bool
	WaitForJobs     bool
	Timeout         time.Duration
	Force           bool
	Atomic          bool
	ImageOverrides  map[string]string
	SetValues       map[string]string
}

// HelmStatus represents the status of a Helm release
type HelmStatus struct {
	Name      string
	Namespace string
	Status    string
	Revision  int
	Updated   time.Time
}

// HelmRelease represents a Helm release
type HelmRelease struct {
	Name       string
	Namespace  string
	Revision   int
	Status     string
	Chart      string
	AppVersion string
	Updated    time.Time
}

// HelmRevision represents a Helm release revision
type HelmRevision struct {
	Revision    int
	Status      string
	Chart       string
	AppVersion  string
	Updated     time.Time
	Description string
}

// HelmOperation represents a pending or running Helm operation
type HelmOperation struct {
	Type        string // "install", "upgrade", "rollback", "uninstall"
	ReleaseName string
	Namespace   string
	Status      string // "pending", "running", "stuck"
	StartTime   time.Time
	PID         int // Process ID if available
}

// PVCStatus represents the status of a Persistent Volume Claim
type PVCStatus struct {
	Phase      string   `json:"phase"`
	Conditions []string `json:"conditions"`
}

// PersistentVolume represents a Kubernetes Persistent Volume
type PersistentVolume struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}
