// PHASE R3: Deployment command flags management
package deployment

import (
	"time"

	"github.com/spf13/cobra"

	"deploy-cli/domain"
)

// DeployFlags manages deployment command flags
type DeployFlags struct {
	// Core deployment flags
	DryRun         bool
	Restart        bool
	ForceUpdate    bool
	Namespace      string
	Timeout        time.Duration
	ChartsDir      string

	// Automatic recovery flags
	AutoFixSecrets      bool
	AutoCreateNamespaces bool
	AutoFixStorage     bool
	AutoEverything     bool

	// Monitoring flags
	ContinuousMonitoring  bool
	MonitoringInterval    time.Duration
	DiagnosticReport     bool

	// Emergency and recovery flags
	SkipStatefulSetRecovery bool
	EmergencyMode          bool
	SkipHealthChecks       bool

	// Helm lock management flags
	ForceUnlock        bool
	LockWaitTimeout    time.Duration
	MaxLockRetries     int

	// Cleanup flags
	SkipCleanup         bool
	CleanupThreshold    time.Duration
	ConservativeCleanup bool
}

// NewDeployFlags creates a new deployment flags instance
func NewDeployFlags() *DeployFlags {
	return &DeployFlags{
		// Default values
		Timeout:             300 * time.Second,
		ChartsDir:          "../charts",
		MonitoringInterval: 30 * time.Second,
		LockWaitTimeout:    5 * time.Minute,
		MaxLockRetries:     5,
		CleanupThreshold:   15 * time.Minute,
		ConservativeCleanup: true,
	}
}

// AddToCommand adds all deployment flags to the given command
func (f *DeployFlags) AddToCommand(cmd *cobra.Command) {
	// Core deployment flags
	cmd.Flags().BoolP("dry-run", "d", f.DryRun, 
		"Perform dry-run (template charts without deploying)")
	cmd.Flags().BoolP("restart", "r", f.Restart, 
		"Restart deployments after deployment")
	cmd.Flags().BoolP("force-update", "f", f.ForceUpdate, 
		"Force pod updates even when manifests are identical")
	cmd.Flags().StringP("namespace", "n", f.Namespace, 
		"Override target namespace")
	cmd.Flags().Duration("timeout", f.Timeout, 
		"Timeout for deployment operations")
	cmd.Flags().String("charts-dir", f.ChartsDir, 
		"Directory containing Helm charts")

	// Automatic recovery flags
	cmd.Flags().Bool("auto-fix-secrets", f.AutoFixSecrets, 
		"Enable automatic secret error recovery")
	cmd.Flags().Bool("auto-create-namespaces", f.AutoCreateNamespaces, 
		"Enable automatic namespace creation if not exists")
	cmd.Flags().Bool("auto-fix-storage", f.AutoFixStorage, 
		"Enable automatic StorageClass configuration")
	cmd.Flags().Bool("auto-everything", f.AutoEverything, 
		"Enable all automatic recovery features")

	// Monitoring flags
	cmd.Flags().Bool("continuous-monitoring", f.ContinuousMonitoring, 
		"Enable continuous monitoring after deployment")
	cmd.Flags().Duration("monitoring-interval", f.MonitoringInterval, 
		"Monitoring interval for continuous monitoring")
	cmd.Flags().Bool("diagnostic-report", f.DiagnosticReport, 
		"Generate detailed diagnostic report before deployment")

	// Emergency and recovery flags
	cmd.Flags().Bool("skip-statefulset-recovery", f.SkipStatefulSetRecovery, 
		"Skip StatefulSet recovery for emergency deployments")
	cmd.Flags().Bool("emergency-mode", f.EmergencyMode, 
		"Emergency deployment mode: aggressive timeouts, skip non-critical checks")
	cmd.Flags().Bool("skip-health-checks", f.SkipHealthChecks, 
		"Skip all health checks for emergency deployment")

	// Helm lock management flags
	cmd.Flags().Bool("force-unlock", f.ForceUnlock, 
		"Force cleanup of Helm lock conflicts before deployment")
	cmd.Flags().Duration("lock-wait-timeout", f.LockWaitTimeout, 
		"Maximum time to wait for Helm lock release")
	cmd.Flags().Int("max-lock-retries", f.MaxLockRetries, 
		"Maximum number of lock cleanup retry attempts")

	// Cleanup flags
	cmd.Flags().Bool("skip-cleanup", f.SkipCleanup, 
		"Skip automatic Helm operation cleanup")
	cmd.Flags().Duration("cleanup-threshold", f.CleanupThreshold, 
		"Minimum age for cleanup operations")
	cmd.Flags().Bool("conservative-cleanup", f.ConservativeCleanup, 
		"Use conservative cleanup approach")
}

// ParseFromCommand parses flags from command and applies them to deployment options
func (f *DeployFlags) ParseFromCommand(cmd *cobra.Command, options *domain.DeploymentOptions) error {
	var err error

	// Core deployment flags
	if options.DryRun, err = cmd.Flags().GetBool("dry-run"); err != nil {
		return err
	}
	if options.DoRestart, err = cmd.Flags().GetBool("restart"); err != nil {
		return err
	}
	if options.ForceUpdate, err = cmd.Flags().GetBool("force-update"); err != nil {
		return err
	}
	if options.TargetNamespace, err = cmd.Flags().GetString("namespace"); err != nil {
		return err
	}
	if options.Timeout, err = cmd.Flags().GetDuration("timeout"); err != nil {
		return err
	}
	if options.ChartsDir, err = cmd.Flags().GetString("charts-dir"); err != nil {
		return err
	}

	// Automatic recovery flags
	if options.AutoFixSecrets, err = cmd.Flags().GetBool("auto-fix-secrets"); err != nil {
		return err
	}
	if options.AutoCreateNamespaces, err = cmd.Flags().GetBool("auto-create-namespaces"); err != nil {
		return err
	}
	if options.AutoFixStorage, err = cmd.Flags().GetBool("auto-fix-storage"); err != nil {
		return err
	}

	// Process auto-everything flag
	if autoEverything, err := cmd.Flags().GetBool("auto-everything"); err != nil {
		return err
	} else if autoEverything {
		options.AutoFixSecrets = true
		options.AutoCreateNamespaces = true
		options.AutoFixStorage = true
	}

	// Emergency and recovery flags
	if options.SkipStatefulSetRecovery, err = cmd.Flags().GetBool("skip-statefulset-recovery"); err != nil {
		return err
	}
	if options.SkipHealthChecks, err = cmd.Flags().GetBool("skip-health-checks"); err != nil {
		return err
	}

	// Helm lock management flags
	if options.ForceUnlock, err = cmd.Flags().GetBool("force-unlock"); err != nil {
		return err
	}
	if options.LockWaitTimeout, err = cmd.Flags().GetDuration("lock-wait-timeout"); err != nil {
		return err
	}
	if options.MaxLockRetries, err = cmd.Flags().GetInt("max-lock-retries"); err != nil {
		return err
	}

	// Cleanup flags
	if options.SkipCleanup, err = cmd.Flags().GetBool("skip-cleanup"); err != nil {
		return err
	}
	if options.CleanupThreshold, err = cmd.Flags().GetDuration("cleanup-threshold"); err != nil {
		return err
	}
	if options.ConservativeCleanup, err = cmd.Flags().GetBool("conservative-cleanup"); err != nil {
		return err
	}

	return nil
}

// GetFlagDescriptions returns a map of flag names to their descriptions for help text
func (f *DeployFlags) GetFlagDescriptions() map[string]string {
	return map[string]string{
		"dry-run":                   "Preview deployment without applying changes",
		"restart":                   "Restart services after successful deployment",
		"force-update":              "Force pod recreation even when manifests are unchanged",
		"namespace":                 "Override default namespace for deployment",
		"timeout":                   "Maximum time to wait for deployment completion",
		"charts-dir":                "Path to directory containing Helm charts",
		"auto-fix-secrets":          "Automatically resolve secret conflicts and metadata issues",
		"auto-create-namespaces":    "Automatically create missing namespaces",
		"auto-fix-storage":          "Automatically resolve StorageClass and PVC issues",
		"auto-everything":           "Enable all automatic recovery features",
		"continuous-monitoring":     "Monitor deployment status continuously after completion",
		"monitoring-interval":       "Interval for continuous monitoring checks",
		"diagnostic-report":         "Generate detailed pre-deployment diagnostic report",
		"skip-statefulset-recovery": "Skip StatefulSet recovery operations for faster deployment",
		"emergency-mode":            "Enable emergency deployment mode with aggressive timeouts",
		"skip-health-checks":        "Skip post-deployment health verification",
		"force-unlock":              "Force cleanup of conflicting Helm locks",
		"lock-wait-timeout":         "Maximum time to wait for Helm lock resolution",
		"max-lock-retries":          "Maximum attempts to resolve Helm lock conflicts",
		"skip-cleanup":              "Skip automatic cleanup of failed Helm operations",
		"cleanup-threshold":         "Minimum age threshold for cleanup operations",
		"conservative-cleanup":      "Use conservative approach for cleanup operations",
	}
}