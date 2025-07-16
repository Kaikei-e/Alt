package helm_driver

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
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

// UpgradeInstall installs or upgrades a Helm release
func (h *HelmDriver) UpgradeInstall(ctx context.Context, releaseName, chartPath string, options helm_port.HelmUpgradeOptions) error {
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
	if err != nil {
		return fmt.Errorf("helm upgrade failed: %w, output: %s", err, string(output))
	}
	return nil
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