package database

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	_ "github.com/lib/pq"
)

// Config holds database connection configuration
type Config struct {
	Host            string
	Port            int
	User            string
	Password        string
	Database        string
	SSLMode         string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
	ConnMaxIdleTime time.Duration
	ConnTimeout     time.Duration
}

// Connection represents a database connection wrapper
type Connection struct {
	db     *sql.DB
	config *Config
	logger *slog.Logger
}

// NewConnection creates a new database connection
func NewConnection(config *Config, logger *slog.Logger) (*Connection, error) {
	conn := &Connection{
		config: config,
		logger: logger.With("component", "database"),
	}

	if err := conn.connect(); err != nil {
		return nil, fmt.Errorf("failed to establish database connection: %w", err)
	}

	return conn, nil
}

// connect establishes the database connection
func (c *Connection) connect() error {
	dsn := c.buildDSN()
	
	c.logger.Info("Connecting to database", 
		"host", c.config.Host,
		"port", c.config.Port,
		"database", c.config.Database,
		"ssl_mode", c.config.SSLMode)

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return fmt.Errorf("failed to open database connection: %w", err)
	}

	// Configure connection pool
	db.SetMaxOpenConns(c.config.MaxOpenConns)
	db.SetMaxIdleConns(c.config.MaxIdleConns)
	db.SetConnMaxLifetime(c.config.ConnMaxLifetime)
	db.SetConnMaxIdleTime(c.config.ConnMaxIdleTime)

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), c.config.ConnTimeout)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return fmt.Errorf("failed to ping database: %w", err)
	}

	c.db = db
	c.logger.Info("Database connection established successfully")
	return nil
}

// buildDSN builds the database connection string
func (c *Connection) buildDSN() string {
	return fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s connect_timeout=%d",
		c.config.Host,
		c.config.Port,
		c.config.User,
		c.config.Password,
		c.config.Database,
		c.config.SSLMode,
		int(c.config.ConnTimeout.Seconds()),
	)
}

// DB returns the underlying *sql.DB instance
func (c *Connection) DB() *sql.DB {
	return c.db
}

// Close closes the database connection
func (c *Connection) Close() error {
	if c.db != nil {
		c.logger.Info("Closing database connection")
		return c.db.Close()
	}
	return nil
}

// Health checks the database connection health
func (c *Connection) Health(ctx context.Context) error {
	if c.db == nil {
		return fmt.Errorf("database connection is nil")
	}

	if err := c.db.PingContext(ctx); err != nil {
		return fmt.Errorf("database ping failed: %w", err)
	}

	return nil
}

// Stats returns database connection statistics
func (c *Connection) Stats() sql.DBStats {
	if c.db == nil {
		return sql.DBStats{}
	}
	return c.db.Stats()
}

// WithTransaction executes a function within a database transaction
func (c *Connection) WithTransaction(ctx context.Context, fn func(*sql.Tx) error) error {
	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	if err := fn(tx); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// QueryRow executes a query that returns a single row
func (c *Connection) QueryRow(ctx context.Context, query string, args ...interface{}) *sql.Row {
	return c.db.QueryRowContext(ctx, query, args...)
}

// Query executes a query that returns rows
func (c *Connection) Query(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	return c.db.QueryContext(ctx, query, args...)
}

// Exec executes a query that doesn't return rows
func (c *Connection) Exec(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	return c.db.ExecContext(ctx, query, args...)
}

// DefaultConfig returns a default database configuration
func DefaultConfig() *Config {
	return &Config{
		Host:            "localhost",
		Port:            5432,
		User:            "postgres",
		Password:        "password",
		Database:        "auth_db",
		SSLMode:         "prefer",
		MaxOpenConns:    25,
		MaxIdleConns:    10,
		ConnMaxLifetime: 5 * time.Minute,
		ConnMaxIdleTime: 5 * time.Minute,
		ConnTimeout:     10 * time.Second,
	}
}

// ConfigFromEnv creates database config from environment variables
func ConfigFromEnv() *Config {
	// This would typically read from environment variables
	// For now, returning default config
	return DefaultConfig()
}