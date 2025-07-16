package integration_test

import (
	"context"
	"fmt"
	"time"

	"auth-service/app/config"
	"auth-service/app/driver/postgres"
	"auth-service/app/driver/kratos"
	"auth-service/app/utils/logger"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	// Test environment configuration
	TestPostgresHost     = "localhost"
	TestPostgresPort     = "5433"
	TestPostgresDB       = "auth_test_db"
	TestPostgresUser     = "auth_test_user"
	TestPostgresPassword = "test_password"
	TestPostgresSSLMode  = "disable"
	
	TestKratosPublicURL = "http://localhost:4433"
	TestKratosAdminURL  = "http://localhost:4434"
	
	TestAuthServiceURL = "http://localhost:9500"
)

// TestConfig creates a configuration for integration tests
func TestConfig() *config.Config {
	return &config.Config{
		// Server
		Port:     "9500",
		Host:     "0.0.0.0",
		LogLevel: "debug",
		
		// Database
		DatabaseURL:      fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=%s", TestPostgresUser, TestPostgresPassword, TestPostgresHost, TestPostgresPort, TestPostgresDB, TestPostgresSSLMode),
		DatabaseHost:     TestPostgresHost,
		DatabasePort:     TestPostgresPort,
		DatabaseName:     TestPostgresDB,
		DatabaseUser:     TestPostgresUser,
		DatabasePassword: TestPostgresPassword,
		DatabaseSSLMode:  TestPostgresSSLMode,
		
		// Kratos
		KratosPublicURL: TestKratosPublicURL,
		KratosAdminURL:  TestKratosAdminURL,
		
		// CSRF
		CSRFTokenLength: 32,
		SessionTimeout:  24 * time.Hour,
		
		// Features
		EnableAuditLog: true,
		EnableMetrics:  true,
	}
}

// TestDatabaseConnection creates a database connection for integration tests
func TestDatabaseConnection() (*pgxpool.Pool, error) {
	cfg := TestConfig()
	
	// Create logger
	testLogger, err := logger.New("debug")
	if err != nil {
		return nil, fmt.Errorf("failed to create logger: %w", err)
	}
	
	// Create database connection
	db, err := postgres.NewConnection(cfg, testLogger)
	if err != nil {
		return nil, fmt.Errorf("failed to create database connection: %w", err)
	}
	
	return db.Pool(), nil
}

// TestKratosClient creates a Kratos client for integration tests
func TestKratosClient() (*kratos.Client, error) {
	cfg := TestConfig()
	
	// Create logger
	testLogger, err := logger.New("debug")
	if err != nil {
		return nil, fmt.Errorf("failed to create logger: %w", err)
	}
	
	// Create Kratos client
	return kratos.NewClient(cfg, testLogger)
}

// WaitForService waits for a service to be healthy
func WaitForService(ctx context.Context, healthCheckFunc func(context.Context) error, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	
	for time.Now().Before(deadline) {
		if err := healthCheckFunc(ctx); err == nil {
			return nil
		}
		
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(1 * time.Second):
			// Continue waiting
		}
	}
	
	return fmt.Errorf("service did not become healthy within %v", timeout)
}

// WaitForDatabase waits for the database to be ready
func WaitForDatabase(ctx context.Context) error {
	return WaitForService(ctx, func(ctx context.Context) error {
		pool, err := TestDatabaseConnection()
		if err != nil {
			return err
		}
		defer pool.Close()
		
		return pool.Ping(ctx)
	}, 30*time.Second)
}

// WaitForKratos waits for Kratos to be ready
func WaitForKratos(ctx context.Context) error {
	return WaitForService(ctx, func(ctx context.Context) error {
		_, err := TestKratosClient()
		if err != nil {
			return err
		}
		
		// Try to call Kratos health endpoint
		// This would require implementing a health check method in the client
		// For now, we'll just check if the client can be created
		return nil
	}, 60*time.Second)
}

// CleanupTestData cleans up test data from the database
func CleanupTestData(ctx context.Context) error {
	pool, err := TestDatabaseConnection()
	if err != nil {
		return err
	}
	defer pool.Close()
	
	// Clean up in reverse order of dependencies
	cleanupQueries := []string{
		"DELETE FROM audit_logs WHERE tenant_id IN (SELECT id FROM tenants WHERE domain LIKE 'test%.example.com')",
		"DELETE FROM user_preferences WHERE user_id IN (SELECT id FROM users WHERE email LIKE '%@example.com')",
		"DELETE FROM csrf_tokens WHERE user_id IN (SELECT id FROM users WHERE email LIKE '%@example.com')",
		"DELETE FROM user_sessions WHERE user_id IN (SELECT id FROM users WHERE email LIKE '%@example.com')",
		"DELETE FROM users WHERE email LIKE '%@example.com'",
		"DELETE FROM tenants WHERE domain LIKE 'test%.example.com'",
	}
	
	for _, query := range cleanupQueries {
		if _, err := pool.Exec(ctx, query); err != nil {
			return fmt.Errorf("failed to execute cleanup query: %w", err)
		}
	}
	
	return nil
}

// SetupTestData sets up test data in the database
func SetupTestData(ctx context.Context) error {
	pool, err := TestDatabaseConnection()
	if err != nil {
		return err
	}
	defer pool.Close()
	
	// The test data is already set up by the SQL scripts
	// This function can be used to add additional test data if needed
	return nil
}