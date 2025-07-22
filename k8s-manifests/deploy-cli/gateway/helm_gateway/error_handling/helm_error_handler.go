// PHASE R2: Helm error handling and recovery functionality
package error_handling

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"deploy-cli/domain"
	"deploy-cli/port/helm_port"
	"deploy-cli/port/logger_port"
)

// HelmErrorHandler handles Helm-specific error classification and recovery
type HelmErrorHandler struct {
	helmPort helm_port.HelmPort
	logger   logger_port.LoggerPort
	
	// Error classification patterns
	errorPatterns map[string]*domain.ErrorPattern
}

// HelmErrorHandlerPort defines the interface for Helm error handling operations
type HelmErrorHandlerPort interface {
	ClassifyError(ctx context.Context, err error, operation string) (*domain.ErrorClassification, error)
	SuggestRecoveryActions(ctx context.Context, classification *domain.ErrorClassification) ([]*domain.RecoveryAction, error)
	ExecuteRecoveryAction(ctx context.Context, action *domain.RecoveryAction, context *domain.ErrorContext) (*domain.RecoveryResult, error)
	GetErrorHistory(ctx context.Context, releaseName, namespace string) ([]*domain.ErrorEvent, error)
	ReportError(ctx context.Context, errorEvent *domain.ErrorEvent) error
}

// NewHelmErrorHandler creates a new Helm error handler
func NewHelmErrorHandler(
	helmPort helm_port.HelmPort,
	logger logger_port.LoggerPort,
) *HelmErrorHandler {
	handler := &HelmErrorHandler{
		helmPort: helmPort,
		logger:   logger,
	}
	
	// Initialize error patterns
	handler.initializeErrorPatterns()
	
	return handler
}

// ClassifyError classifies Helm errors by type, severity, and recoverability
func (h *HelmErrorHandler) ClassifyError(ctx context.Context, err error, operation string) (*domain.ErrorClassification, error) {
	h.logger.DebugWithContext("classifying Helm error", map[string]interface{}{
		"operation": operation,
		"error":     err.Error(),
	})

	classification := &domain.ErrorClassification{
		Type:      domain.ErrorTypeUnknown,
		Retriable: false,
		Reason:    "Unclassified Helm error",
		Suggestion: "Manual investigation required",
		Timestamp: time.Now(),
	}

	errorMessage := strings.ToLower(err.Error())

	// Classify by pattern matching
	for patternName, pattern := range h.errorPatterns {
		if regexp.MustCompile(pattern.Pattern).MatchString(errorMessage) {
			classification.Type = domain.ErrorType(patternName)
			classification.Retriable = len(pattern.Actions) > 0
			classification.Reason = pattern.Description
			classification.Suggestion = fmt.Sprintf("See pattern: %s", pattern.Name)
			
			h.logger.DebugWithContext("error classified", map[string]interface{}{
				"operation":   operation,
				"type":        classification.Type,
				"reason":      classification.Reason,
				"retriable":   classification.Retriable,
			})
			
			return classification, nil
		}
	}

	// Default classification for unmatched errors
	classification.Type = domain.ErrorTypeUnknown
	classification.Reason = "Unclassified Helm error"
	classification.Suggestion = "Manual investigation required"
	
	h.logger.WarnWithContext("unclassified Helm error", map[string]interface{}{
		"operation": operation,
		"error":     err.Error(),
	})

	return classification, nil
}

// SuggestRecoveryActions suggests appropriate recovery actions based on error classification
func (h *HelmErrorHandler) SuggestRecoveryActions(ctx context.Context, classification *domain.ErrorClassification) ([]*domain.RecoveryAction, error) {
	h.logger.DebugWithContext("suggesting recovery actions", map[string]interface{}{
		"error_type": classification.Type,
		"reason":     classification.Reason,
		"retriable":  classification.Retriable,
	})

	actions := make([]*domain.RecoveryAction, 0)

	// Suggest actions based on error type
	switch classification.Type {
	case "release_not_found":
		actions = append(actions, &domain.RecoveryAction{
			Type:        domain.RecoveryTypeRetry,
			Name:        "Install release",
			Description: "Install the release instead of upgrading",
			Command:     "helm install",
			Automated:   true,
			Retries:     1,
		})

	case "timeout_error":
		actions = append(actions, 
			&domain.RecoveryAction{
				Type:        domain.RecoveryActionTypeRetry,
				Priority:    domain.RecoveryPriorityMedium,
				Description: "Retry with increased timeout",
				Command:     "helm upgrade --timeout=10m",
				AutoRetry:   true,
				RetryCount:  2,
				RetryDelay:  30 * time.Second,
			},
			&domain.RecoveryAction{
				Type:        domain.RecoveryActionTypeRollback,
				Priority:    domain.RecoveryPriorityLow,
				Description: "Rollback to previous version",
				Command:     "helm rollback",
				AutoRetry:   false,
			},
		)

	case "resource_conflict":
		actions = append(actions, 
			&domain.RecoveryAction{
				Type:        domain.RecoveryActionTypeForce,
				Priority:    domain.RecoveryPriorityMedium,
				Description: "Force update conflicting resources",
				Command:     "helm upgrade --force",
				AutoRetry:   false,
				RequiresConfirmation: true,
			},
			&domain.RecoveryAction{
				Type:        domain.RecoveryActionTypeCleanup,
				Priority:    domain.RecoveryPriorityHigh,
				Description: "Clean up conflicting resources manually",
				Command:     "kubectl delete",
				AutoRetry:   false,
				RequiresConfirmation: true,
			},
		)

	case "insufficient_permissions":
		actions = append(actions, &domain.RecoveryAction{
			Type:        domain.RecoveryActionTypeManual,
			Priority:    domain.RecoveryPriorityHigh,
			Description: "Check and update RBAC permissions",
			Command:     "kubectl auth can-i",
			AutoRetry:   false,
			RequiresConfirmation: true,
		})

	case "chart_not_found":
		actions = append(actions, 
			&domain.RecoveryAction{
				Type:        domain.RecoveryActionTypeRetry,
				Priority:    domain.RecoveryPriorityHigh,
				Description: "Update chart repositories and retry",
				Command:     "helm repo update",
				AutoRetry:   true,
				RetryCount:  1,
			},
			&domain.RecoveryAction{
				Type:        domain.RecoveryActionTypeManual,
				Priority:    domain.RecoveryPriorityMedium,
				Description: "Verify chart name and repository",
				Command:     "helm search repo",
				AutoRetry:   false,
			},
		)

	case "values_validation_error":
		actions = append(actions, 
			&domain.RecoveryAction{
				Type:        domain.RecoveryActionTypeManual,
				Priority:    domain.RecoveryPriorityHigh,
				Description: "Validate and fix chart values",
				Command:     "helm lint",
				AutoRetry:   false,
			},
			&domain.RecoveryAction{
				Type:        domain.RecoveryActionTypeRetry,
				Priority:    domain.RecoveryPriorityMedium,
				Description: "Retry with default values",
				Command:     "helm upgrade --reset-values",
				AutoRetry:   false,
			},
		)

	case "hook_failure":
		actions = append(actions, 
			&domain.RecoveryAction{
				Type:        domain.RecoveryActionTypeRetry,
				Priority:    domain.RecoveryPriorityMedium,
				Description: "Retry without hooks",
				Command:     "helm upgrade --no-hooks",
				AutoRetry:   false,
			},
			&domain.RecoveryAction{
				Type:        domain.RecoveryActionTypeCleanup,
				Priority:    domain.RecoveryPriorityLow,
				Description: "Clean up failed hook resources",
				Command:     "kubectl delete job",
				AutoRetry:   false,
			},
		)

	default:
		// Generic recovery actions
		actions = append(actions, 
			&domain.RecoveryAction{
				Type:        domain.RecoveryActionTypeRetry,
				Priority:    domain.RecoveryPriorityLow,
				Description: "Simple retry",
				Command:     "retry original command",
				AutoRetry:   true,
				RetryCount:  1,
				RetryDelay:  5 * time.Second,
			},
			&domain.RecoveryAction{
				Type:        domain.RecoveryActionTypeManual,
				Priority:    domain.RecoveryPriorityMedium,
				Description: "Manual investigation required",
				Command:     "kubectl get pods,events",
				AutoRetry:   false,
			},
		)
	}

	h.logger.DebugWithContext("recovery actions suggested", map[string]interface{}{
		"error_type":   classification.Type,
		"action_count": len(actions),
	})

	return actions, nil
}

// ExecuteRecoveryAction executes a specific recovery action
func (h *HelmErrorHandler) ExecuteRecoveryAction(ctx context.Context, action *domain.RecoveryAction, errorContext *domain.ErrorContext) (*domain.RecoveryResult, error) {
	h.logger.InfoWithContext("executing recovery action", map[string]interface{}{
		"action_type": action.Type,
		"description": action.Description,
		"command":     action.Command,
		"auto_retry":  action.AutoRetry,
	})

	result := &domain.RecoveryResult{
		Action:    action,
		StartTime: time.Now(),
		Success:   false,
	}

	var err error
	switch action.Type {
	case domain.RecoveryActionTypeRetry:
		result.Success, err = h.executeRetryAction(ctx, action, errorContext)
		
	case domain.RecoveryActionTypeRollback:
		result.Success, err = h.executeRollbackAction(ctx, action, errorContext)
		
	case domain.RecoveryActionTypeForce:
		result.Success, err = h.executeForceAction(ctx, action, errorContext)
		
	case domain.RecoveryActionTypeCleanup:
		result.Success, err = h.executeCleanupAction(ctx, action, errorContext)
		
	case domain.RecoveryActionTypeManual:
		result.Success = false
		err = fmt.Errorf("manual action required: %s", action.Description)
		result.ManualSteps = []string{action.Description, fmt.Sprintf("Run: %s", action.Command)}
		
	default:
		err = fmt.Errorf("unknown recovery action type: %s", action.Type)
		result.Success = false
	}
	
	if err != nil {
		result.Error = err.Error()
	}

	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(result.StartTime)

	h.logger.InfoWithContext("recovery action execution completed", map[string]interface{}{
		"action_type": action.Type,
		"success":     result.Success,
		"duration":    result.Duration.String(),
		"error":       result.Error,
	})

	return result, nil
}

// GetErrorHistory retrieves error history for a specific release
func (h *HelmErrorHandler) GetErrorHistory(ctx context.Context, releaseName, namespace string) ([]*domain.ErrorEvent, error) {
	h.logger.DebugWithContext("getting error history", map[string]interface{}{
		"release_name": releaseName,
		"namespace":    namespace,
	})

	// This would typically query a persistent error storage
	// For now, return empty history
	history := make([]*domain.ErrorEvent, 0)

	h.logger.DebugWithContext("error history retrieved", map[string]interface{}{
		"release_name": releaseName,
		"namespace":    namespace,
		"event_count":  len(history),
	})

	return history, nil
}

// ReportError reports an error event for tracking and analysis
func (h *HelmErrorHandler) ReportError(ctx context.Context, errorEvent *domain.ErrorEvent) error {
	h.logger.InfoWithContext("reporting error event", map[string]interface{}{
		"release_name": errorEvent.ReleaseName,
		"namespace":    errorEvent.Namespace,
		"error_type":   errorEvent.ErrorType,
		"severity":     errorEvent.Severity,
	})

	// This would typically store the error in a persistent storage
	// For now, just log it
	h.logger.ErrorWithContext("error event reported", map[string]interface{}{
		"event_id":     errorEvent.ID,
		"release_name": errorEvent.ReleaseName,
		"namespace":    errorEvent.Namespace,
		"error_type":   errorEvent.ErrorType,
		"message":      errorEvent.Message,
		"timestamp":    errorEvent.Timestamp,
	})

	return nil
}

// Private helper methods

// initializeErrorPatterns initializes common Helm error patterns
func (h *HelmErrorHandler) initializeErrorPatterns() {
	h.errorPatterns = map[string]*domain.ErrorPattern{
		"release_not_found": {
			ID:          "release_not_found",
			Name:        "Release Not Found",
			Pattern:     `release.*not found|no release found`,
			Category:    domain.ErrorCategoryConfiguration,
			Severity:    domain.ErrorSeverityMedium,
			Description: "Helm release not found",
			Keywords:    []string{"release", "not found"},
		},
		"timeout_error": {
			ID:          "timeout_error",
			Name:        "Timeout Error",
			Pattern:     `timeout|timed out|deadline exceeded`,
			Category:    domain.ErrorCategoryTimeout,
			Severity:    domain.ErrorSeverityMedium,
			Description: "Operation timed out",
			Keywords:    []string{"timeout", "timed out", "deadline"},
		},
		"resource_conflict": {
			ID:          "resource_conflict",
			Name:        "Resource Conflict",
			Pattern:     `already exists|conflict|resource version conflict`,
			Category:    domain.ErrorCategoryResource,
			Severity:    domain.ErrorSeverityMedium,
			Description: "Resource conflict detected",
			Keywords:    []string{"exists", "conflict", "resource"},
		},
		"insufficient_permissions": {
			ID:          "insufficient_permissions",
			Name:        "Insufficient Permissions",
			Pattern:     `forbidden|unauthorized|permission denied|cannot.*verb`,
			Category:    domain.ErrorCategoryPermission,
			Severity:    domain.ErrorSeverityHigh,
			Description: "Insufficient permissions",
			Keywords:    []string{"forbidden", "unauthorized", "permission", "denied"},
		},
		"chart_not_found": {
			ID:          "chart_not_found",
			Name:        "Chart Not Found",
			Pattern:     `chart.*not found|no such chart|failed to find chart`,
			Category:    domain.ErrorCategoryConfiguration,
			Severity:    domain.ErrorSeverityHigh,
			Description: "Chart not found",
			Keywords:    []string{"chart", "not found"},
		},
		"values_validation_error": {
			ID:          "values_validation_error",
			Name:        "Values Validation Error",
			Pattern:     `values.*validation|invalid.*values|values.*error`,
			Category:    domain.ErrorCategoryValidation,
			Severity:    domain.ErrorSeverityMedium,
			Description: "Chart values validation failed",
			Keywords:    []string{"values", "validation", "invalid"},
		},
		"network_error": {
			ID:          "network_error",
			Name:        "Network Error",
			Pattern:     `connection refused|network.*unreachable|dns.*resolution.*failed`,
			Category:    domain.ErrorCategoryNetwork,
			Severity:    domain.ErrorSeverityMedium,
			Description: "Network connectivity issue",
			Keywords:    []string{"connection", "network", "dns"},
		},
		"storage_error": {
			ID:          "storage_error",
			Name:        "Storage Error",
			Pattern:     `no space left|disk full|storage.*exceeded`,
			Category:    domain.ErrorCategoryResource,
			Severity:    domain.ErrorSeverityHigh,
			Description: "Storage capacity issue",
			Keywords:    []string{"space", "disk", "storage"},
		},
	}
}

// executeRetryAction executes a retry recovery action
func (h *HelmErrorHandler) executeRetryAction(ctx context.Context, action *domain.RecoveryAction, errorContext *domain.ErrorContext) (bool, error) {
	h.logger.DebugWithContext("executing retry action", map[string]interface{}{
		"retry_count": action.RetryCount,
		"retry_delay": action.RetryDelay.String(),
	})

	// Apply retry delay if specified
	if action.RetryDelay > 0 {
		time.Sleep(action.RetryDelay)
	}

	// Execute the original operation with modifications
	switch errorContext.Operation {
	case "install", "upgrade":
		return h.retryDeployment(ctx, errorContext)
	case "uninstall":
		return h.retryUninstall(ctx, errorContext)
	default:
		return false, fmt.Errorf("retry not supported for operation: %s", errorContext.Operation)
	}
}

// executeRollbackAction executes a rollback recovery action
func (h *HelmErrorHandler) executeRollbackAction(ctx context.Context, action *domain.RecoveryAction, errorContext *domain.ErrorContext) (bool, error) {
	h.logger.DebugWithContext("executing rollback action", map[string]interface{}{
		"release_name": errorContext.Release,
		"namespace":    errorContext.Namespace,
	})

	request := &domain.HelmRollbackRequest{
		ReleaseName: errorContext.Release,
		Namespace:   errorContext.Namespace,
		Revision:    1, // Rollback to revision 1
		Wait:        true,
		Timeout:     5 * time.Minute,
	}

	err := h.helmPort.RollbackChart(ctx, request)
	return err == nil, err
}

// executeForceAction executes a force recovery action
func (h *HelmErrorHandler) executeForceAction(ctx context.Context, action *domain.RecoveryAction, errorContext *domain.ErrorContext) (bool, error) {
	h.logger.DebugWithContext("executing force action", map[string]interface{}{
		"release_name": errorContext.Release,
		"namespace":    errorContext.Namespace,
	})

	// This would typically re-execute the original operation with --force flag
	// Implementation depends on the specific operation
	return false, fmt.Errorf("force action not yet implemented for operation: %s", errorContext.Operation)
}

// executeCleanupAction executes a cleanup recovery action
func (h *HelmErrorHandler) executeCleanupAction(ctx context.Context, action *domain.RecoveryAction, errorContext *domain.ErrorContext) (bool, error) {
	h.logger.DebugWithContext("executing cleanup action", map[string]interface{}{
		"release_name": errorContext.Release,
		"namespace":    errorContext.Namespace,
	})

	// This would typically clean up failed resources
	// Implementation depends on the specific error type and resources involved
	return false, fmt.Errorf("cleanup action requires manual intervention")
}

// retryDeployment retries a failed deployment with potential modifications
func (h *HelmErrorHandler) retryDeployment(ctx context.Context, errorContext *domain.ErrorContext) (bool, error) {
	h.logger.DebugWithContext("retrying deployment", map[string]interface{}{
		"release_name": errorContext.Release,
		"namespace":    errorContext.Namespace,
	})

	// This would re-execute the deployment with the original parameters
	// Potentially with modifications based on the error type
	return false, fmt.Errorf("deployment retry not yet implemented")
}

// retryUninstall retries a failed uninstall operation
func (h *HelmErrorHandler) retryUninstall(ctx context.Context, errorContext *domain.ErrorContext) (bool, error) {
	h.logger.DebugWithContext("retrying uninstall", map[string]interface{}{
		"release_name": errorContext.Release,
		"namespace":    errorContext.Namespace,
	})

	request := &domain.HelmUndeploymentRequest{
		ReleaseName: errorContext.Release,
		Namespace:   errorContext.Namespace,
		KeepHistory: false,
		Wait:        true,
		Timeout:     5 * time.Minute,
	}

	err := h.helmPort.UninstallChart(ctx, request)
	return err == nil, err
}