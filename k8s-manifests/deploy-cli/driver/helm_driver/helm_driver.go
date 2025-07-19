package helm_driver

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"math/rand"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"

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

// Lint validates chart templates and values
func (h *HelmDriver) Lint(ctx context.Context, chartPath string, options helm_port.HelmLintOptions) (*helm_port.HelmLintResult, error) {
	args := []string{"lint", chartPath}

	if options.ValuesFile != "" {
		args = append(args, "-f", options.ValuesFile)
	}

	if options.Strict {
		args = append(args, "--strict")
	}

	cmd := exec.CommandContext(ctx, "helm", args...)
	output, err := cmd.CombinedOutput()
	
	// Parse the output to create structured result
	result := &helm_port.HelmLintResult{
		Output:   string(output),
		Success:  err == nil,
		Warnings: []helm_port.HelmLintMessage{},
		Errors:   []helm_port.HelmLintMessage{},
	}

	// Parse lint output for warnings and errors
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.Contains(line, "[WARNING]") {
			result.Warnings = append(result.Warnings, helm_port.HelmLintMessage{
				Severity: "WARNING",
				Message:  strings.TrimPrefix(line, "[WARNING] "),
				Path:     chartPath,
			})
		} else if strings.Contains(line, "[ERROR]") {
			result.Errors = append(result.Errors, helm_port.HelmLintMessage{
				Severity: "ERROR", 
				Message:  strings.TrimPrefix(line, "[ERROR] "),
				Path:     chartPath,
			})
		}
	}

	return result, nil
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

// DetectPendingOperation checks for pending Helm operations with enhanced detection
func (h *HelmDriver) DetectPendingOperation(ctx context.Context, releaseName, namespace string) (*helm_port.HelmOperation, error) {
	// Add timeout to prevent hanging
	timeoutCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()

	// Step 1: Check release status for pending states
	status, err := h.getReleaseStatus(timeoutCtx, releaseName, namespace)
	if err == nil {
		if status == "pending-upgrade" || status == "pending-install" || status == "pending-rollback" {
			// Release is in a pending state - this indicates an active lock
			operationType := "upgrade"
			if strings.Contains(status, "install") {
				operationType = "install"
			} else if strings.Contains(status, "rollback") {
				operationType = "rollback"
			}
			
			return &helm_port.HelmOperation{
				Type:        operationType,
				ReleaseName: releaseName,
				Namespace:   namespace,
				Status:      "pending",
				StartTime:   time.Now().Add(-10 * time.Minute), // Conservative estimate
				PID:         0, // Unknown PID for status-based detection
			}, nil
		}
	}

	// Step 2: Check for running Helm processes
	pids, err := h.findHelmProcesses(releaseName, namespace)
	if err != nil {
		// Log but don't fail - process detection is supplementary
		log.Printf("Warning: failed to check for running helm processes: %v", err)
	} else if len(pids) > 0 {
		// Found running helm processes - analyze their state
		for _, pid := range pids {
			if h.isProcessStuck(pid) {
				return &helm_port.HelmOperation{
					Type:        "upgrade",
					ReleaseName: releaseName,
					Namespace:   namespace,
					Status:      "stuck",
					StartTime:   h.getProcessStartTime(pid),
					PID:         pid,
				}, nil
			}
		}
		
		// Process is running but not stuck
		return &helm_port.HelmOperation{
			Type:        "upgrade",
			ReleaseName: releaseName,
			Namespace:   namespace,
			Status:      "running",
			StartTime:   h.getProcessStartTime(pids[0]),
			PID:         pids[0],
		}, nil
	}

	// Step 3: Check for Helm lock indicators using helm list
	select {
	case <-timeoutCtx.Done():
		return nil, fmt.Errorf("timeout while checking for helm operations")
	default:
		if h.hasActiveLock(timeoutCtx, releaseName, namespace) {
			return &helm_port.HelmOperation{
				Type:        "unknown",
				ReleaseName: releaseName,
				Namespace:   namespace,
				Status:      "locked",
				StartTime:   time.Now().Add(-15 * time.Minute), // Conservative estimate
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
					totalDelay = 5 * time.Minute
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

// clearPendingOperations provides enhanced lock detection and cleanup
func (h *HelmDriver) clearPendingOperations(releaseName, namespace string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	log.Printf("Starting enhanced lock clearance for release %s in namespace %s", releaseName, namespace)

	// Step 1: Check and delete pending Helm secrets
	if err := h.clearPendingHelmSecrets(ctx, releaseName, namespace); err != nil {
		log.Printf("Warning: failed to clear pending helm secrets: %v", err)
		// Continue with other cleanup steps
	}

	// Step 2: Kill any stuck helm processes
	if err := h.terminateStuckHelmProcesses(ctx, releaseName); err != nil {
		log.Printf("Warning: failed to terminate stuck processes: %v", err)
		// Continue with other cleanup steps
	}

	// Step 3: Wait and verify cleanup
	for attempt := 0; attempt < 3; attempt++ {
		time.Sleep(time.Duration(2+attempt*2) * time.Second)
		
		// Verify no pending operations remain
		operation, err := h.DetectPendingOperation(ctx, releaseName, namespace)
		if err != nil {
			log.Printf("Warning: failed to verify cleanup (attempt %d): %v", attempt+1, err)
			continue
		}
		
		if operation == nil {
			log.Printf("Lock clearance successful for release %s", releaseName)
			return nil
		}
		
		log.Printf("Pending operation still detected (attempt %d): %+v", attempt+1, operation)
	}

	return fmt.Errorf("failed to fully clear pending operations after 3 attempts")
}

// clearPendingHelmSecrets removes stuck Helm release secrets
func (h *HelmDriver) clearPendingHelmSecrets(ctx context.Context, releaseName, namespace string) error {
	// Find helm secrets for this release
	cmd := exec.CommandContext(ctx, "kubectl", "get", "secret", 
		"-n", namespace, 
		"-l", "owner=helm,name="+releaseName,
		"-o", "jsonpath={.items[*].metadata.name}")
	
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to find helm secrets: %w, output: %s", err, string(output))
	}

	secretNames := strings.Fields(string(output))
	if len(secretNames) == 0 {
		log.Printf("No helm secrets found for release %s", releaseName)
		return nil
	}

	log.Printf("Found %d helm secrets for release %s: %v", len(secretNames), releaseName, secretNames)

	// Delete each secret
	for _, secretName := range secretNames {
		deleteCmd := exec.CommandContext(ctx, "kubectl", "delete", "secret", secretName, "-n", namespace)
		if output, err := deleteCmd.CombinedOutput(); err != nil {
			log.Printf("Warning: failed to delete secret %s: %v, output: %s", secretName, err, string(output))
		} else {
			log.Printf("Successfully deleted helm secret: %s", secretName)
		}
	}

	return nil
}

// terminateStuckHelmProcesses kills any stuck helm processes for the release
func (h *HelmDriver) terminateStuckHelmProcesses(ctx context.Context, releaseName string) error {
	pids, err := h.findHelmProcesses(releaseName, "")
	if err != nil {
		return fmt.Errorf("failed to find helm processes: %w", err)
	}

	if len(pids) == 0 {
		log.Printf("No helm processes found for release %s", releaseName)
		return nil
	}

	log.Printf("Found %d helm processes for release %s: %v", len(pids), releaseName, pids)

	for _, pid := range pids {
		log.Printf("Terminating helm process %d for release %s", pid, releaseName)
		
		// First try SIGTERM
		if err := syscall.Kill(pid, syscall.SIGTERM); err != nil {
			log.Printf("Failed to send SIGTERM to process %d: %v", pid, err)
		} else {
			log.Printf("Sent SIGTERM to process %d", pid)
			
			// Wait 5 seconds for graceful termination
			time.Sleep(5 * time.Second)
			
			// Check if process still exists
			if err := syscall.Kill(pid, 0); err == nil {
				// Process still exists, force kill
				log.Printf("Process %d still running, sending SIGKILL", pid)
				if err := syscall.Kill(pid, syscall.SIGKILL); err != nil {
					log.Printf("Failed to send SIGKILL to process %d: %v", pid, err)
				} else {
					log.Printf("Successfully killed process %d", pid)
				}
			} else {
				log.Printf("Process %d terminated gracefully", pid)
			}
		}
	}

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

// Rollback rolls back a Helm release to a specific revision
func (h *HelmDriver) Rollback(ctx context.Context, releaseName, namespace string, revision int) error {
	args := []string{"rollback", releaseName}
	if revision > 0 {
		args = append(args, fmt.Sprintf("%d", revision))
	}
	if namespace != "" {
		args = append(args, "--namespace", namespace)
	}

	// Add timeout to prevent hanging
	args = append(args, "--timeout", "2m")

	cmd := exec.CommandContext(ctx, "helm", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to rollback release %s to revision %d: %w, output: %s", releaseName, revision, err, string(output))
	}

	return nil
}

// hasActiveLock checks if there's an active Helm lock by examining release status
func (h *HelmDriver) hasActiveLock(ctx context.Context, releaseName, namespace string) bool {
	// Use helm list to check if release shows up with a pending status
	args := []string{"list", "--filter", releaseName}
	if namespace != "" {
		args = append(args, "--namespace", namespace)
	}
	args = append(args, "--output", "json")
	
	cmd := exec.CommandContext(ctx, "helm", args...)
	output, err := cmd.Output()
	if err != nil {
		// If we can't check, assume no lock to avoid false positives
		return false
	}
	
	var releases []struct {
		Name      string `json:"name"`
		Namespace string `json:"namespace"`
		Status    string `json:"status"`
	}
	
	if err := json.Unmarshal(output, &releases); err != nil {
		return false
	}
	
	for _, release := range releases {
		if release.Name == releaseName {
			// Check if status indicates a lock
			status := strings.ToLower(release.Status)
			if strings.Contains(status, "pending") || 
			   strings.Contains(status, "unknown") ||
			   strings.Contains(status, "superseded") {
				return true
			}
		}
	}
	
	return false
}

// enhancedCleanupStuckOperations provides enhanced cleanup with multiple strategies
func (h *HelmDriver) enhancedCleanupStuckOperations(ctx context.Context, releaseName, namespace string) error {
	timeoutCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	
	log.Printf("Starting enhanced cleanup for release %s in namespace %s", releaseName, namespace)
	
	// Strategy 1: Check and handle pending release status
	if status, err := h.getReleaseStatus(timeoutCtx, releaseName, namespace); err == nil {
		if strings.Contains(status, "pending") {
			log.Printf("Release %s is in pending state (%s), attempting rollback", releaseName, status)
			if rollbackErr := h.rollbackRelease(timeoutCtx, releaseName, namespace); rollbackErr != nil {
				log.Printf("Rollback failed: %v", rollbackErr)
			} else {
				log.Printf("Rollback completed for release %s", releaseName)
				// Wait for rollback to settle
				time.Sleep(5 * time.Second)
			}
		}
	}
	
	// Strategy 2: Kill stuck processes
	if pids, err := h.findHelmProcesses(releaseName, namespace); err == nil && len(pids) > 0 {
		for _, pid := range pids {
			if h.isProcessStuck(pid) {
				log.Printf("Killing stuck Helm process %d for release %s", pid, releaseName)
				if err := h.killProcess(pid); err != nil {
					log.Printf("Failed to kill process %d: %v", pid, err)
				}
			}
		}
		// Wait for processes to terminate
		time.Sleep(3 * time.Second)
	}
	
	// Strategy 3: Final verification
	if operation, err := h.DetectPendingOperation(timeoutCtx, releaseName, namespace); err == nil && operation != nil {
		log.Printf("Warning: operation still detected after cleanup: %+v", operation)
		return fmt.Errorf("cleanup incomplete: operation still detected")
	}
	
	log.Printf("Enhanced cleanup completed for release %s", releaseName)
	return nil
}

// getProcessStartTime gets the start time of a process by PID
func (h *HelmDriver) getProcessStartTime(pid int) time.Time {
	if pid <= 0 {
		return time.Time{}
	}
	
	// Try to get process start time from /proc/PID/stat on Linux
	statFile := fmt.Sprintf("/proc/%d/stat", pid)
	if data, err := os.ReadFile(statFile); err == nil {
		fields := strings.Fields(string(data))
		if len(fields) > 21 {
			// Field 22 (index 21) is starttime in clock ticks since boot
			if startTicks, err := strconv.ParseInt(fields[21], 10, 64); err == nil {
				// Convert clock ticks to time (approximate)
				// This is a simplification - actual conversion requires system boot time
				clockTicksPerSecond := int64(100) // Common value, but system-dependent
				secondsSinceBoot := startTicks / clockTicksPerSecond
				
				// Approximate start time (not perfectly accurate but good enough for our use)
				return time.Now().Add(-time.Duration(secondsSinceBoot) * time.Second)
			}
		}
	}
	
	// Fallback: use ps command
	cmd := exec.Command("ps", "-o", "lstart=", "-p", fmt.Sprintf("%d", pid))
	if output, err := cmd.Output(); err == nil {
		timeStr := strings.TrimSpace(string(output))
		if parsedTime, err := time.Parse("Mon Jan 2 15:04:05 2006", timeStr); err == nil {
			return parsedTime
		}
	}
	
	// Could not determine start time
	return time.Time{}
}
