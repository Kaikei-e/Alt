package domain

import (
	"strings"
	"time"
)

// ErrorType represents the classification of deployment errors
type ErrorType string

const (
	// Permanent errors that should not be retried
	ErrorTypeValidation     ErrorType = "validation"     // Helm lint, template, syntax errors
	ErrorTypeConfiguration ErrorType = "configuration"  // Invalid chart configuration
	ErrorTypePermission    ErrorType = "permission"     // RBAC, access denied errors
	ErrorTypeResource      ErrorType = "resource"       // Resource conflicts, quota exceeded
	
	// Transient errors that can be retried
	ErrorTypeNetwork      ErrorType = "network"       // Network connectivity issues
	ErrorTypeTemporary    ErrorType = "temporary"     // Temporary service unavailability
	ErrorTypeTimeout      ErrorType = "timeout"       // Operation timeouts
	ErrorTypeLockConflict ErrorType = "lock_conflict" // Helm lock conflicts, another operation in progress
	ErrorTypeUnknown      ErrorType = "unknown"       // Unclassified errors
)

// ErrorClassification contains error analysis results
type ErrorClassification struct {
	Type        ErrorType
	Category    string        // Error category
	Retriable   bool
	Reason      string
	Suggestion  string
	Timestamp   time.Time
}

// DeploymentError represents a deployment error with classification
type DeploymentError struct {
	ChartName      string
	Namespace      string
	Operation      string // "lint", "template", "install", "upgrade"
	OriginalError  error
	Classification ErrorClassification
	Attempt        int
	LastAttempt    time.Time
}

// IsRetriable returns whether the error should be retried
func (e *DeploymentError) IsRetriable() bool {
	return e.Classification.Retriable
}

// ShouldRetry determines if we should retry based on classification and attempts
func (e *DeploymentError) ShouldRetry(maxRetries int, backoffDuration time.Duration) bool {
	if !e.IsRetriable() {
		return false
	}
	
	if e.Attempt >= maxRetries {
		return false
	}
	
	// Check if enough time has passed since last attempt for backoff
	if time.Since(e.LastAttempt) < backoffDuration {
		return false
	}
	
	return true
}

// ErrorClassifier provides error classification functionality
type ErrorClassifier struct{}

// NewErrorClassifier creates a new error classifier
func NewErrorClassifier() *ErrorClassifier {
	return &ErrorClassifier{}
}

// ClassifyError analyzes an error and returns its classification
func (c *ErrorClassifier) ClassifyError(err error, operation string) ErrorClassification {
	if err == nil {
		return ErrorClassification{
			Type:      ErrorTypeUnknown,
			Retriable: false,
			Reason:    "No error provided",
			Timestamp: time.Now(),
		}
	}

	errorMsg := strings.ToLower(err.Error())
	
	// Validation errors (never retry)
	if c.isValidationError(errorMsg) {
		return ErrorClassification{
			Type:      ErrorTypeValidation,
			Retriable: false,
			Reason:    "Chart validation failed",
			Suggestion: "Fix chart templates, values, or YAML syntax issues",
			Timestamp: time.Now(),
		}
	}
	
	// Configuration errors (never retry)
	if c.isConfigurationError(errorMsg) {
		return ErrorClassification{
			Type:      ErrorTypeConfiguration,
			Retriable: false,
			Reason:    "Chart configuration error",
			Suggestion: "Review chart values and template configuration",
			Timestamp: time.Now(),
		}
	}
	
	// Permission errors (never retry)
	if c.isPermissionError(errorMsg) {
		return ErrorClassification{
			Type:      ErrorTypePermission,
			Retriable: false,
			Reason:    "Insufficient permissions",
			Suggestion: "Check RBAC permissions and service account configuration",
			Timestamp: time.Now(),
		}
	}
	
	// Resource errors (sometimes retry)
	if c.isResourceError(errorMsg) {
		return ErrorClassification{
			Type:      ErrorTypeResource,
			Retriable: c.isRetriableResourceError(errorMsg),
			Reason:    "Resource constraint or conflict",
			Suggestion: "Check resource quotas, conflicts, or dependencies",
			Timestamp: time.Now(),
		}
	}
	
	// Network errors (retry)
	if c.isNetworkError(errorMsg) {
		return ErrorClassification{
			Type:      ErrorTypeNetwork,
			Retriable: true,
			Reason:    "Network connectivity issue",
			Suggestion: "Check network connectivity to Kubernetes API",
			Timestamp: time.Now(),
		}
	}
	
	// Lock conflict errors (retry with special handling)
	if c.isLockConflictError(errorMsg) {
		return ErrorClassification{
			Type:      ErrorTypeLockConflict,
			Retriable: true,
			Reason:    "Helm lock conflict detected",
			Suggestion: "Clean up stale Helm secrets and retry",
			Timestamp: time.Now(),
		}
	}
	
	// Timeout errors (retry)
	if c.isTimeoutError(errorMsg) {
		return ErrorClassification{
			Type:      ErrorTypeTimeout,
			Retriable: true,
			Reason:    "Operation timeout",
			Suggestion: "Increase timeout or check system load",
			Timestamp: time.Now(),
		}
	}
	
	// Temporary errors (retry)
	if c.isTemporaryError(errorMsg) {
		return ErrorClassification{
			Type:      ErrorTypeTemporary,
			Retriable: true,
			Reason:    "Temporary service issue",
			Suggestion: "Wait and retry the operation",
			Timestamp: time.Now(),
		}
	}
	
	// Default to unknown but retriable with caution
	return ErrorClassification{
		Type:      ErrorTypeUnknown,
		Retriable: true,
		Reason:    "Unclassified error",
		Suggestion: "Investigate error details and consider manual intervention",
		Timestamp: time.Now(),
	}
}

// isValidationError checks for validation-related errors
func (c *ErrorClassifier) isValidationError(errorMsg string) bool {
	validationKeywords := []string{
		"validation failed",
		"unknown field",
		"yaml:",
		"json:",
		"template:",
		"lint",
		"invalid yaml",
		"invalid json",
		"syntax error",
		"parse error",
		"chart has no values",
		"values don't meet the specifications",
		"failed to parse",
		"unmarshal",
	}
	
	return c.containsAny(errorMsg, validationKeywords)
}

// isConfigurationError checks for configuration-related errors
func (c *ErrorClassifier) isConfigurationError(errorMsg string) bool {
	configKeywords := []string{
		"invalid configuration",
		"missing required",
		"invalid value for",
		"unsupported value",
		"invalid chart",
		"invalid values",
		"required value",
		"chart requires",
		"incompatible",
		"invalid template",
	}
	
	return c.containsAny(errorMsg, configKeywords)
}

// isPermissionError checks for permission-related errors
func (c *ErrorClassifier) isPermissionError(errorMsg string) bool {
	permissionKeywords := []string{
		"forbidden",
		"access denied",
		"unauthorized",
		"permission denied",
		"rbac",
		"insufficient privileges",
		"not allowed",
		"authentication",
		"serviceaccount",
		"clusterrole",
	}
	
	return c.containsAny(errorMsg, permissionKeywords)
}

// isResourceError checks for resource-related errors
func (c *ErrorClassifier) isResourceError(errorMsg string) bool {
	resourceKeywords := []string{
		"already exists",
		"resource conflict",
		"quota exceeded",
		"insufficient resources",
		"resource not found",
		"storage class",
		"persistent volume",
		"resource quota",
		"limit exceeded",
	}
	
	return c.containsAny(errorMsg, resourceKeywords)
}

// isRetriableResourceError determines if a resource error can be retried
func (c *ErrorClassifier) isRetriableResourceError(errorMsg string) bool {
	// Some resource errors are temporary and can be retried
	retriableResourceKeywords := []string{
		"insufficient resources",
		"resource temporarily unavailable",
		"quota exceeded", // might be temporary if other resources are freed
		"storage class not found", // might be added later
	}
	
	return c.containsAny(errorMsg, retriableResourceKeywords)
}

// isNetworkError checks for network-related errors
func (c *ErrorClassifier) isNetworkError(errorMsg string) bool {
	networkKeywords := []string{
		"connection refused",
		"network unreachable",
		"timeout",
		"connection timeout",
		"dial tcp",
		"connection reset",
		"dns",
		"resolve",
		"connection failed",
		"network error",
		"connection lost",
	}
	
	return c.containsAny(errorMsg, networkKeywords)
}

// isTimeoutError checks for timeout-related errors
func (c *ErrorClassifier) isTimeoutError(errorMsg string) bool {
	timeoutKeywords := []string{
		"timeout",
		"deadline exceeded",
		"context deadline exceeded",
		"operation timeout",
		"timed out",
		"time limit exceeded",
	}
	
	return c.containsAny(errorMsg, timeoutKeywords)
}

// isTemporaryError checks for temporary errors
func (c *ErrorClassifier) isTemporaryError(errorMsg string) bool {
	temporaryKeywords := []string{
		"service unavailable",
		"temporarily unavailable",
		"try again",
		"server error",
		"internal error",
		"temporary failure",
		"resource busy",
		"too many requests",
		"rate limit",
	}
	
	return c.containsAny(errorMsg, temporaryKeywords)
}

// isLockConflictError checks for Helm lock conflict errors
func (c *ErrorClassifier) isLockConflictError(errorMsg string) bool {
	lockKeywords := []string{
		"another operation in progress",
		"another operation (install/upgrade/rollback) is in progress",
		"operation in progress",
		"pending-install",
		"pending-upgrade", 
		"pending-rollback",
		"resource busy",
		"helm lock",
		"operation already in progress",
	}
	
	return c.containsAny(errorMsg, lockKeywords)
}

// containsAny checks if the error message contains any of the given keywords
func (c *ErrorClassifier) containsAny(errorMsg string, keywords []string) bool {
	for _, keyword := range keywords {
		if strings.Contains(errorMsg, keyword) {
			return true
		}
	}
	return false
}