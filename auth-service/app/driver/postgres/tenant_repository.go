package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	"auth-service/app/domain"

	"github.com/google/uuid"
)

// TenantRepository handles tenant operations in PostgreSQL
type TenantRepository struct {
	db     *sql.DB
	logger *slog.Logger
}

// NewTenantRepository creates a new PostgreSQL tenant repository
func NewTenantRepository(db *sql.DB, logger *slog.Logger) *TenantRepository {
	return &TenantRepository{
		db:     db,
		logger: logger.With("component", "tenant_repository"),
	}
}

// Create creates a new tenant in the database
func (r *TenantRepository) Create(ctx context.Context, tenant *domain.Tenant) error {
	query := `
		INSERT INTO tenants (
			id, slug, name, status, settings, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7
		)`

	r.logger.Info("Creating tenant", "tenant_id", tenant.ID, "name", tenant.Name)

	_, err := r.db.ExecContext(ctx, query,
		tenant.ID,
		tenant.Slug,
		tenant.Name,
		tenant.Status,
		tenant.Settings,
		tenant.CreatedAt,
		tenant.UpdatedAt,
	)

	if err != nil {
		r.logger.Error("Failed to create tenant", "tenant_id", tenant.ID, "error", err)
		return fmt.Errorf("failed to create tenant: %w", err)
	}

	r.logger.Info("Tenant created successfully", "tenant_id", tenant.ID)
	return nil
}

// GetByID retrieves a tenant by ID
func (r *TenantRepository) GetByID(ctx context.Context, id uuid.UUID) (*domain.Tenant, error) {
	query := `
		SELECT id, slug, name, status, settings, created_at, updated_at, deleted_at
		FROM tenants WHERE id = $1 AND deleted_at IS NULL`

	var tenant domain.Tenant
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&tenant.ID,
		&tenant.Slug,
		&tenant.Name,
		&tenant.Status,
		&tenant.Settings,
		&tenant.CreatedAt,
		&tenant.UpdatedAt,
		&tenant.DeletedAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("tenant not found: %w", err)
		}
		r.logger.Error("Failed to get tenant by ID", "tenant_id", id, "error", err)
		return nil, fmt.Errorf("failed to get tenant: %w", err)
	}

	return &tenant, nil
}

// GetBySlug retrieves a tenant by slug
func (r *TenantRepository) GetBySlug(ctx context.Context, slug string) (*domain.Tenant, error) {
	query := `
		SELECT id, slug, name, status, settings, created_at, updated_at, deleted_at
		FROM tenants WHERE slug = $1 AND deleted_at IS NULL`

	var tenant domain.Tenant
	err := r.db.QueryRowContext(ctx, query, slug).Scan(
		&tenant.ID,
		&tenant.Slug,
		&tenant.Name,
		&tenant.Status,
		&tenant.Settings,
		&tenant.CreatedAt,
		&tenant.UpdatedAt,
		&tenant.DeletedAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("tenant not found: %w", err)
		}
		r.logger.Error("Failed to get tenant by slug", "slug", slug, "error", err)
		return nil, fmt.Errorf("failed to get tenant: %w", err)
	}

	return &tenant, nil
}

// Update updates a tenant in the database
func (r *TenantRepository) Update(ctx context.Context, tenant *domain.Tenant) error {
	query := `
		UPDATE tenants SET
			slug = $2, name = $3, status = $4, settings = $5, updated_at = $6
		WHERE id = $1 AND deleted_at IS NULL`

	r.logger.Info("Updating tenant", "tenant_id", tenant.ID, "name", tenant.Name)

	result, err := r.db.ExecContext(ctx, query,
		tenant.ID,
		tenant.Slug,
		tenant.Name,
		tenant.Status,
		tenant.Settings,
		tenant.UpdatedAt,
	)

	if err != nil {
		r.logger.Error("Failed to update tenant", "tenant_id", tenant.ID, "error", err)
		return fmt.Errorf("failed to update tenant: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("tenant not found or already deleted")
	}

	r.logger.Info("Tenant updated successfully", "tenant_id", tenant.ID)
	return nil
}

// Delete soft deletes a tenant
func (r *TenantRepository) Delete(ctx context.Context, id uuid.UUID) error {
	query := `UPDATE tenants SET deleted_at = $2, status = $3 WHERE id = $1 AND deleted_at IS NULL`

	now := time.Now()
	result, err := r.db.ExecContext(ctx, query, id, now, domain.TenantStatusDeleted)

	if err != nil {
		r.logger.Error("Failed to delete tenant", "tenant_id", id, "error", err)
		return fmt.Errorf("failed to delete tenant: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("tenant not found or already deleted")
	}

	r.logger.Info("Tenant deleted successfully", "tenant_id", id)
	return nil
}

// List retrieves all active tenants
func (r *TenantRepository) List(ctx context.Context) ([]*domain.Tenant, error) {
	query := `
		SELECT id, slug, name, status, settings, created_at, updated_at, deleted_at
		FROM tenants WHERE deleted_at IS NULL ORDER BY created_at DESC`

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		r.logger.Error("Failed to list tenants", "error", err)
		return nil, fmt.Errorf("failed to list tenants: %w", err)
	}
	defer rows.Close()

	var tenants []*domain.Tenant
	for rows.Next() {
		var tenant domain.Tenant
		err := rows.Scan(
			&tenant.ID,
			&tenant.Slug,
			&tenant.Name,
			&tenant.Status,
			&tenant.Settings,
			&tenant.CreatedAt,
			&tenant.UpdatedAt,
			&tenant.DeletedAt,
		)
		if err != nil {
			r.logger.Error("Failed to scan tenant row", "error", err)
			return nil, fmt.Errorf("failed to scan tenant: %w", err)
		}
		tenants = append(tenants, &tenant)
	}

	if err := rows.Err(); err != nil {
		r.logger.Error("Error iterating tenant rows", "error", err)
		return nil, fmt.Errorf("error iterating tenants: %w", err)
	}

	r.logger.Info("Retrieved tenants", "count", len(tenants))
	return tenants, nil
}

// GetTenantStats returns tenant statistics
func (r *TenantRepository) GetTenantStats(ctx context.Context, tenantID uuid.UUID) (map[string]interface{}, error) {
	query := `
		SELECT
			(SELECT COUNT(*) FROM users WHERE tenant_id = $1 AND status != 'deleted') as user_count,
			(SELECT COUNT(*) FROM user_sessions WHERE user_id IN (SELECT id FROM users WHERE tenant_id = $1) AND active = true) as active_sessions,
			(SELECT COUNT(*) FROM user_sessions WHERE user_id IN (SELECT id FROM users WHERE tenant_id = $1) AND created_at >= CURRENT_TIMESTAMP - INTERVAL '24 hours') as sessions_today`

	r.logger.Info("Getting tenant stats", "tenant_id", tenantID)

	var stats map[string]interface{}
	var userCount, activeSessions, sessionsToday int

	err := r.db.QueryRowContext(ctx, query, tenantID).Scan(
		&userCount,
		&activeSessions,
		&sessionsToday,
	)

	if err != nil {
		r.logger.Error("Failed to get tenant stats", "tenant_id", tenantID, "error", err)
		return nil, fmt.Errorf("failed to get tenant stats: %w", err)
	}

	stats = map[string]interface{}{
		"user_count":      userCount,
		"active_sessions": activeSessions,
		"sessions_today":  sessionsToday,
		"timestamp":       time.Now(),
	}

	r.logger.Info("Tenant stats retrieved successfully", "tenant_id", tenantID)
	return stats, nil
}

// IsSlugAvailable checks if a tenant slug is available
func (r *TenantRepository) IsSlugAvailable(ctx context.Context, slug string, excludeID *uuid.UUID) (bool, error) {
	query := `SELECT COUNT(*) FROM tenants WHERE slug = $1 AND deleted_at IS NULL`
	args := []interface{}{slug}

	if excludeID != nil {
		query += " AND id != $2"
		args = append(args, *excludeID)
	}

	r.logger.Info("Checking slug availability", "slug", slug)

	var count int
	err := r.db.QueryRowContext(ctx, query, args...).Scan(&count)
	if err != nil {
		r.logger.Error("Failed to check slug availability", "slug", slug, "error", err)
		return false, fmt.Errorf("failed to check slug availability: %w", err)
	}

	available := count == 0
	r.logger.Info("Slug availability checked", "slug", slug, "available", available)
	return available, nil
}

// GetDefaultTenant returns the default tenant for migration purposes
func (r *TenantRepository) GetDefaultTenant(ctx context.Context) (*domain.Tenant, error) {
	return r.GetBySlug(ctx, "default")
}
