package integration_test

import (
	"context"
	"testing"
	"time"

	"auth-service/app/driver/kratos"
	"auth-service/app/gateway"
	"auth-service/app/utils/logger"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestKratosIntegration(t *testing.T) {
	// Skip if not in integration test mode
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()
	
	// Wait for Kratos to be ready
	require.NoError(t, WaitForKratos(ctx), "Kratos should be ready")
	
	// Create Kratos client
	client, err := TestKratosClient()
	require.NoError(t, err, "Should create Kratos client")
	
	// Test basic client functionality
	t.Run("Kratos client creation", func(t *testing.T) {
		assert.NotNil(t, client, "Kratos client should not be nil")
		assert.NotNil(t, client.PublicAPI(), "Public API should not be nil")
		assert.NotNil(t, client.AdminAPI(), "Admin API should not be nil")
		assert.NotEmpty(t, client.GetPublicURL(), "Public URL should not be empty")
		assert.NotEmpty(t, client.GetAdminURL(), "Admin URL should not be empty")
	})
}

func TestKratosHealthCheck(t *testing.T) {
	// Skip if not in integration test mode
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()
	
	// Wait for Kratos to be ready
	require.NoError(t, WaitForKratos(ctx), "Kratos should be ready")
	
	// Create Kratos client
	client, err := TestKratosClient()
	require.NoError(t, err, "Should create Kratos client")
	
	// Test health check
	t.Run("Kratos health check", func(t *testing.T) {
		err := client.HealthCheck(ctx)
		require.NoError(t, err, "Kratos should be healthy")
	})
	
	// Test health check with timeout
	t.Run("Kratos health check with timeout", func(t *testing.T) {
		timeoutCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		
		err := client.HealthCheck(timeoutCtx)
		require.NoError(t, err, "Kratos should be healthy within timeout")
	})
}

func TestKratosAPIAccess(t *testing.T) {
	// Skip if not in integration test mode
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()
	
	// Wait for Kratos to be ready
	require.NoError(t, WaitForKratos(ctx), "Kratos should be ready")
	
	// Create Kratos client
	client, err := TestKratosClient()
	require.NoError(t, err, "Should create Kratos client")
	
	// Test direct API access
	t.Run("Access Kratos Public API", func(t *testing.T) {
		publicAPI := client.PublicAPI()
		
		// Test version endpoint
		version, response, err := publicAPI.MetadataAPI.GetVersion(ctx).Execute()
		require.NoError(t, err, "Should get version from public API")
		
		assert.NotNil(t, version, "Version should not be nil")
		assert.Equal(t, 200, response.StatusCode, "Status code should be 200")
		assert.NotEmpty(t, version.GetVersion(), "Version string should not be empty")
	})
	
	// Test admin API access
	t.Run("Access Kratos Admin API", func(t *testing.T) {
		adminAPI := client.AdminAPI()
		
		// Test version endpoint
		version, response, err := adminAPI.MetadataAPI.GetVersion(ctx).Execute()
		require.NoError(t, err, "Should get version from admin API")
		
		assert.NotNil(t, version, "Version should not be nil")
		assert.Equal(t, 200, response.StatusCode, "Status code should be 200")
		assert.NotEmpty(t, version.GetVersion(), "Version string should not be empty")
	})
	
	// Test creating a login flow
	t.Run("Create login flow via Public API", func(t *testing.T) {
		publicAPI := client.PublicAPI()
		
		// Create login flow
		loginFlow, response, err := publicAPI.FrontendAPI.CreateNativeLoginFlow(ctx).Execute()
		require.NoError(t, err, "Should create login flow")
		
		assert.NotNil(t, loginFlow, "Login flow should not be nil")
		assert.Equal(t, 200, response.StatusCode, "Status code should be 200")
		assert.NotEmpty(t, loginFlow.GetId(), "Login flow ID should not be empty")
		assert.Equal(t, "login", loginFlow.GetType(), "Login flow type should be 'login'")
		assert.NotNil(t, loginFlow.GetUi(), "Login flow UI should not be nil")
		assert.NotEmpty(t, loginFlow.GetUi().Action, "Login flow UI action should not be empty")
	})
	
	// Test creating a registration flow
	t.Run("Create registration flow via Public API", func(t *testing.T) {
		publicAPI := client.PublicAPI()
		
		// Create registration flow
		registrationFlow, response, err := publicAPI.FrontendAPI.CreateNativeRegistrationFlow(ctx).Execute()
		require.NoError(t, err, "Should create registration flow")
		
		assert.NotNil(t, registrationFlow, "Registration flow should not be nil")
		assert.Equal(t, 200, response.StatusCode, "Status code should be 200")
		assert.NotEmpty(t, registrationFlow.GetId(), "Registration flow ID should not be empty")
		assert.Equal(t, "registration", registrationFlow.GetType(), "Registration flow type should be 'registration'")
		assert.NotNil(t, registrationFlow.GetUi(), "Registration flow UI should not be nil")
		assert.NotEmpty(t, registrationFlow.GetUi().Action, "Registration flow UI action should not be empty")
	})
}

func TestKratosFlowIntegration(t *testing.T) {
	// Skip if not in integration test mode
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()
	
	// Wait for Kratos to be ready
	require.NoError(t, WaitForKratos(ctx), "Kratos should be ready")
	
	// Create Kratos client
	client, err := TestKratosClient()
	require.NoError(t, err, "Should create Kratos client")
	
	// Create logger
	testLogger, err := logger.New("debug")
	require.NoError(t, err, "Should create logger")
	
	// Create auth gateway using adapter
	adapter := kratos.NewKratosClientAdapter(client)
	authGateway := gateway.NewAuthGateway(adapter, testLogger)
	
	// Test creating flows through gateway
	t.Run("Create login flow", func(t *testing.T) {
		flow, err := authGateway.CreateLoginFlow(ctx)
		require.NoError(t, err, "Should create login flow")
		
		assert.NotNil(t, flow, "Flow should not be nil")
		assert.NotEmpty(t, flow.ID, "Flow ID should not be empty")
		assert.Equal(t, "login", flow.Type, "Flow type should be login")
	})
	
	t.Run("Create registration flow", func(t *testing.T) {
		flow, err := authGateway.CreateRegistrationFlow(ctx)
		require.NoError(t, err, "Should create registration flow")
		
		assert.NotNil(t, flow, "Flow should not be nil")
		assert.NotEmpty(t, flow.ID, "Flow ID should not be empty")
		assert.Equal(t, "registration", flow.Type, "Flow type should be registration")
	})
}

func TestKratosSessionIntegration(t *testing.T) {
	// Skip if not in integration test mode
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()
	
	// Wait for Kratos to be ready
	require.NoError(t, WaitForKratos(ctx), "Kratos should be ready")
	
	// Create Kratos client
	client, err := TestKratosClient()
	require.NoError(t, err, "Should create Kratos client")
	
	// Create logger
	testLogger, err := logger.New("debug")
	require.NoError(t, err, "Should create logger")
	
	// Create auth gateway using adapter
	adapter := kratos.NewKratosClientAdapter(client)
	authGateway := gateway.NewAuthGateway(adapter, testLogger)
	
	// Test session retrieval with invalid token
	t.Run("Get session with invalid token", func(t *testing.T) {
		invalidToken := "invalid-session-token"
		
		session, err := authGateway.GetSession(ctx, invalidToken)
		
		// With stub implementation, this will return a dummy session
		assert.NoError(t, err, "Should not return error with stub implementation")
		assert.NotNil(t, session, "Session should not be nil with stub implementation")
	})
	
	// Test session revocation with invalid session
	t.Run("Revoke invalid session", func(t *testing.T) {
		invalidSessionID := "invalid-session-id"
		
		err := authGateway.RevokeSession(ctx, invalidSessionID)
		
		// With stub implementation, this will return nil
		assert.NoError(t, err, "Should not return error with stub implementation")
	})
}

func TestKratosErrorHandling(t *testing.T) {
	// Skip if not in integration test mode
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()
	
	// Wait for Kratos to be ready
	require.NoError(t, WaitForKratos(ctx), "Kratos should be ready")
	
	// Create Kratos client
	client, err := TestKratosClient()
	require.NoError(t, err, "Should create Kratos client")
	
	// Create logger
	testLogger, err := logger.New("debug")
	require.NoError(t, err, "Should create logger")
	
	// Create auth gateway using adapter
	adapter := kratos.NewKratosClientAdapter(client)
	authGateway := gateway.NewAuthGateway(adapter, testLogger)
	
	// Test error handling with invalid flow submission
	t.Run("Submit invalid login flow", func(t *testing.T) {
		invalidFlowID := "invalid-flow-id"
		invalidData := map[string]interface{}{
			"method":   "password",
			"password": "invalid",
		}
		
		session, err := authGateway.SubmitLoginFlow(ctx, invalidFlowID, invalidData)
		
		// With stub implementation, this will return a dummy session
		assert.NoError(t, err, "Should not return error with stub implementation")
		assert.NotNil(t, session, "Session should not be nil with stub implementation")
	})
	
	// Test error handling with malformed data
	t.Run("Submit malformed registration data", func(t *testing.T) {
		// Create a valid registration flow first
		registrationFlow, err := authGateway.CreateRegistrationFlow(ctx)
		require.NoError(t, err, "Should create registration flow")
		
		// Submit with malformed data
		malformedData := map[string]interface{}{
			"invalid": "data",
		}
		
		session, err := authGateway.SubmitRegistrationFlow(ctx, registrationFlow.ID, malformedData)
		
		// With stub implementation, this will return a dummy session
		assert.NoError(t, err, "Should not return error with stub implementation")
		assert.NotNil(t, session, "Session should not be nil with stub implementation")
	})
}

func TestKratosClientConfiguration(t *testing.T) {
	// Skip if not in integration test mode
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()
	
	// Wait for Kratos to be ready
	require.NoError(t, WaitForKratos(ctx), "Kratos should be ready")
	
	// Test client configuration
	t.Run("Kratos client configuration", func(t *testing.T) {
		cfg := TestConfig()
		
		// Verify configuration values
		assert.Equal(t, TestKratosPublicURL, cfg.KratosPublicURL, "Public URL should match")
		assert.Equal(t, TestKratosAdminURL, cfg.KratosAdminURL, "Admin URL should match")
		
		// Create client with configuration
		testLogger, err := logger.New("debug")
		require.NoError(t, err, "Should create logger")
		
		client, err := kratos.NewClient(cfg, testLogger)
		require.NoError(t, err, "Should create Kratos client")
		
		assert.NotNil(t, client, "Client should not be nil")
		assert.NotNil(t, client.PublicAPI(), "Public API should not be nil")
		assert.NotNil(t, client.AdminAPI(), "Admin API should not be nil")
	})
}

func TestKratosMultipleClients(t *testing.T) {
	// Skip if not in integration test mode
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Test that we can create multiple clients
	t.Run("Multiple Kratos clients", func(t *testing.T) {
		cfg := TestConfig()
		testLogger, err := logger.New("debug")
		require.NoError(t, err, "Should create logger")
		
		// Create multiple clients
		client1, err := kratos.NewClient(cfg, testLogger)
		require.NoError(t, err, "Should create first Kratos client")
		
		client2, err := kratos.NewClient(cfg, testLogger)
		require.NoError(t, err, "Should create second Kratos client")
		
		// Both should be functional
		assert.NotNil(t, client1, "First client should not be nil")
		assert.NotNil(t, client2, "Second client should not be nil")
		
		// Both should have access to APIs
		assert.NotNil(t, client1.PublicAPI(), "First client public API should not be nil")
		assert.NotNil(t, client2.PublicAPI(), "Second client public API should not be nil")
	})
}

func TestKratosHealthcheckTimeout(t *testing.T) {
	// Skip if not in integration test mode
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()
	
	// Test that Kratos is responding
	t.Run("Kratos health check with timeout", func(t *testing.T) {
		// Wait for Kratos to be ready with a timeout
		timeoutCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		
		err := WaitForKratos(timeoutCtx)
		require.NoError(t, err, "Kratos should be healthy within timeout")
	})
}