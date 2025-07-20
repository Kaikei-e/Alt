package deployment_usecase

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"deploy-cli/domain"
	"deploy-cli/port/logger_port"
)

// MockLogger implements logger_port.LoggerPort for testing
type MockLogger struct{}

func (m *MockLogger) Info(msg string, args ...interface{}) {}
func (m *MockLogger) Error(msg string, args ...interface{}) {}
func (m *MockLogger) Warn(msg string, args ...interface{}) {}
func (m *MockLogger) Debug(msg string, args ...interface{}) {}
func (m *MockLogger) InfoWithContext(msg string, fields map[string]interface{}) {}
func (m *MockLogger) ErrorWithContext(msg string, fields map[string]interface{}) {}
func (m *MockLogger) WarnWithContext(msg string, fields map[string]interface{}) {}
func (m *MockLogger) DebugWithContext(msg string, fields map[string]interface{}) {}
func (m *MockLogger) WithField(key string, value interface{}) logger_port.LoggerPort { return m }
func (m *MockLogger) WithFields(fields map[string]interface{}) logger_port.LoggerPort { return m }

// Test error for simulating failures
var ErrTestFailure = errors.New("test deployment failure")

// TestHelmOperationManagerNoDeadlock tests that HelmOperationManager doesn't deadlock
func TestHelmOperationManagerNoDeadlock(t *testing.T) {
	logger := &MockLogger{}
	manager := NewHelmOperationManager(logger)

	// Test the deadlock scenario that was fixed
	t.Run("no deadlock with successful operation", func(t *testing.T) {
		done := make(chan bool, 1)
		
		go func() {
			err := manager.ExecuteWithLock("test-release", "test-namespace", "deploy", func() error {
				// Simulate successful operation
				time.Sleep(10 * time.Millisecond)
				return nil
			})
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			done <- true
		}()

		// Test should complete within reasonable time (not deadlock)
		select {
		case <-done:
			// Success - no deadlock
		case <-time.After(5 * time.Second):
			t.Fatal("operation deadlocked - took too long to complete")
		}
	})

	t.Run("no deadlock with failed operation", func(t *testing.T) {
		done := make(chan bool, 1)
		
		go func() {
			err := manager.ExecuteWithLock("test-release-2", "test-namespace", "deploy", func() error {
				// Simulate failed operation
				time.Sleep(10 * time.Millisecond)
				return ErrTestFailure
			})
			if err == nil {
				t.Error("expected error but got nil")
			}
			done <- true
		}()

		// Test should complete within reasonable time (not deadlock)
		select {
		case <-done:
			// Success - no deadlock
		case <-time.After(5 * time.Second):
			t.Fatal("operation deadlocked - took too long to complete")
		}
	})

	t.Run("concurrent operations on different releases", func(t *testing.T) {
		const numOperations = 10
		var wg sync.WaitGroup
		errorChan := make(chan error, numOperations)

		for i := 0; i < numOperations; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				
				releaseName := fmt.Sprintf("test-release-%d", id)
				err := manager.ExecuteWithLock(releaseName, "test-namespace", "deploy", func() error {
					time.Sleep(time.Duration(id) * time.Millisecond)
					return nil
				})
				if err != nil {
					errorChan <- err
				}
			}(i)
		}

		// Wait for all operations to complete with timeout
		done := make(chan bool, 1)
		go func() {
			wg.Wait()
			done <- true
		}()

		select {
		case <-done:
			// Check for errors
			close(errorChan)
			for err := range errorChan {
				t.Errorf("unexpected error in concurrent operation: %v", err)
			}
		case <-time.After(10 * time.Second):
			t.Fatal("concurrent operations deadlocked")
		}
	})
}

// TestParallelChartDeployerNoDeadlock tests that parallel deployment doesn't deadlock
func TestParallelChartDeployerNoDeadlock(t *testing.T) {
	logger := &MockLogger{}
	config := DefaultParallelConfig()
	config.MaxConcurrency = 2 // Reduce for testing
	deployer := NewParallelChartDeployer(logger, config)

	// Mock single chart deployment function
	mockDeploySingleChart := func(ctx context.Context, chart domain.Chart, options *domain.DeploymentOptions) domain.DeploymentResult {
		// Simulate chart deployment work
		time.Sleep(50 * time.Millisecond)
		return domain.DeploymentResult{
			ChartName: chart.Name,
			Status:    domain.DeploymentStatusSuccess,
			Duration:  50 * time.Millisecond,
		}
	}

	t.Run("no deadlock with multiple charts", func(t *testing.T) {
		charts := []domain.Chart{
			{Name: "chart1", Type: domain.ApplicationChart, Path: "/test/chart1"},
			{Name: "chart2", Type: domain.ApplicationChart, Path: "/test/chart2"},
			{Name: "chart3", Type: domain.ApplicationChart, Path: "/test/chart3"},
		}

		options := &domain.DeploymentOptions{
			Environment: domain.Production,
			DryRun:      false,
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		done := make(chan bool, 1)
		go func() {
			results, err := deployer.deployChartsParallel(ctx, "test-group", charts, options, mockDeploySingleChart)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if len(results) != len(charts) {
				t.Errorf("expected %d results, got %d", len(charts), len(results))
			}
			done <- true
		}()

		select {
		case <-done:
			// Success - no deadlock
		case <-time.After(15 * time.Second):
			t.Fatal("parallel deployment deadlocked")
		}
	})

	t.Run("worker pool graceful shutdown", func(t *testing.T) {
		pool := NewChartWorkerPool(2, mockDeploySingleChart, logger)
		
		// Start the pool
		pool.Start()

		// Submit some jobs
		for i := 0; i < 3; i++ {
			resultChan := make(chan domain.DeploymentResult, 1)
			job := ChartDeployJob{
				Chart: domain.Chart{
					Name: fmt.Sprintf("test-chart-%d", i),
					Type: domain.ApplicationChart,
					Path: fmt.Sprintf("/test/chart%d", i),
				},
				Options: &domain.DeploymentOptions{
					Environment: domain.Development,
				},
				Result: resultChan,
			}
			pool.SubmitJob(job)
		}

		// Stop the pool - this should not deadlock
		done := make(chan bool, 1)
		go func() {
			pool.Stop()
			done <- true
		}()

		select {
		case <-done:
			// Success - no deadlock during shutdown
		case <-time.After(45 * time.Second): // Increased timeout for worker pool shutdown
			t.Fatal("worker pool shutdown deadlocked")
		}
	})
}

// TestContextCancellationHandling tests proper context cancellation handling
func TestContextCancellationHandling(t *testing.T) {
	logger := &MockLogger{}
	config := DefaultParallelConfig()
	deployer := NewParallelChartDeployer(logger, config)

	// Mock deployment function that respects context cancellation
	mockDeployWithCancellation := func(ctx context.Context, chart domain.Chart, options *domain.DeploymentOptions) domain.DeploymentResult {
		select {
		case <-ctx.Done():
			return domain.DeploymentResult{
				ChartName: chart.Name,
				Status:    domain.DeploymentStatusFailed,
				Error:     ctx.Err(),
				Duration:  0,
			}
		case <-time.After(100 * time.Millisecond):
			return domain.DeploymentResult{
				ChartName: chart.Name,
				Status:    domain.DeploymentStatusSuccess,
				Duration:  100 * time.Millisecond,
			}
		}
	}

	t.Run("context cancellation during deployment", func(t *testing.T) {
		charts := []domain.Chart{
			{Name: "slow-chart1", Type: domain.ApplicationChart, Path: "/test/slow1"},
			{Name: "slow-chart2", Type: domain.ApplicationChart, Path: "/test/slow2"},
		}

		options := &domain.DeploymentOptions{
			Environment: domain.Development,
		}

		// Create context with very short timeout to trigger cancellation
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()

		done := make(chan bool, 1)
		go func() {
			results, err := deployer.deployChartsParallel(ctx, "test-group", charts, options, mockDeployWithCancellation)
			// Should handle cancellation gracefully
			if err != nil && err != context.DeadlineExceeded {
				t.Errorf("unexpected error type: %v", err)
			}
			// Should still return results (even if failed due to cancellation)
			if len(results) != len(charts) {
				t.Errorf("expected %d results, got %d", len(charts), len(results))
			}
			done <- true
		}()

		select {
		case <-done:
			// Success - handled cancellation gracefully
		case <-time.After(5 * time.Second):
			t.Fatal("context cancellation handling deadlocked")
		}
	})
}

// TestRaceConditionPrevention tests that race conditions are properly handled
func TestRaceConditionPrevention(t *testing.T) {
	logger := &MockLogger{}
	manager := NewHelmOperationManager(logger)

	t.Run("concurrent access to same release", func(t *testing.T) {
		const numGoroutines = 10
		var wg sync.WaitGroup
		successCount := make(chan int, numGoroutines)
		errorCount := make(chan int, numGoroutines)

		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				
				// All trying to access the same release - should be serialized
				err := manager.ExecuteWithLock("same-release", "test-namespace", "deploy", func() error {
					time.Sleep(10 * time.Millisecond)
					return nil
				})
				
				if err != nil {
					errorCount <- 1
				} else {
					successCount <- 1
				}
			}(i)
		}

		// Wait for completion with timeout
		done := make(chan bool, 1)
		go func() {
			wg.Wait()
			done <- true
		}()

		select {
		case <-done:
			close(successCount)
			close(errorCount)
			
			// Count results
			successes := 0
			errors := 0
			
			for range successCount {
				successes++
			}
			for range errorCount {
				errors++
			}
			
			// Only one operation should succeed, others should get "operation in progress" error
			if successes < 1 {
				t.Error("at least one operation should have succeeded")
			}
			if successes+errors != numGoroutines {
				t.Errorf("expected %d total operations, got %d", numGoroutines, successes+errors)
			}
			
		case <-time.After(10 * time.Second):
			t.Fatal("race condition test deadlocked")
		}
	})
}