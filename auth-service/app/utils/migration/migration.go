package migration

import (
	"database/sql"
	"fmt"
	"io/fs"
	"log/slog"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	_ "github.com/lib/pq"
)

type Migration struct {
	Version   int
	Name      string
	UpSQL     string
	DownSQL   string
	Timestamp time.Time
}

type Migrator struct {
	db          *sql.DB
	logger      *slog.Logger
	migrationsFS fs.FS
}

// NewMigrator creates a new migration manager
func NewMigrator(db *sql.DB, logger *slog.Logger, migrationsFS fs.FS) *Migrator {
	return &Migrator{
		db:           db,
		logger:       logger.With("component", "migrator"),
		migrationsFS: migrationsFS,
	}
}

// CreateMigrationsTable creates the migrations tracking table
func (m *Migrator) CreateMigrationsTable() error {
	query := `
	CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY,
		name VARCHAR(255) NOT NULL,
		applied_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
		checksum VARCHAR(64) NOT NULL
	)`

	if _, err := m.db.Exec(query); err != nil {
		return fmt.Errorf("failed to create migrations table: %w", err)
	}

	m.logger.Info("Migrations table created successfully")
	return nil
}

// LoadMigrations loads all migration files from the filesystem
func (m *Migrator) LoadMigrations() ([]Migration, error) {
	migrations := make([]Migration, 0)

	err := fs.WalkDir(m.migrationsFS, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() || !strings.HasSuffix(path, ".up.sql") {
			return nil
		}

		// Extract version from filename (e.g., "001_create_users.up.sql")
		filename := filepath.Base(path)
		parts := strings.Split(filename, "_")
		if len(parts) < 2 {
			m.logger.Warn("Invalid migration filename format", "filename", filename)
			return nil
		}

		version, err := strconv.Atoi(parts[0])
		if err != nil {
			m.logger.Warn("Invalid migration version", "filename", filename, "error", err)
			return nil
		}

		// Read up migration
		upContent, err := fs.ReadFile(m.migrationsFS, path)
		if err != nil {
			return fmt.Errorf("failed to read up migration %s: %w", path, err)
		}

		// Read down migration
		downPath := strings.Replace(path, ".up.sql", ".down.sql", 1)
		downContent, err := fs.ReadFile(m.migrationsFS, downPath)
		if err != nil {
			return fmt.Errorf("failed to read down migration %s: %w", downPath, err)
		}

		// Extract name from filename
		name := strings.TrimSuffix(filename, ".up.sql")
		name = strings.Join(parts[1:], "_") // Remove version prefix

		migration := Migration{
			Version: version,
			Name:    name,
			UpSQL:   string(upContent),
			DownSQL: string(downContent),
		}

		migrations = append(migrations, migration)
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to load migrations: %w", err)
	}

	// Sort migrations by version
	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].Version < migrations[j].Version
	})

	m.logger.Info("Loaded migrations", "count", len(migrations))
	return migrations, nil
}

// GetAppliedMigrations returns the list of applied migrations
func (m *Migrator) GetAppliedMigrations() ([]Migration, error) {
	query := `SELECT version, name, applied_at FROM schema_migrations ORDER BY version`
	rows, err := m.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query applied migrations: %w", err)
	}
	defer rows.Close()

	var migrations []Migration
	for rows.Next() {
		var migration Migration
		if err := rows.Scan(&migration.Version, &migration.Name, &migration.Timestamp); err != nil {
			return nil, fmt.Errorf("failed to scan migration row: %w", err)
		}
		migrations = append(migrations, migration)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating migration rows: %w", err)
	}

	return migrations, nil
}

// Up runs all pending migrations
func (m *Migrator) Up() error {
	if err := m.CreateMigrationsTable(); err != nil {
		return err
	}

	allMigrations, err := m.LoadMigrations()
	if err != nil {
		return err
	}

	appliedMigrations, err := m.GetAppliedMigrations()
	if err != nil {
		return err
	}

	// Create a map of applied migrations for quick lookup
	appliedMap := make(map[int]bool)
	for _, migration := range appliedMigrations {
		appliedMap[migration.Version] = true
	}

	// Apply pending migrations
	for _, migration := range allMigrations {
		if appliedMap[migration.Version] {
			continue
		}

		if err := m.ApplyMigration(migration); err != nil {
			return fmt.Errorf("failed to apply migration %d: %w", migration.Version, err)
		}

		m.logger.Info("Applied migration", 
			"version", migration.Version, 
			"name", migration.Name)
	}

	return nil
}

// Down rolls back the last migration
func (m *Migrator) Down() error {
	appliedMigrations, err := m.GetAppliedMigrations()
	if err != nil {
		return err
	}

	if len(appliedMigrations) == 0 {
		m.logger.Info("No migrations to roll back")
		return nil
	}

	// Get the last applied migration
	lastMigration := appliedMigrations[len(appliedMigrations)-1]

	allMigrations, err := m.LoadMigrations()
	if err != nil {
		return err
	}

	// Find the migration to roll back
	var migrationToRollback *Migration
	for _, migration := range allMigrations {
		if migration.Version == lastMigration.Version {
			migrationToRollback = &migration
			break
		}
	}

	if migrationToRollback == nil {
		return fmt.Errorf("migration %d not found in filesystem", lastMigration.Version)
	}

	if err := m.RollbackMigration(*migrationToRollback); err != nil {
		return fmt.Errorf("failed to rollback migration %d: %w", migrationToRollback.Version, err)
	}

	m.logger.Info("Rolled back migration", 
		"version", migrationToRollback.Version, 
		"name", migrationToRollback.Name)

	return nil
}

// ApplyMigration applies a single migration
func (m *Migrator) ApplyMigration(migration Migration) error {
	tx, err := m.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Execute the migration
	if _, err := tx.Exec(migration.UpSQL); err != nil {
		return fmt.Errorf("failed to execute migration: %w", err)
	}

	// Record the migration
	insertQuery := `INSERT INTO schema_migrations (version, name, checksum) VALUES ($1, $2, $3)`
	checksum := m.calculateChecksum(migration.UpSQL)
	if _, err := tx.Exec(insertQuery, migration.Version, migration.Name, checksum); err != nil {
		return fmt.Errorf("failed to record migration: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit migration: %w", err)
	}

	return nil
}

// RollbackMigration rolls back a single migration
func (m *Migrator) RollbackMigration(migration Migration) error {
	tx, err := m.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Execute the rollback
	if _, err := tx.Exec(migration.DownSQL); err != nil {
		return fmt.Errorf("failed to execute rollback: %w", err)
	}

	// Remove the migration record
	deleteQuery := `DELETE FROM schema_migrations WHERE version = $1`
	if _, err := tx.Exec(deleteQuery, migration.Version); err != nil {
		return fmt.Errorf("failed to remove migration record: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit rollback: %w", err)
	}

	return nil
}

// Status shows the current migration status
func (m *Migrator) Status() error {
	allMigrations, err := m.LoadMigrations()
	if err != nil {
		return err
	}

	appliedMigrations, err := m.GetAppliedMigrations()
	if err != nil {
		return err
	}

	appliedMap := make(map[int]time.Time)
	for _, migration := range appliedMigrations {
		appliedMap[migration.Version] = migration.Timestamp
	}

	m.logger.Info("Migration status")
	for _, migration := range allMigrations {
		if appliedTime, applied := appliedMap[migration.Version]; applied {
			m.logger.Info("Migration applied", 
				"version", migration.Version, 
				"name", migration.Name,
				"applied_at", appliedTime.Format(time.RFC3339))
		} else {
			m.logger.Info("Migration pending", 
				"version", migration.Version, 
				"name", migration.Name)
		}
	}

	return nil
}

// calculateChecksum calculates a simple checksum for migration content
func (m *Migrator) calculateChecksum(content string) string {
	// Simple checksum implementation
	// In production, you might want to use a proper hash function
	return fmt.Sprintf("%x", len(content))
}