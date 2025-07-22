// PHASE R3: Shared validation utilities
package shared

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// ValidationHelper provides shared validation functionality
type ValidationHelper struct {
	shared *CommandShared
}

// NewValidationHelper creates a new validation helper
func NewValidationHelper(shared *CommandShared) *ValidationHelper {
	return &ValidationHelper{
		shared: shared,
	}
}

// ValidateEnvironmentName validates environment name format
func (v *ValidationHelper) ValidateEnvironmentName(name string) error {
	if name == "" {
		return fmt.Errorf("environment name cannot be empty")
	}

	// Check length
	if len(name) < 2 || len(name) > 50 {
		return fmt.Errorf("environment name must be between 2 and 50 characters, got %d", len(name))
	}

	// Check format - alphanumeric with hyphens and underscores
	validNameRegex := regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]*[a-zA-Z0-9]$`)
	if !validNameRegex.MatchString(name) {
		return fmt.Errorf("environment name '%s' contains invalid characters. Must start and end with alphanumeric, contain only letters, numbers, hyphens, and underscores", name)
	}

	return nil
}

// ValidateFilePath validates a file path exists and is accessible
func (v *ValidationHelper) ValidateFilePath(path string, mustExist bool) error {
	if path == "" {
		return fmt.Errorf("file path cannot be empty")
	}

	// Clean the path
	cleanPath := filepath.Clean(path)

	// Check if file exists when required
	if mustExist {
		if _, err := os.Stat(cleanPath); os.IsNotExist(err) {
			return fmt.Errorf("file does not exist: %s", cleanPath)
		} else if err != nil {
			return fmt.Errorf("cannot access file %s: %w", cleanPath, err)
		}
	} else {
		// Check if directory exists for new files
		dir := filepath.Dir(cleanPath)
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			return fmt.Errorf("directory does not exist: %s", dir)
		} else if err != nil {
			return fmt.Errorf("cannot access directory %s: %w", dir, err)
		}
	}

	return nil
}

// ValidateDirectoryPath validates a directory path exists and is accessible
func (v *ValidationHelper) ValidateDirectoryPath(path string) error {
	if path == "" {
		return fmt.Errorf("directory path cannot be empty")
	}

	// Clean the path
	cleanPath := filepath.Clean(path)

	// Check if directory exists
	info, err := os.Stat(cleanPath)
	if os.IsNotExist(err) {
		return fmt.Errorf("directory does not exist: %s", cleanPath)
	} else if err != nil {
		return fmt.Errorf("cannot access directory %s: %w", cleanPath, err)
	}

	// Check if it's actually a directory
	if !info.IsDir() {
		return fmt.Errorf("path is not a directory: %s", cleanPath)
	}

	return nil
}

// ValidateTimeout validates timeout duration values
func (v *ValidationHelper) ValidateTimeout(timeout time.Duration) error {
	if timeout < 0 {
		return fmt.Errorf("timeout cannot be negative: %s", timeout)
	}

	if timeout > 24*time.Hour {
		return fmt.Errorf("timeout cannot exceed 24 hours: %s", timeout)
	}

	// Warn about very short timeouts
	if timeout > 0 && timeout < 30*time.Second {
		v.shared.Logger.WarnWithContext("timeout is very short", map[string]interface{}{
			"timeout": timeout.String(),
		})
	}

	return nil
}

// ValidateResourceName validates Kubernetes resource name format
func (v *ValidationHelper) ValidateResourceName(name string) error {
	if name == "" {
		return fmt.Errorf("resource name cannot be empty")
	}

	// Check length (Kubernetes limit)
	if len(name) > 253 {
		return fmt.Errorf("resource name is too long: %d characters (max 253)", len(name))
	}

	// Check format - DNS-1123 subdomain format
	validNameRegex := regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$`)
	if !validNameRegex.MatchString(name) {
		return fmt.Errorf("resource name '%s' is invalid. Must consist of lower case alphanumeric characters, '-' or '.', and must start and end with an alphanumeric character", name)
	}

	return nil
}

// ValidateNamespace validates Kubernetes namespace format
func (v *ValidationHelper) ValidateNamespace(namespace string) error {
	if namespace == "" {
		return fmt.Errorf("namespace cannot be empty")
	}

	// Check length
	if len(namespace) > 63 {
		return fmt.Errorf("namespace is too long: %d characters (max 63)", len(namespace))
	}

	// Check format - DNS-1123 label format
	validNamespaceRegex := regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)
	if !validNamespaceRegex.MatchString(namespace) {
		return fmt.Errorf("namespace '%s' is invalid. Must consist of lower case alphanumeric characters or '-', and must start and end with an alphanumeric character", namespace)
	}

	// Check for reserved namespaces
	reservedNamespaces := []string{
		"kube-system",
		"kube-public",
		"kube-node-lease",
		"kubernetes-dashboard",
	}

	for _, reserved := range reservedNamespaces {
		if namespace == reserved {
			return fmt.Errorf("namespace '%s' is reserved and cannot be used", namespace)
		}
	}

	return nil
}

// ValidateImageTag validates Docker image tag format
func (v *ValidationHelper) ValidateImageTag(tag string) error {
	if tag == "" {
		return fmt.Errorf("image tag cannot be empty")
	}

	// Check length
	if len(tag) > 128 {
		return fmt.Errorf("image tag is too long: %d characters (max 128)", len(tag))
	}

	// Check format - valid Docker tag
	validTagRegex := regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9._-]*[a-zA-Z0-9])?$`)
	if !validTagRegex.MatchString(tag) {
		return fmt.Errorf("image tag '%s' is invalid. Must consist of alphanumeric characters, periods, underscores, and hyphens, and must start and end with alphanumeric", tag)
	}

	return nil
}

// ValidatePort validates port number
func (v *ValidationHelper) ValidatePort(port int) error {
	if port < 1 || port > 65535 {
		return fmt.Errorf("port %d is invalid. Must be between 1 and 65535", port)
	}

	// Warn about privileged ports
	if port < 1024 {
		v.shared.Logger.WarnWithContext("using privileged port", map[string]interface{}{
			"port": port,
		})
	}

	return nil
}

// ValidateURL validates URL format
func (v *ValidationHelper) ValidateURL(url string) error {
	if url == "" {
		return fmt.Errorf("URL cannot be empty")
	}

	// Basic URL validation
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return fmt.Errorf("URL '%s' must start with http:// or https://", url)
	}

	// Check for obvious issues
	if strings.Contains(url, " ") {
		return fmt.Errorf("URL '%s' contains spaces", url)
	}

	return nil
}

// ValidateSliceNotEmpty validates that a slice is not empty
func (v *ValidationHelper) ValidateSliceNotEmpty(slice []string, fieldName string) error {
	if len(slice) == 0 {
		return fmt.Errorf("%s cannot be empty", fieldName)
	}

	// Check for empty strings in slice
	for i, item := range slice {
		if item == "" {
			return fmt.Errorf("%s[%d] cannot be empty", fieldName, i)
		}
	}

	return nil
}

// ValidateMapNotEmpty validates that a map is not empty
func (v *ValidationHelper) ValidateMapNotEmpty(m map[string]string, fieldName string) error {
	if len(m) == 0 {
		return fmt.Errorf("%s cannot be empty", fieldName)
	}

	// Check for empty keys or values
	for key, value := range m {
		if key == "" {
			return fmt.Errorf("%s cannot have empty keys", fieldName)
		}
		if value == "" {
			return fmt.Errorf("%s['%s'] cannot have empty value", fieldName, key)
		}
	}

	return nil
}

// ValidateStringInSlice validates that a string is in a given slice of valid values
func (v *ValidationHelper) ValidateStringInSlice(value string, validValues []string, fieldName string) error {
	for _, valid := range validValues {
		if value == valid {
			return nil
		}
	}

	return fmt.Errorf("invalid %s '%s'. Valid values: %v", fieldName, value, validValues)
}

// ValidateRegex validates a string against a regex pattern
func (v *ValidationHelper) ValidateRegex(value, pattern, fieldName string) error {
	matched, err := regexp.MatchString(pattern, value)
	if err != nil {
		return fmt.Errorf("invalid regex pattern for %s validation: %w", fieldName, err)
	}

	if !matched {
		return fmt.Errorf("%s '%s' does not match required pattern '%s'", fieldName, value, pattern)
	}

	return nil
}

// ValidateRequired validates that a required field is not empty
func (v *ValidationHelper) ValidateRequired(value, fieldName string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s is required", fieldName)
	}
	return nil
}