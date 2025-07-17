package helm_driver

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
	"os"
	"syscall"
	"math"
	"math/rand"
	"log"
	
	"deploy-cli/port/helm_port"
)

// HelmDriver implements Helm operations using helm CLI
type HelmDriver struct{}

// Ensure HelmDriver implements HelmPort interface
var _ helm_port.HelmPort = (*HelmDriver)(nil)

// NewHelmDriver creates a new Helm driver
func NewHelmDriver() *HelmDriver {
	return &HelmDriver{}
}

// Template renders chart templates locally
func (h *HelmDriver) Template(ctx context.Context, releaseName, chartPath string, options helm_port.HelmTemplateOptions) (string, error) {
	args := []string{"template", releaseName, chartPath}
	
	if options.ValuesFile != "" {
		args = append(args, "-f", options.ValuesFile)
	}
	
	if options.Namespace != "" {
		args = append(args, "--namespace", options.Namespace)
	}
	
	// Add image overrides
	for key, value := range options.ImageOverrides {
		args = append(args, "--set", fmt.Sprintf("%s=%s", key, value))
	}
	
	// Add set values
	for key, value := range options.SetValues {
		args = append(args, "--set", fmt.Sprintf("%s=%s", key, value))
	}
	
	cmd := exec.CommandContext(ctx, "helm", args...)
	output, err := cmd.CombinedOutput()
	return string(output), err
}

// UpgradeInstall installs or upgrades a Helm release
func (h *HelmDriver) UpgradeInstall(ctx context.Context, releaseName, chartPath string, options helm_port.HelmUpgradeOptions) error {
	// First check for and cleanup any stuck operations
	if err := h.CleanupStuckOperations(ctx, releaseName, options.Namespace); err != nil {
		return fmt.Errorf("failed to cleanup stuck operations: %w", err)
	}
	
	// Create the operation function
	upgradeOperation := func() error {
		args := []string{"upgrade", "--install", releaseName, chartPath}
		
		if options.ValuesFile != "" {
			args = append(args, "-f", options.ValuesFile)
		}
		
		if options.Namespace != "" {
			args = append(args, "--namespace", options.Namespace)
		}
		
		if options.CreateNamespace {
			args = append(args, "--create-namespace")
		}
		
		if options.Wait {
			args = append(args, "--wait")
			if options.Timeout > 0 {
				args = append(args, "--timeout", options.Timeout.String())
			}
		}
		
		if !options.WaitForJobs {
			args = append(args, "--wait-for-jobs=false")
		}
		
		if options.Atomic {
			args = append(args, "--atomic")
		}
		
		// Always add timeout to prevent hanging
		if options.Timeout > 0 {
			args = append(args, "--timeout", options.Timeout.String())
		} else {
			args = append(args, "--timeout", "3m")
		}
		
		if options.Force {
			args = append(args, "--force")
		}
		
		// Add image overrides
		for key, value := range options.ImageOverrides {
			args = append(args, "--set", fmt.Sprintf("%s=%s", key, value))
			log.Printf("Adding image override: %s=%s", key, value)
		}
		
		// Add set values
		for key, value := range options.SetValues {
			args = append(args, "--set", fmt.Sprintf("%s=%s", key, value))
			log.Printf("Adding set value: %s=%s", key, value)
		}
		
		// Log the full helm command
		log.Printf("Executing helm command: helm %s", strings.Join(args, " "))
		
		// Create a timeout context for the helm command itself
		helmTimeout := 3 * time.Minute // Default timeout for most helm commands
		
		// Special handling for migration operations that need longer timeouts
		if releaseName == "migrate" {
			helmTimeout = 15 * time.Minute // Extended timeout for migration operations
		}
		
		// If options.Timeout is explicitly set and is reasonable, use it
		if options.Timeout > 0 {
			helmTimeout = options.Timeout
		}
		
		// Add some buffer to the context timeout beyond the helm timeout
		contextTimeout := helmTimeout + 30*time.Second
		
		helmCtx, cancel := context.WithTimeout(ctx, contextTimeout)
		defer cancel()
		
		cmd := exec.CommandContext(helmCtx, "helm", args...)
		
		// Start a watchdog goroutine to monitor the process
		watchdogDone := make(chan bool, 1)
		go func() {
			select {
			case <-watchdogDone:
				// Command completed normally
				return
			case <-time.After(helmTimeout):
				// Command is taking too long, kill it
				log.Printf("Helm command for %s exceeded timeout %v, killing process", releaseName, helmTimeout)
				if cmd.Process != nil {
					log.Printf("Sending SIGTERM to helm process %d", cmd.Process.Pid)
					// Try graceful termination first
					cmd.Process.Signal(syscall.SIGTERM)
					time.Sleep(5 * time.Second)
					// Force kill if still running
					log.Printf("Force killing helm process %d", cmd.Process.Pid)
					cmd.Process.Kill()
				}
			}
		}()
		
		output, err := cmd.CombinedOutput()
		watchdogDone <- true // Signal watchdog that command completed
		
		if err != nil {
			// Check if this is a timeout error
			if helmCtx.Err() == context.DeadlineExceeded {
				return fmt.Errorf("helm upgrade timed out after %v: %w", helmTimeout, err)
			}
			return fmt.Errorf("helm upgrade failed: %w, output: %s", err, string(output))
		}
		return nil
	}
	
	// Retry the operation with exponential backoff
	return h.RetryWithBackoff(ctx, upgradeOperation, 3, 5*time.Second)
}

// Status returns the status of a Helm release
func (h *HelmDriver) Status(ctx context.Context, releaseName, namespace string) (helm_port.HelmStatus, error) {
	args := []string{"status", releaseName}
	if namespace != "" {
		args = append(args, "--namespace", namespace)
	}
	args = append(args, "--output", "json")
	
	cmd := exec.CommandContext(ctx, "helm", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return helm_port.HelmStatus{}, fmt.Errorf("helm status failed: %w, output: %s", err, string(output))
	}
	
	// Parse JSON output to extract status information
	// This is a simplified implementation - in practice, you'd use JSON parsing
	status := helm_port.HelmStatus{
		Name:      releaseName,
		Namespace: namespace,
		Status:    "deployed", // Simplified - would parse from JSON
		Revision:  1,          // Simplified - would parse from JSON
		Updated:   time.Now(), // Simplified - would parse from JSON
	}
	
	return status, nil
}

// List returns list of Helm releases
func (h *HelmDriver) List(ctx context.Context, namespace string) ([]helm_port.HelmRelease, error) {
	args := []string{"list"}
	if namespace != "" {
		args = append(args, "--namespace", namespace)
	}
	args = append(args, "--output", "json")
	
	cmd := exec.CommandContext(ctx, "helm", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("helm list failed: %w, output: %s", err, string(output))
	}
	
	// Parse JSON output to extract releases
	// This is a simplified implementation - in practice, you'd use JSON parsing
	releases := []helm_port.HelmRelease{}
	
	return releases, nil
}

// Uninstall removes a Helm release
func (h *HelmDriver) Uninstall(ctx context.Context, releaseName, namespace string) error {
	args := []string{"uninstall", releaseName}
	if namespace != "" {
		args = append(args, "--namespace", namespace)
	}
	
	cmd := exec.CommandContext(ctx, "helm", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("helm uninstall failed: %w, output: %s", err, string(output))
	}
	return nil
}

// History returns the history of a Helm release
func (h *HelmDriver) History(ctx context.Context, releaseName, namespace string) ([]helm_port.HelmRevision, error) {
	args := []string{"history", releaseName}
	
	if namespace != "" {
		args = append(args, "--namespace", namespace)
	}
	
	// Add output format for easier parsing
	args = append(args, "--output", "json")
	
	cmd := exec.CommandContext(ctx, "helm", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("helm history failed: %w, output: %s", err, string(output))
	}
	
	// Parse the JSON output
	var revisions []helm_port.HelmRevision
	if err := h.parseHistoryOutput(string(output), &revisions); err != nil {
		return nil, fmt.Errorf("failed to parse history output: %w", err)
	}
	
	return revisions, nil
}

// IsInstalled checks if Helm is installed
func (h *HelmDriver) IsInstalled() bool {
	_, err := exec.LookPath("helm")
	return err == nil
}

// GetVersion returns the Helm version
func (h *HelmDriver) GetVersion(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, "helm", "version", "--short")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("helm version failed: %w", err)
	}
	return strings.TrimSpace(string(output)), nil
}

// buildImageOverrideArgs builds image override arguments
func (h *HelmDriver) buildImageOverrideArgs(imagePrefix, tagBase, chartName string) []string {
	var args []string
	if imagePrefix != "" && tagBase != "" {
		args = append(args, "--set", fmt.Sprintf("image.repository=%s", imagePrefix))
		args = append(args, "--set", fmt.Sprintf("image.tag=%s-%s", chartName, tagBase))
	}
	return args
}

// parseHelmOutput parses helm command output for structured information
func (h *HelmDriver) parseHelmOutput(output string) map[string]string {
	result := make(map[string]string)
	lines := strings.Split(output, "\n")
	
	for _, line := range lines {
		if strings.Contains(line, ":") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				key := strings.TrimSpace(parts[0])
				value := strings.TrimSpace(parts[1])
				result[key] = value
			}
		}
	}
	
	return result
}

// parseRevision parses revision number from output
func (h *HelmDriver) parseRevision(output string) int {
	result := h.parseHelmOutput(output)
	if revStr, exists := result["REVISION"]; exists {
		if rev, err := strconv.Atoi(revStr); err == nil {
			return rev
		}
	}
	return 1
}

// parseHistoryOutput parses helm history JSON output
func (h *HelmDriver) parseHistoryOutput(output string, revisions *[]helm_port.HelmRevision) error {
	// Helm history JSON output structure
	type helmHistoryItem struct {
		Revision    int    `json:"revision"`
		Status      string `json:"status"`
		Chart       string `json:"chart"`
		AppVersion  string `json:"app_version"`
		Updated     string `json:"updated"`
		Description string `json:"description"`
	}
	
	var items []helmHistoryItem
	if err := json.Unmarshal([]byte(output), &items); err != nil {
		return fmt.Errorf("failed to unmarshal history JSON: %w", err)
	}
	
	*revisions = make([]helm_port.HelmRevision, len(items))
	for i, item := range items {
		// Parse updated time
		updatedTime, err := time.Parse("2006-01-02 15:04:05.000000000 -0700 MST", item.Updated)
		if err != nil {
			// Try alternative time formats
			updatedTime, err = time.Parse(time.RFC3339, item.Updated)
			if err != nil {
				updatedTime = time.Time{}
			}
		}
		
		(*revisions)[i] = helm_port.HelmRevision{
			Revision:    item.Revision,
			Status:      item.Status,
			Chart:       item.Chart,
			AppVersion:  item.AppVersion,
			Updated:     updatedTime,
			Description: item.Description,
		}
	}
	
	return nil
}

// DetectPendingOperation checks for pending Helm operations
func (h *HelmDriver) DetectPendingOperation(ctx context.Context, releaseName, namespace string) (*helm_port.HelmOperation, error) {
	// Add timeout to prevent hanging
	timeoutCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	
	// Check if there's a running Helm operation by checking for helm processes
	pids, err := h.findHelmProcesses(releaseName, namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to check for running helm processes: %w", err)
	}
	
	if len(pids) > 0 {
		// Found running helm processes - check if they're stuck
		stuckPIDs := h.checkStuckProcesses(pids)
		if len(stuckPIDs) > 0 {
			return &helm_port.HelmOperation{
				Type:        "upgrade",
				ReleaseName: releaseName,
				Namespace:   namespace,
				Status:      "stuck",
				StartTime:   time.Now().Add(-30 * time.Minute), // Estimate
				PID:         stuckPIDs[0],
			}, nil
		} else {
			return &helm_port.HelmOperation{
				Type:        "upgrade",
				ReleaseName: releaseName,
				Namespace:   namespace,
				Status:      "running",
				StartTime:   time.Now().Add(-5 * time.Minute), // Estimate
				PID:         pids[0],
			}, nil
		}
	}
	
	// Check for helm lock files or status indicating pending operations
	select {
	case <-timeoutCtx.Done():
		return nil, fmt.Errorf("timeout while checking for helm operations")
	default:
		if hasLockFiles := h.checkHelmLockFiles(releaseName, namespace); hasLockFiles {
			return &helm_port.HelmOperation{
				Type:        "unknown",
				ReleaseName: releaseName,
				Namespace:   namespace,
				Status:      "pending",
				StartTime:   time.Now().Add(-10 * time.Minute), // Estimate
				PID:         0,
			}, nil
		}
	}
	
	return nil, nil // No pending operations found
}

// CleanupStuckOperations cleans up stuck Helm operations
func (h *HelmDriver) CleanupStuckOperations(ctx context.Context, releaseName, namespace string) error {
	// Add timeout to prevent hanging
	timeoutCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	
	// First, check the current release status
	status, err := h.getReleaseStatus(timeoutCtx, releaseName, namespace)
	if err == nil && status == "pending-upgrade" {
		// Release is in pending-upgrade state, try to rollback
		if err := h.rollbackRelease(timeoutCtx, releaseName, namespace); err != nil {
			return fmt.Errorf("failed to rollback stuck release: %w", err)
		}
		
		// Wait a bit for rollback to complete
		select {
		case <-timeoutCtx.Done():
			return fmt.Errorf("timeout during rollback wait")
		case <-time.After(5 * time.Second):
			// Continue
		}
	}
	
	// Then try to detect any stuck operations
	operation, err := h.DetectPendingOperation(timeoutCtx, releaseName, namespace)
	if err != nil {
		return fmt.Errorf("failed to detect pending operations: %w", err)
	}
	
	if operation == nil {
		return nil // No stuck operations to clean up
	}
	
	// Kill stuck processes
	if operation.PID > 0 {
		if err := h.killProcess(operation.PID); err != nil {
			return fmt.Errorf("failed to kill stuck process %d: %w", operation.PID, err)
		}
	}
	
	// Clean up any lock files
	if err := h.cleanupLockFiles(releaseName, namespace); err != nil {
		return fmt.Errorf("failed to cleanup lock files: %w", err)
	}
	
	// Wait a bit for cleanup to take effect
	select {
	case <-timeoutCtx.Done():
		return fmt.Errorf("timeout during cleanup wait")
	case <-time.After(2 * time.Second):
		// Continue
	}
	
	return nil
}

// RetryWithBackoff retries an operation with exponential backoff
func (h *HelmDriver) RetryWithBackoff(ctx context.Context, operation func() error, maxRetries int, baseDelay time.Duration) error {
	var lastErr error
	
	for attempt := 0; attempt < maxRetries; attempt++ {
		// Try the operation
		if err := operation(); err != nil {
			lastErr = err
			
			// Check if this is a "another operation in progress" error
			if strings.Contains(err.Error(), "another operation") && strings.Contains(err.Error(), "in progress") {
				// Calculate delay with exponential backoff and jitter
				delay := time.Duration(math.Pow(2, float64(attempt))) * baseDelay
				jitter := time.Duration(rand.Float64() * float64(baseDelay/2))
				totalDelay := delay + jitter
				
				// Cap the delay to prevent extremely long waits
				if totalDelay > 5*time.Minute {
					totalDelay = 5*time.Minute
				}
				
				// Wait before retrying
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(totalDelay):
					// Continue to next attempt
				}
			} else {
				// Different error, no point in retrying
				return err
			}
		} else {
			// Success!
			return nil
		}
	}
	
	return fmt.Errorf("operation failed after %d attempts: %w", maxRetries, lastErr)
}

// findHelmProcesses finds running helm processes for a specific release
func (h *HelmDriver) findHelmProcesses(releaseName, namespace string) ([]int, error) {
	// Use ps to find helm processes with more specific matching
	cmd := exec.Command("ps", "aux")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to run ps command: %w", err)
	}
	
	lines := strings.Split(string(output), "\n")
	var pids []int
	
	// Look for helm processes that match our release and namespace more specifically
	for _, line := range lines {
		// Must contain helm command and either upgrade or install operations
		if strings.Contains(line, "helm") && 
		   (strings.Contains(line, "upgrade") || strings.Contains(line, "install")) &&
		   strings.Contains(line, releaseName) {
			
			// Additional check for namespace if provided
			if namespace != "" && !strings.Contains(line, namespace) {
				continue
			}
			
			// Extract PID from ps output
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				if pid, err := strconv.Atoi(fields[1]); err == nil {
					pids = append(pids, pid)
				}
			}
		}
	}
	
	return pids, nil
}

// checkStuckProcesses checks if processes are stuck (running for too long)
func (h *HelmDriver) checkStuckProcesses(pids []int) []int {
	var stuckPIDs []int
	
	for _, pid := range pids {
		// Check process start time (simplified - in real implementation would check actual process start time)
		if h.isProcessStuck(pid) {
			stuckPIDs = append(stuckPIDs, pid)
		}
	}
	
	return stuckPIDs
}

// isProcessStuck checks if a process has been running for too long
func (h *HelmDriver) isProcessStuck(pid int) bool {
	// Check if process exists
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	
	// Try to send signal 0 to check if process is still running
	err = process.Signal(syscall.Signal(0))
	if err != nil {
		return false // Process is not running
	}
	
	// For simplicity, consider any helm process that we found as potentially stuck
	// In a real implementation, you'd check the actual process start time
	return true
}

// killProcess kills a process by PID
func (h *HelmDriver) killProcess(pid int) error {
	process, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("failed to find process %d: %w", pid, err)
	}
	
	// First try SIGTERM
	if err := process.Signal(syscall.SIGTERM); err != nil {
		return fmt.Errorf("failed to send SIGTERM to process %d: %w", pid, err)
	}
	
	// Wait a bit for graceful shutdown
	time.Sleep(5 * time.Second)
	
	// Check if process is still running
	if err := process.Signal(syscall.Signal(0)); err == nil {
		// Process still running, force kill
		if err := process.Kill(); err != nil {
			return fmt.Errorf("failed to kill process %d: %w", pid, err)
		}
	}
	
	return nil
}

// checkHelmLockFiles checks for helm lock files that might indicate stuck operations
func (h *HelmDriver) checkHelmLockFiles(releaseName, namespace string) bool {
	// Check common locations for helm lock files
	// This is a simplified implementation - actual helm stores state in Kubernetes secrets
	
	// In Helm v3, the state is stored in Kubernetes secrets in the release namespace
	// We could check for secrets with specific patterns, but for now we'll return false
	// as the main conflict detection is done through process checking
	
	return false
}

// cleanupLockFiles removes any lock files for the release
func (h *HelmDriver) cleanupLockFiles(releaseName, namespace string) error {
	// In Helm v3, we don't have traditional lock files
	// The state is managed through Kubernetes secrets
	// For now, we'll just return nil as the main cleanup is done through process killing
	
	return nil
}

// getReleaseStatus gets the current status of a release
func (h *HelmDriver) getReleaseStatus(ctx context.Context, releaseName, namespace string) (string, error) {
	args := []string{"status", releaseName}
	if namespace != "" {
		args = append(args, "--namespace", namespace)
	}
	args = append(args, "--output", "json")
	
	cmd := exec.CommandContext(ctx, "helm", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to get release status: %w, output: %s", err, string(output))
	}
	
	// Parse JSON to extract status
	var statusData struct {
		Info struct {
			Status string `json:"status"`
		} `json:"info"`
	}
	
	if err := json.Unmarshal(output, &statusData); err != nil {
		return "", fmt.Errorf("failed to parse status JSON: %w", err)
	}
	
	return statusData.Info.Status, nil
}

// rollbackRelease rolls back a release to the previous revision
func (h *HelmDriver) rollbackRelease(ctx context.Context, releaseName, namespace string) error {
	args := []string{"rollback", releaseName}
	if namespace != "" {
		args = append(args, "--namespace", namespace)
	}
	
	// Add timeout to prevent hanging
	args = append(args, "--timeout", "2m")
	
	cmd := exec.CommandContext(ctx, "helm", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to rollback release: %w, output: %s", err, string(output))
	}
	
	return nil
}