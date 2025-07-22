// PHASE R2: Integration tests for refactored Helm gateway
package helm_gateway

import (
	"context"
	"testing"
	"time"

	"deploy-cli/domain"
	"deploy-cli/gateway/helm_gateway/core"
	"deploy-cli/gateway/helm_gateway/management"
	"deploy-cli/gateway/helm_gateway/error_handling"
	"deploy-cli/mocks"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRefactoredHelmGateway_Integration(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHelmPort := mocks.NewMockHelmPort(ctrl)
	mockLogger := mocks.NewMockLoggerPort(ctrl)

	// Setup mock expectations for initialization
	mockLogger.EXPECT().InfoWithContext(gomock.Any(), gomock.Any()).AnyTimes()
	mockLogger.EXPECT().DebugWithContext(gomock.Any(), gomock.Any()).AnyTimes()
	mockLogger.EXPECT().ErrorWithContext(gomock.Any(), gomock.Any()).AnyTimes()
	mockLogger.EXPECT().WarnWithContext(gomock.Any(), gomock.Any()).AnyTimes()

	gateway := NewRefactoredHelmGateway(mockHelmPort, mockLogger)

	ctx := context.Background()
	testChart := domain.Chart{
		Name:    "test-chart",
		Path:    "/path/to/test-chart",
		Version: "1.0.0",
		Type:    "application",
		Values: map[string]interface{}{
			"image": map[string]interface{}{
				"repository": "nginx",
				"tag":        "latest",
			},
		},
	}

	testOptions := &domain.DeploymentOptions{
		Strategy:        "rolling-update",
		DeployTimeout:   5 * time.Minute,
		CreateNamespace: true,
		DryRun:          false,
	}

	t.Run("DeployChart Integration", func(t *testing.T) {
		// Mock deployment request
		mockHelmPort.EXPECT().
			InstallChart(ctx, gomock.Any()).
			Return(&domain.HelmDeploymentResult{
				ReleaseName: "test-chart",
				Namespace:   "default",
				Revision:    1,
				Status:      "deployed",
				Duration:    30 * time.Second,
			}, nil)

		err := gateway.DeployChart(ctx, testChart, testOptions)
		assert.NoError(t, err)
	})

	t.Run("Template Operations Integration", func(t *testing.T) {
		templateOptions := &domain.TemplateOptions{
			ReleaseName: "test-chart",
			Namespace:   "default",
			Values:      testChart.Values,
			Validate:    true,
		}

		// Mock template rendering
		mockHelmPort.EXPECT().
			RenderTemplate(ctx, gomock.Any()).
			Return(&domain.TemplateResult{
				Manifests: map[string]string{
					"deployment.yaml": "apiVersion: apps/v1\nkind: Deployment",
					"service.yaml":    "apiVersion: v1\nkind: Service",
				},
				Notes: "Test notes",
			}, nil)

		result, err := gateway.RenderTemplate(ctx, testChart, templateOptions)
		require.NoError(t, err)
		assert.Equal(t, 2, len(result.Manifests))
		assert.Contains(t, result.Manifests, "deployment.yaml")
		assert.Contains(t, result.Manifests, "service.yaml")
	})

	t.Run("Release Management Integration", func(t *testing.T) {
		// Mock release listing
		mockHelmPort.EXPECT().
			ListReleases(ctx, gomock.Any()).
			Return([]*domain.ReleaseInfo{
				{
					Name:      "test-chart",
					Namespace: "default",
					Revision:  1,
					Status:    "deployed",
					Chart:     "test-chart-1.0.0",
					Updated:   time.Now(),
				},
			}, nil)

		releases, err := gateway.ListReleases(ctx, "default")
		require.NoError(t, err)
		assert.Equal(t, 1, len(releases))
		assert.Equal(t, "test-chart", releases[0].Name)
	})

	t.Run("Chart Validation Integration", func(t *testing.T) {
		result, err := gateway.ValidateChart(ctx, testChart)
		require.NoError(t, err)
		assert.NotNil(t, result)
		// Validation should pass for well-formed test chart
	})

	t.Run("Error Handling Integration", func(t *testing.T) {
		testError := fmt.Errorf("release test-chart not found")
		
		classification, err := gateway.ClassifyError(ctx, testError, "deploy")
		require.NoError(t, err)
		assert.Equal(t, "release_not_found", classification.Type)
		assert.Equal(t, domain.ErrorCategoryConfiguration, classification.Category)
		assert.True(t, classification.Recoverable)

		actions, err := gateway.SuggestRecoveryActions(ctx, classification)
		require.NoError(t, err)
		assert.Greater(t, len(actions), 0)
		assert.Equal(t, domain.RecoveryActionTypeRetry, actions[0].Type)
	})
}

func TestRefactoredHelmGateway_ComponentIntegration(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHelmPort := mocks.NewMockHelmPort(ctrl)
	mockLogger := mocks.NewMockLoggerPort(ctrl)

	// Setup logging mocks
	mockLogger.EXPECT().InfoWithContext(gomock.Any(), gomock.Any()).AnyTimes()
	mockLogger.EXPECT().DebugWithContext(gomock.Any(), gomock.Any()).AnyTimes()
	mockLogger.EXPECT().ErrorWithContext(gomock.Any(), gomock.Any()).AnyTimes()
	mockLogger.EXPECT().WarnWithContext(gomock.Any(), gomock.Any()).AnyTimes()

	t.Run("Core Components Integration", func(t *testing.T) {
		deployment := core.NewHelmDeploymentGateway(mockHelmPort, mockLogger)
		template := core.NewHelmTemplateGateway(mockHelmPort, mockLogger)

		assert.NotNil(t, deployment)
		assert.NotNil(t, template)

		// Test that components can work together
		ctx := context.Background()
		testChart := domain.Chart{
			Name:    "integration-test",
			Path:    "/path/to/chart",
			Version: "1.0.0",
		}

		// Mock status check for deployment component
		mockHelmPort.EXPECT().
			GetReleaseStatus(ctx, "integration-test", "default").
			Return(&domain.ChartStatus{
				Status:      "deployed",
				Revision:    1,
				LastUpdated: time.Now(),
			}, nil)

		status, err := deployment.GetDeploymentStatus(ctx, "integration-test", "default")
		assert.NoError(t, err)
		assert.Equal(t, "deployed", status.Status)
	})

	t.Run("Management Components Integration", func(t *testing.T) {
		validation := management.NewHelmValidationGateway(mockHelmPort, mockLogger)
		releaseManager := management.NewHelmReleaseManager(mockHelmPort, mockLogger)
		metadataManager := management.NewHelmMetadataManager(mockHelmPort, mockLogger)

		assert.NotNil(t, validation)
		assert.NotNil(t, releaseManager)
		assert.NotNil(t, metadataManager)

		// Test component interactions
		ctx := context.Background()
		
		// Mock metadata retrieval
		mockHelmPort.EXPECT().
			GetChartMetadata(ctx, gomock.Any()).
			Return(&domain.ChartMetadata{
				Name:       "test-chart",
				Version:    "1.0.0",
				APIVersion: "v2",
				Type:       "application",
			}, nil)

		metadata, err := metadataManager.GetChartMetadata(ctx, "/path/to/chart")
		assert.NoError(t, err)
		assert.Equal(t, "test-chart", metadata.Name)
	})

	t.Run("Error Handling Component Integration", func(t *testing.T) {
		errorHandler := error_handling.NewHelmErrorHandler(mockHelmPort, mockLogger)
		assert.NotNil(t, errorHandler)

		ctx := context.Background()
		testError := fmt.Errorf("timeout: context deadline exceeded")

		classification, err := errorHandler.ClassifyError(ctx, testError, "deploy")
		assert.NoError(t, err)
		assert.Equal(t, "timeout_error", classification.Type)
		assert.Equal(t, domain.ErrorCategoryTimeout, classification.Category)
	})
}

func TestRefactoredHelmGateway_BackwardCompatibility(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHelmPort := mocks.NewMockHelmPort(ctrl)
	mockLogger := mocks.NewMockLoggerPort(ctrl)

	// Setup logging mocks
	mockLogger.EXPECT().InfoWithContext(gomock.Any(), gomock.Any()).AnyTimes()
	mockLogger.EXPECT().DebugWithContext(gomock.Any(), gomock.Any()).AnyTimes()
	mockLogger.EXPECT().ErrorWithContext(gomock.Any(), gomock.Any()).AnyTimes()
	mockLogger.EXPECT().WarnWithContext(gomock.Any(), gomock.Any()).AnyTimes()

	gateway := NewRefactoredHelmGateway(mockHelmPort, mockLogger)

	// Ensure the gateway implements the legacy interface
	var _ LegacyHelmGateway = gateway

	ctx := context.Background()
	testChart := domain.Chart{
		Name:    "compatibility-test",
		Path:    "/path/to/chart",
		Version: "1.0.0",
	}

	t.Run("Legacy DeployChart Method", func(t *testing.T) {
		mockHelmPort.EXPECT().
			InstallChart(ctx, gomock.Any()).
			Return(&domain.HelmDeploymentResult{
				ReleaseName: "compatibility-test",
				Namespace:   "default",
				Revision:    1,
				Status:      "deployed",
			}, nil)

		testOptions := &domain.DeploymentOptions{
			Strategy:      "rolling-update",
			DeployTimeout: 5 * time.Minute,
		}

		err := gateway.DeployChart(ctx, testChart, testOptions)
		assert.NoError(t, err)
	})

	t.Run("Legacy ListReleases Method", func(t *testing.T) {
		mockHelmPort.EXPECT().
			ListReleases(ctx, gomock.Any()).
			Return([]*domain.ReleaseInfo{
				{
					Name:      "compatibility-test",
					Namespace: "default",
					Status:    "deployed",
				},
			}, nil)

		releases, err := gateway.ListReleases(ctx, "default")
		assert.NoError(t, err)
		assert.Equal(t, 1, len(releases))
	})

	t.Run("Legacy Template Methods", func(t *testing.T) {
		templateOptions := &domain.TemplateOptions{
			ReleaseName: "compatibility-test",
			Namespace:   "default",
		}

		mockHelmPort.EXPECT().
			RenderTemplate(ctx, gomock.Any()).
			Return(&domain.TemplateResult{
				Manifests: map[string]string{
					"test.yaml": "apiVersion: v1\nkind: Pod",
				},
			}, nil)

		result, err := gateway.RenderTemplate(ctx, testChart, templateOptions)
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, 1, len(result.Manifests))
	})
}

func TestRefactoredHelmGateway_ErrorScenarios(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHelmPort := mocks.NewMockHelmPort(ctrl)
	mockLogger := mocks.NewMockLoggerPort(ctrl)

	// Setup logging mocks for error scenarios
	mockLogger.EXPECT().InfoWithContext(gomock.Any(), gomock.Any()).AnyTimes()
	mockLogger.EXPECT().DebugWithContext(gomock.Any(), gomock.Any()).AnyTimes()
	mockLogger.EXPECT().ErrorWithContext(gomock.Any(), gomock.Any()).AnyTimes()
	mockLogger.EXPECT().WarnWithContext(gomock.Any(), gomock.Any()).AnyTimes()

	gateway := NewRefactoredHelmGateway(mockHelmPort, mockLogger)

	ctx := context.Background()
	testChart := domain.Chart{
		Name: "error-test",
		Path: "/path/to/chart",
	}

	t.Run("Deployment Error Handling", func(t *testing.T) {
		mockHelmPort.EXPECT().
			InstallChart(ctx, gomock.Any()).
			Return(nil, fmt.Errorf("release error-test already exists"))

		testOptions := &domain.DeploymentOptions{
			Strategy: "rolling-update",
		}

		err := gateway.DeployChart(ctx, testChart, testOptions)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "already exists")
	})

	t.Run("Template Error Handling", func(t *testing.T) {
		templateOptions := &domain.TemplateOptions{
			ReleaseName: "error-test",
			Namespace:   "default",
		}

		mockHelmPort.EXPECT().
			RenderTemplate(ctx, gomock.Any()).
			Return(nil, fmt.Errorf("template parsing error"))

		result, err := gateway.RenderTemplate(ctx, testChart, templateOptions)
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "template parsing")
	})

	t.Run("Release Management Error Handling", func(t *testing.T) {
		mockHelmPort.EXPECT().
			ListReleases(ctx, gomock.Any()).
			Return(nil, fmt.Errorf("kubernetes api server unreachable"))

		releases, err := gateway.ListReleases(ctx, "default")
		assert.Error(t, err)
		assert.Nil(t, releases)
		assert.Contains(t, err.Error(), "unreachable")
	})
}

func BenchmarkRefactoredHelmGateway_Performance(b *testing.B) {
	ctrl := gomock.NewController(b)
	defer ctrl.Finish()

	mockHelmPort := mocks.NewMockHelmPort(ctrl)
	mockLogger := mocks.NewMockLoggerPort(ctrl)

	// Setup minimal mocks for benchmarking
	mockLogger.EXPECT().InfoWithContext(gomock.Any(), gomock.Any()).AnyTimes()
	mockLogger.EXPECT().DebugWithContext(gomock.Any(), gomock.Any()).AnyTimes()

	gateway := NewRefactoredHelmGateway(mockHelmPort, mockLogger)

	ctx := context.Background()
	testChart := domain.Chart{
		Name:    "benchmark-test",
		Path:    "/path/to/chart",
		Version: "1.0.0",
	}

	b.Run("DeployChart Performance", func(b *testing.B) {
		mockHelmPort.EXPECT().
			InstallChart(ctx, gomock.Any()).
			Return(&domain.HelmDeploymentResult{
				ReleaseName: "benchmark-test",
				Namespace:   "default",
				Revision:    1,
				Status:      "deployed",
			}, nil).
			Times(b.N)

		testOptions := &domain.DeploymentOptions{
			Strategy: "rolling-update",
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = gateway.DeployChart(ctx, testChart, testOptions)
		}
	})

	b.Run("Error Classification Performance", func(b *testing.B) {
		testError := fmt.Errorf("release benchmark-test not found")
		
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _ = gateway.ClassifyError(ctx, testError, "deploy")
		}
	})
}

// Helper function to validate the refactored gateway maintains all expected functionality
func validateRefactoredGatewayInterface(gateway *RefactoredHelmGateway) {
	// This function ensures the refactored gateway implements all expected interfaces
	var _ LegacyHelmGateway = gateway

	// Additional interface checks could be added here for extended functionality
	// These would be compile-time checks to ensure interface compliance
}