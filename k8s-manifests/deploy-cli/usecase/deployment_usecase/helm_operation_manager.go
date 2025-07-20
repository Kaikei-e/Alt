package deployment_usecase

import (
	"fmt"
	"sync"
	"time"

	"deploy-cli/port/logger_port"
)

// HelmOperation represents an ongoing helm operation
type HelmOperation struct {
	StartTime time.Time
	Operation string
}

// HelmOperationManager manages concurrent helm operations to prevent conflicts
type HelmOperationManager struct {
	mu               sync.Mutex
	activeOperations map[string]*HelmOperation
	logger           logger_port.LoggerPort
	staleTimeout     time.Duration
}

// NewHelmOperationManager creates a new HelmOperationManager
func NewHelmOperationManager(logger logger_port.LoggerPort) *HelmOperationManager {
	return &HelmOperationManager{
		activeOperations: make(map[string]*HelmOperation),
		logger:           logger,
		staleTimeout:     10 * time.Minute, // Operations older than 10 minutes are considered stale
	}
}

// ExecuteWithLock executes a helm operation with mutual exclusion
func (h *HelmOperationManager) ExecuteWithLock(releaseName, namespace, operation string, operationFunc func() error) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	key := fmt.Sprintf("%s/%s", namespace, releaseName)

	// Check if operation is already in progress
	if activeOp, exists := h.activeOperations[key]; exists {
		// Check if operation is stale
		if time.Since(activeOp.StartTime) < h.staleTimeout {
			h.logger.WarnWithContext("helm operation already in progress", map[string]interface{}{
				"release":    releaseName,
				"namespace":  namespace,
				"operation":  operation,
				"active_op":  activeOp.Operation,
				"started_at": activeOp.StartTime,
				"elapsed":    time.Since(activeOp.StartTime),
			})
			return fmt.Errorf("helm operation already in progress for %s (operation: %s, started: %v)",
				key, activeOp.Operation, activeOp.StartTime)
		}
		// Clean up stale operation
		h.logger.InfoWithContext("cleaning up stale helm operation", map[string]interface{}{
			"release":    releaseName,
			"namespace":  namespace,
			"stale_op":   activeOp.Operation,
			"started_at": activeOp.StartTime,
			"elapsed":    time.Since(activeOp.StartTime),
		})
		delete(h.activeOperations, key)
	}

	// Register the operation
	h.activeOperations[key] = &HelmOperation{
		StartTime: time.Now(),
		Operation: operation,
	}

	h.logger.InfoWithContext("starting helm operation", map[string]interface{}{
		"release":   releaseName,
		"namespace": namespace,
		"operation": operation,
	})

	// Clean up operation when done (mutex already held by outer function)
	defer func() {
		delete(h.activeOperations, key)
		h.logger.InfoWithContext("completed helm operation", map[string]interface{}{
			"release":   releaseName,
			"namespace": namespace,
			"operation": operation,
		})
	}()

	return operationFunc()
}

// IsOperationInProgress checks if an operation is in progress for the given release
func (h *HelmOperationManager) IsOperationInProgress(releaseName, namespace string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()

	key := fmt.Sprintf("%s/%s", namespace, releaseName)
	activeOp, exists := h.activeOperations[key]

	if !exists {
		return false
	}

	// Check if operation is stale
	if time.Since(activeOp.StartTime) >= h.staleTimeout {
		// Clean up stale operation
		delete(h.activeOperations, key)
		return false
	}

	return true
}

// GetActiveOperations returns a copy of currently active operations
func (h *HelmOperationManager) GetActiveOperations() map[string]*HelmOperation {
	h.mu.Lock()
	defer h.mu.Unlock()

	operations := make(map[string]*HelmOperation)
	for key, op := range h.activeOperations {
		operations[key] = &HelmOperation{
			StartTime: op.StartTime,
			Operation: op.Operation,
		}
	}
	return operations
}

// CleanupStaleOperations removes operations that have been running for too long
func (h *HelmOperationManager) CleanupStaleOperations() int {
	h.mu.Lock()
	defer h.mu.Unlock()

	var cleaned int
	for key, op := range h.activeOperations {
		if time.Since(op.StartTime) >= h.staleTimeout {
			h.logger.WarnWithContext("cleaning up stale helm operation", map[string]interface{}{
				"key":        key,
				"operation":  op.Operation,
				"started_at": op.StartTime,
				"elapsed":    time.Since(op.StartTime),
			})
			delete(h.activeOperations, key)
			cleaned++
		}
	}

	if cleaned > 0 {
		h.logger.InfoWithContext("cleaned up stale helm operations", map[string]interface{}{
			"cleaned_count": cleaned,
		})
	}

	return cleaned
}
