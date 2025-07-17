package deployment_usecase

import (
	"context"
	"fmt"
	"sync"
	"time"

	"deploy-cli/domain"
	"deploy-cli/port/logger_port"
)

// DependencyFailureDetector proactively detects and reports dependency failures
type DependencyFailureDetector struct {
	logger               logger_port.LoggerPort
	dependencyGraph      map[string][]string // chart -> dependencies
	dependencyStatus     map[string]DependencyStatus
	dependencyChecks     map[string][]DependencyCheck
	metricsCollector     *MetricsCollector
	layerMonitor         *LayerHealthMonitor
	mutex                sync.RWMutex
	alertThresholds      *DependencyAlertThresholds
}

// DependencyStatus represents the status of a dependency
type DependencyStatus struct {
	Name              string
	Status            string
	LastCheck         time.Time
	LastSuccess       time.Time
	FailureCount      int
	ConsecutiveFailures int
	AverageResponseTime time.Duration
	ErrorHistory      []DependencyError
}

// DependencyCheck represents a dependency check
type DependencyCheck struct {
	CheckType     string
	Target        string
	Status        string
	Duration      time.Duration
	Timestamp     time.Time
	ErrorMessage  string
	Context       map[string]interface{}
}

// DependencyError represents a dependency error
type DependencyError struct {
	Timestamp    time.Time
	ErrorMessage string
	ErrorType    string
	Severity     domain.ErrorSeverity
	Context      map[string]interface{}
}

// DependencyAlertThresholds defines thresholds for dependency alerts
type DependencyAlertThresholds struct {
	MaxConsecutiveFailures int
	MaxResponseTime        time.Duration
	FailureRateThreshold   float64
	CheckInterval          time.Duration
}

// NewDependencyFailureDetector creates a new dependency failure detector
func NewDependencyFailureDetector(logger logger_port.LoggerPort, metricsCollector *MetricsCollector, layerMonitor *LayerHealthMonitor) *DependencyFailureDetector {
	return &DependencyFailureDetector{
		logger:           logger,
		dependencyGraph:  make(map[string][]string),
		dependencyStatus: make(map[string]DependencyStatus),
		dependencyChecks: make(map[string][]DependencyCheck),
		metricsCollector: metricsCollector,
		layerMonitor:     layerMonitor,
		alertThresholds:  getDefaultDependencyAlertThresholds(),
	}
}

// getDefaultDependencyAlertThresholds returns default dependency alert thresholds
func getDefaultDependencyAlertThresholds() *DependencyAlertThresholds {
	return &DependencyAlertThresholds{
		MaxConsecutiveFailures: 3,
		MaxResponseTime:        30 * time.Second,
		FailureRateThreshold:   0.2, // 20%
		CheckInterval:          1 * time.Minute,
	}
}

// RegisterDependency registers a dependency relationship
func (d *DependencyFailureDetector) RegisterDependency(chart string, dependencies []string) error {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	d.dependencyGraph[chart] = dependencies

	// Initialize dependency status for new dependencies
	for _, dep := range dependencies {
		if _, exists := d.dependencyStatus[dep]; !exists {
			d.dependencyStatus[dep] = DependencyStatus{
				Name:                dep,
				Status:              "unknown",
				LastCheck:           time.Time{},
				LastSuccess:         time.Time{},
				FailureCount:        0,
				ConsecutiveFailures: 0,
				AverageResponseTime: 0,
				ErrorHistory:        make([]DependencyError, 0),
			}
			d.dependencyChecks[dep] = make([]DependencyCheck, 0)
		}
	}

	d.logger.InfoWithContext("registered dependency relationship", map[string]interface{}{
		"chart":        chart,
		"dependencies": dependencies,
	})

	return nil
}

// StartDependencyMonitoring starts monitoring dependencies for a deployment
func (d *DependencyFailureDetector) StartDependencyMonitoring(ctx context.Context, deploymentID string) error {
	d.logger.InfoWithContext("starting dependency monitoring", map[string]interface{}{
		"deployment_id": deploymentID,
	})

	// Start monitoring goroutine
	go d.monitorDependencies(ctx, deploymentID)

	return nil
}

// CheckDependency checks the health of a specific dependency
func (d *DependencyFailureDetector) CheckDependency(dependencyName string, checkType string) (*DependencyCheck, error) {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	startTime := time.Now()
	check := DependencyCheck{
		CheckType: checkType,
		Target:    dependencyName,
		Timestamp: startTime,
		Context:   make(map[string]interface{}),
	}

	// Perform the actual dependency check based on type
	switch checkType {
	case "database":
		check = d.checkDatabaseDependency(dependencyName, check)
	case "service":
		check = d.checkServiceDependency(dependencyName, check)
	case "storage":
		check = d.checkStorageDependency(dependencyName, check)
	case "network":
		check = d.checkNetworkDependency(dependencyName, check)
	default:
		check.Status = "unknown"
		check.ErrorMessage = fmt.Sprintf("Unknown check type: %s", checkType)
	}

	check.Duration = time.Since(startTime)

	// Update dependency status
	d.updateDependencyStatus(dependencyName, check)

	// Record the check
	d.dependencyChecks[dependencyName] = append(d.dependencyChecks[dependencyName], check)

	// Keep only last 100 checks
	if len(d.dependencyChecks[dependencyName]) > 100 {
		d.dependencyChecks[dependencyName] = d.dependencyChecks[dependencyName][len(d.dependencyChecks[dependencyName])-100:]
	}

	d.logger.InfoWithContext("dependency check completed", map[string]interface{}{
		"dependency": dependencyName,
		"check_type": checkType,
		"status":     check.Status,
		"duration":   check.Duration,
	})

	return &check, nil
}

// checkDatabaseDependency checks database dependencies
func (d *DependencyFailureDetector) checkDatabaseDependency(dependencyName string, check DependencyCheck) DependencyCheck {
	// This is a placeholder - in a real implementation, you would:
	// 1. Connect to the database
	// 2. Execute a simple query
	// 3. Check response time
	// 4. Verify database accessibility

	check.Status = "success"
	check.Context["type"] = "database"
	check.Context["simulated"] = true

	return check
}

// checkServiceDependency checks service dependencies
func (d *DependencyFailureDetector) checkServiceDependency(dependencyName string, check DependencyCheck) DependencyCheck {
	// This is a placeholder - in a real implementation, you would:
	// 1. Check service health endpoint
	// 2. Verify service is running
	// 3. Check service response time
	// 4. Validate service functionality

	check.Status = "success"
	check.Context["type"] = "service"
	check.Context["simulated"] = true

	return check
}

// checkStorageDependency checks storage dependencies
func (d *DependencyFailureDetector) checkStorageDependency(dependencyName string, check DependencyCheck) DependencyCheck {
	// This is a placeholder - in a real implementation, you would:
	// 1. Check storage availability
	// 2. Verify read/write permissions
	// 3. Check storage capacity
	// 4. Validate storage performance

	check.Status = "success"
	check.Context["type"] = "storage"
	check.Context["simulated"] = true

	return check
}

// checkNetworkDependency checks network dependencies
func (d *DependencyFailureDetector) checkNetworkDependency(dependencyName string, check DependencyCheck) DependencyCheck {
	// This is a placeholder - in a real implementation, you would:
	// 1. Check network connectivity
	// 2. Verify DNS resolution
	// 3. Test network latency
	// 4. Validate network routes

	check.Status = "success"
	check.Context["type"] = "network"
	check.Context["simulated"] = true

	return check
}

// updateDependencyStatus updates the status of a dependency based on a check
func (d *DependencyFailureDetector) updateDependencyStatus(dependencyName string, check DependencyCheck) {
	status := d.dependencyStatus[dependencyName]
	status.LastCheck = check.Timestamp

	if check.Status == "success" {
		status.Status = "healthy"
		status.LastSuccess = check.Timestamp
		status.ConsecutiveFailures = 0
	} else {
		status.Status = "unhealthy"
		status.FailureCount++
		status.ConsecutiveFailures++

		// Record error
		error := DependencyError{
			Timestamp:    check.Timestamp,
			ErrorMessage: check.ErrorMessage,
			ErrorType:    check.CheckType,
			Severity:     d.determineDependencyErrorSeverity(status.ConsecutiveFailures),
			Context:      check.Context,
		}
		status.ErrorHistory = append(status.ErrorHistory, error)

		// Keep only last 50 errors
		if len(status.ErrorHistory) > 50 {
			status.ErrorHistory = status.ErrorHistory[len(status.ErrorHistory)-50:]
		}
	}

	// Update average response time
	d.updateAverageResponseTime(&status, check.Duration)

	d.dependencyStatus[dependencyName] = status

	// Check for alerts
	d.checkDependencyAlerts(dependencyName, status)
}

// updateAverageResponseTime updates the average response time for a dependency
func (d *DependencyFailureDetector) updateAverageResponseTime(status *DependencyStatus, duration time.Duration) {
	if status.AverageResponseTime == 0 {
		status.AverageResponseTime = duration
	} else {
		// Simple moving average with weight 0.8 for existing average
		status.AverageResponseTime = time.Duration(0.8*float64(status.AverageResponseTime) + 0.2*float64(duration))
	}
}

// determineDependencyErrorSeverity determines the severity of a dependency error
func (d *DependencyFailureDetector) determineDependencyErrorSeverity(consecutiveFailures int) domain.ErrorSeverity {
	switch {
	case consecutiveFailures >= 5:
		return domain.ErrorSeverityCritical
	case consecutiveFailures >= 3:
		return domain.ErrorSeverityHigh
	case consecutiveFailures >= 2:
		return domain.ErrorSeverityMedium
	default:
		return domain.ErrorSeverityLow
	}
}

// checkDependencyAlerts checks for dependency-related alerts
func (d *DependencyFailureDetector) checkDependencyAlerts(dependencyName string, status DependencyStatus) {
	// Alert for consecutive failures
	if status.ConsecutiveFailures >= d.alertThresholds.MaxConsecutiveFailures {
		d.logger.WarnWithContext("dependency consecutive failures threshold exceeded", map[string]interface{}{
			"dependency":           dependencyName,
			"consecutive_failures": status.ConsecutiveFailures,
			"threshold":            d.alertThresholds.MaxConsecutiveFailures,
		})
	}

	// Alert for slow response time
	if status.AverageResponseTime > d.alertThresholds.MaxResponseTime {
		d.logger.WarnWithContext("dependency response time threshold exceeded", map[string]interface{}{
			"dependency":           dependencyName,
			"average_response_time": status.AverageResponseTime,
			"threshold":            d.alertThresholds.MaxResponseTime,
		})
	}

	// Alert for high failure rate
	if len(d.dependencyChecks[dependencyName]) >= 10 {
		recentChecks := d.dependencyChecks[dependencyName]
		if len(recentChecks) > 20 {
			recentChecks = recentChecks[len(recentChecks)-20:] // Last 20 checks
		}

		failureCount := 0
		for _, check := range recentChecks {
			if check.Status != "success" {
				failureCount++
			}
		}

		failureRate := float64(failureCount) / float64(len(recentChecks))
		if failureRate > d.alertThresholds.FailureRateThreshold {
			d.logger.WarnWithContext("dependency failure rate threshold exceeded", map[string]interface{}{
				"dependency":    dependencyName,
				"failure_rate":  failureRate,
				"threshold":     d.alertThresholds.FailureRateThreshold,
				"recent_checks": len(recentChecks),
			})
		}
	}
}

// monitorDependencies monitors dependencies in the background
func (d *DependencyFailureDetector) monitorDependencies(ctx context.Context, deploymentID string) {
	ticker := time.NewTicker(d.alertThresholds.CheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			d.logger.InfoWithContext("dependency monitoring stopped due to context cancellation", map[string]interface{}{
				"deployment_id": deploymentID,
			})
			return
		case <-ticker.C:
			d.performPeriodicDependencyChecks(deploymentID)
		}
	}
}

// performPeriodicDependencyChecks performs periodic dependency checks
func (d *DependencyFailureDetector) performPeriodicDependencyChecks(deploymentID string) {
	d.mutex.RLock()
	dependencies := make([]string, 0)
	for dep := range d.dependencyStatus {
		dependencies = append(dependencies, dep)
	}
	d.mutex.RUnlock()

	for _, dep := range dependencies {
		// Determine check type based on dependency name
		checkType := d.determineCheckType(dep)
		
		_, err := d.CheckDependency(dep, checkType)
		if err != nil {
			d.logger.ErrorWithContext("periodic dependency check failed", map[string]interface{}{
				"deployment_id": deploymentID,
				"dependency":    dep,
				"error":         err.Error(),
			})
		}
	}
}

// determineCheckType determines the check type based on dependency name
func (d *DependencyFailureDetector) determineCheckType(dependencyName string) string {
	// Simple heuristic based on dependency name
	switch {
	case containsAny(dependencyName, []string{"postgres", "mysql", "redis", "mongo"}):
		return "database"
	case containsAny(dependencyName, []string{"service", "api", "backend"}):
		return "service"
	case containsAny(dependencyName, []string{"storage", "volume", "disk"}):
		return "storage"
	case containsAny(dependencyName, []string{"network", "dns", "proxy"}):
		return "network"
	default:
		return "service" // Default to service check
	}
}

// containsAny checks if a string contains any of the given substrings
func containsAny(str string, substrings []string) bool {
	for _, substring := range substrings {
		if len(str) >= len(substring) {
			for i := 0; i <= len(str)-len(substring); i++ {
				if str[i:i+len(substring)] == substring {
					return true
				}
			}
		}
	}
	return false
}

// GetDependencyStatus returns the status of a dependency
func (d *DependencyFailureDetector) GetDependencyStatus(dependencyName string) (*DependencyStatus, error) {
	d.mutex.RLock()
	defer d.mutex.RUnlock()

	if status, exists := d.dependencyStatus[dependencyName]; exists {
		return &status, nil
	}

	return nil, fmt.Errorf("dependency not found: %s", dependencyName)
}

// GetDependencyGraph returns the dependency graph
func (d *DependencyFailureDetector) GetDependencyGraph() map[string][]string {
	d.mutex.RLock()
	defer d.mutex.RUnlock()

	// Create a copy to avoid race conditions
	graph := make(map[string][]string)
	for chart, deps := range d.dependencyGraph {
		graph[chart] = make([]string, len(deps))
		copy(graph[chart], deps)
	}

	return graph
}

// GetDependencyChecks returns recent checks for a dependency
func (d *DependencyFailureDetector) GetDependencyChecks(dependencyName string) []DependencyCheck {
	d.mutex.RLock()
	defer d.mutex.RUnlock()

	if checks, exists := d.dependencyChecks[dependencyName]; exists {
		// Return a copy to avoid race conditions
		result := make([]DependencyCheck, len(checks))
		copy(result, checks)
		return result
	}

	return []DependencyCheck{}
}

// GetUnhealthyDependencies returns all unhealthy dependencies
func (d *DependencyFailureDetector) GetUnhealthyDependencies() []string {
	d.mutex.RLock()
	defer d.mutex.RUnlock()

	unhealthy := make([]string, 0)
	for name, status := range d.dependencyStatus {
		if status.Status == "unhealthy" {
			unhealthy = append(unhealthy, name)
		}
	}

	return unhealthy
}

// GetDependencyInsights returns insights about dependencies
func (d *DependencyFailureDetector) GetDependencyInsights() []domain.DeploymentInsight {
	d.mutex.RLock()
	defer d.mutex.RUnlock()

	insights := make([]domain.DeploymentInsight, 0)

	// Generate insights about unhealthy dependencies
	unhealthy := d.GetUnhealthyDependencies()
	if len(unhealthy) > 0 {
		insights = append(insights, domain.DeploymentInsight{
			InsightType: "dependency_health",
			Title:       "Unhealthy Dependencies Detected",
			Description: fmt.Sprintf("Found %d unhealthy dependencies that may impact deployment", len(unhealthy)),
			Metrics: map[string]interface{}{
				"unhealthy_dependencies": unhealthy,
				"total_dependencies":     len(d.dependencyStatus),
			},
			Suggestions: []domain.OptimizationSuggestion{
				{
					Type:        "reliability",
					Component:   "dependencies",
					Description: "Address unhealthy dependencies before proceeding with deployment",
					Impact:      "Improved deployment reliability and reduced failure risk",
					Effort:      "medium",
					Priority:    "high",
					CreatedAt:   time.Now(),
				},
			},
			CreatedAt: time.Now(),
		})
	}

	return insights
}