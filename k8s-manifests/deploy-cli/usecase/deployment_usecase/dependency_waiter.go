package deployment_usecase

import (
	"context"
	"fmt"
	"time"

	"deploy-cli/domain"
	"deploy-cli/port/logger_port"
)

// DependencyWaiter handles waiting for service dependencies to be ready
type DependencyWaiter struct {
	healthChecker *HealthChecker
	logger        logger_port.LoggerPort
}

// NewDependencyWaiter creates a new dependency waiter
func NewDependencyWaiter(healthChecker *HealthChecker, logger logger_port.LoggerPort) *DependencyWaiter {
	return &DependencyWaiter{
		healthChecker: healthChecker,
		logger:        logger,
	}
}

// WaitForDependencies waits for all dependencies of a service to be ready
func (d *DependencyWaiter) WaitForDependencies(ctx context.Context, serviceName string) error {
	dependencies := domain.GetServiceDependencies(serviceName)
	
	if len(dependencies) == 0 {
		d.logger.Debug("no dependencies found for service",
			"service", serviceName,
		)
		return nil
	}

	d.logger.Info("waiting for service dependencies",
		"service", serviceName,
		"dependency_count", len(dependencies),
	)

	// Wait for required dependencies first
	requiredDeps := domain.GetRequiredDependencies(serviceName)
	for _, dep := range requiredDeps {
		if err := d.waitForSingleDependency(ctx, serviceName, dep); err != nil {
			return fmt.Errorf("required dependency %s not ready: %w", dep.ServiceName, err)
		}
	}

	// Wait for optional dependencies (don't fail if they're not ready)
	optionalDeps := domain.GetOptionalDependencies(serviceName)
	for _, dep := range optionalDeps {
		if err := d.waitForSingleDependency(ctx, serviceName, dep); err != nil {
			d.logger.Warn("optional dependency not ready, continuing",
				"service", serviceName,
				"dependency", dep.ServiceName,
				"error", err,
			)
		}
	}

	d.logger.Info("all dependencies ready for service",
		"service", serviceName,
	)

	return nil
}

// waitForSingleDependency waits for a single dependency to be ready
func (d *DependencyWaiter) waitForSingleDependency(ctx context.Context, serviceName string, dep domain.ServiceDependency) error {
	d.logger.Info("waiting for dependency",
		"service", serviceName,
		"dependency", dep.ServiceName,
		"type", dep.ServiceType,
		"namespace", dep.Namespace,
		"required", dep.Required,
		"timeout", dep.Timeout,
	)

	// Create a timeout context for this dependency
	timeoutCtx, cancel := context.WithTimeout(ctx, time.Duration(dep.Timeout)*time.Second)
	defer cancel()

	// Wait for the dependency to be ready
	if err := d.healthChecker.WaitForServiceReady(timeoutCtx, dep.ServiceName, dep.ServiceType, dep.Namespace); err != nil {
		return fmt.Errorf("dependency %s in namespace %s not ready: %w", dep.ServiceName, dep.Namespace, err)
	}

	d.logger.Info("dependency ready",
		"service", serviceName,
		"dependency", dep.ServiceName,
		"type", dep.ServiceType,
		"namespace", dep.Namespace,
	)

	return nil
}

// ValidateDependencies validates that all dependencies are currently ready (without waiting)
func (d *DependencyWaiter) ValidateDependencies(ctx context.Context, serviceName string) error {
	dependencies := domain.GetServiceDependencies(serviceName)
	
	if len(dependencies) == 0 {
		return nil
	}

	d.logger.Info("validating service dependencies",
		"service", serviceName,
		"dependency_count", len(dependencies),
	)

	// Check all required dependencies
	requiredDeps := domain.GetRequiredDependencies(serviceName)
	for _, dep := range requiredDeps {
		// Create a short timeout context for validation
		validationCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		
		if err := d.healthChecker.WaitForServiceReady(validationCtx, dep.ServiceName, dep.ServiceType, dep.Namespace); err != nil {
			cancel()
			return fmt.Errorf("required dependency %s not ready: %w", dep.ServiceName, err)
		}
		cancel()
	}

	d.logger.Info("all required dependencies validated",
		"service", serviceName,
	)

	return nil
}

// GetDependencyStatus returns the status of all dependencies for a service
func (d *DependencyWaiter) GetDependencyStatus(ctx context.Context, serviceName string) (map[string]bool, error) {
	dependencies := domain.GetServiceDependencies(serviceName)
	status := make(map[string]bool)

	for _, dep := range dependencies {
		// Create a short timeout context for status check
		statusCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		
		err := d.healthChecker.WaitForServiceReady(statusCtx, dep.ServiceName, dep.ServiceType, dep.Namespace)
		status[dep.ServiceName] = (err == nil)
		
		cancel()
	}

	return status, nil
}