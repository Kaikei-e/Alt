package driver

import (
	"context"
	"fmt"
	"sync"

	logger "pre-processor/utils/logger"

	"github.com/jackc/pgx/v5/pgxpool"
)

// PreparedStatement wraps a pgx prepared statement with metadata
type PreparedStatement struct {
	Statement interface{} // Can be *pgx.Stmt for real DB or mock for testing
	Query     string
	Name      string
}

// PreparedStatementsManager manages prepared statements for improved database performance
type PreparedStatementsManager struct {
	statements map[string]*PreparedStatement
	mu         sync.RWMutex
}

// NewPreparedStatementsManager creates a new prepared statements manager
func NewPreparedStatementsManager() *PreparedStatementsManager {
	return &PreparedStatementsManager{
		statements: make(map[string]*PreparedStatement),
	}
}

// PrepareStatement prepares a SQL statement and caches it
func (p *PreparedStatementsManager) PrepareStatement(ctx context.Context, db interface{}, name string, query string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Check if statement already exists
	if _, exists := p.statements[name]; exists {
		logger.Logger.Info("Statement already prepared", "name", name)
		return nil
	}

	// Handle mock database for testing
	if mockDB, ok := db.(*MockDB); ok {
		return p.prepareMockStatement(ctx, mockDB, name, query)
	}

	// Handle real database connection
	pool, ok := db.(*pgxpool.Pool)
	if !ok {
		return fmt.Errorf("invalid database connection type")
	}

	if pool == nil {
		return fmt.Errorf("database connection is nil")
	}

	// Prepare the statement
	conn, err := pool.Acquire(ctx)
	if err != nil {
		logger.Logger.Error("Failed to acquire connection", "error", err)
		return fmt.Errorf("failed to acquire connection: %w", err)
	}
	defer conn.Release()

	stmt, err := conn.Conn().Prepare(ctx, name, query)
	if err != nil {
		logger.Logger.Error("Failed to prepare statement", "name", name, "error", err)
		return fmt.Errorf("failed to prepare statement %s: %w", name, err)
	}

	// Cache the prepared statement
	p.statements[name] = &PreparedStatement{
		Statement: stmt,
		Query:     query,
		Name:      name,
	}

	logger.Logger.Info("Statement prepared successfully", "name", name)
	return nil
}

// GetStatement retrieves a prepared statement by name
func (p *PreparedStatementsManager) GetStatement(name string) *PreparedStatement {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if stmt, exists := p.statements[name]; exists {
		return stmt
	}

	return nil
}

// CloseAll closes all prepared statements
func (p *PreparedStatementsManager) CloseAll(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	var errors []error

	for name, stmt := range p.statements {
		// For mock statements, we don't need to close anything
		if _, ok := stmt.Statement.(*MockPreparedStatement); ok {
			logger.Logger.Info("Mock statement closed", "name", name)
			continue
		}

		// For real statements, we would close them here
		// Note: pgx v5 doesn't expose Close method on prepared statements
		// They are automatically cleaned up when connection is closed
		logger.Logger.Info("Statement marked for cleanup", "name", name)
	}

	if len(errors) > 0 {
		return fmt.Errorf("failed to close some statements: %v", errors)
	}

	logger.Logger.Info("All prepared statements closed", "count", len(p.statements))
	return nil
}

// prepareMockStatement handles mock database preparation for testing
func (p *PreparedStatementsManager) prepareMockStatement(ctx context.Context, db *MockDB, name string, query string) error {
	mockStmt := &MockPreparedStatement{
		Name:  name,
		Query: query,
	}

	p.statements[name] = &PreparedStatement{
		Statement: mockStmt,
		Query:     query,
		Name:      name,
	}

	return nil
}

// MockPreparedStatement represents a mock prepared statement for testing
type MockPreparedStatement struct {
	Name  string
	Query string
}