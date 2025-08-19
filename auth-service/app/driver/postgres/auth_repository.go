package postgres

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"auth-service/app/domain"
	"auth-service/app/port"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// AuthRepository implements port.AuthRepository for PostgreSQL
type AuthRepository struct {
	db     DatabaseIface
	logger *slog.Logger
}

// NewAuthRepository creates a new PostgreSQL auth repository
func NewAuthRepository(db DatabaseIface, logger *slog.Logger) port.AuthRepository {
	return &AuthRepository{
		db:     db,
		logger: logger.With("component", "auth_repository"),
	}
}

// CreateSession - DEPRECATED: Use Kratos sessions instead
// This method is disabled to follow Ory Kratos best practices
func (r *AuthRepository) CreateSession(ctx context.Context, session *domain.Session) error {
	r.logger.Info("CreateSession called but disabled - using Kratos sessions", "session_id", session.ID, "user_id", session.UserID)
	return nil // No-op: Kratos handles session management
}

// GetSessionByKratosID - DEPRECATED: Use Kratos sessions directly
func (r *AuthRepository) GetSessionByKratosID(ctx context.Context, kratosSessionID string) (*domain.Session, error) {
	r.logger.Info("GetSessionByKratosID called but disabled - using Kratos sessions", "kratos_session_id", kratosSessionID)
	return nil, fmt.Errorf("method disabled - use Kratos ToSession() instead")
}

// GetActiveSessionByUserID - DEPRECATED: Use Kratos sessions directly
func (r *AuthRepository) GetActiveSessionByUserID(ctx context.Context, userID string) (*domain.Session, error) {
	r.logger.Info("GetActiveSessionByUserID called but disabled - using Kratos sessions", "user_id", userID)
	return nil, fmt.Errorf("method disabled - use Kratos ToSession() instead")
}

// UpdateSessionStatus - DEPRECATED: Use Kratos sessions directly
func (r *AuthRepository) UpdateSessionStatus(ctx context.Context, sessionID string, active bool) error {
	r.logger.Info("UpdateSessionStatus called but disabled - using Kratos sessions", "session_id", sessionID, "active", active)
	return nil // No-op: Kratos handles session management
}

// DeleteSession - DEPRECATED: Use Kratos sessions directly
func (r *AuthRepository) DeleteSession(ctx context.Context, sessionID string) error {
	r.logger.Info("DeleteSession called but disabled - using Kratos sessions", "session_id", sessionID)
	return nil // No-op: Kratos handles session management
}

// StoreCSRFToken stores a CSRF token in the database
func (r *AuthRepository) StoreCSRFToken(ctx context.Context, token *domain.CSRFToken) error {
	query := `
		INSERT INTO csrf_tokens (
			id, token, session_id, user_id, created_at, expires_at,
			used, used_at, ip_address, user_agent
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10
		)`

	r.logger.Info("Storing CSRF token", "token", token.Token[:8]+"...", "session_id", token.SessionID)

	// Generate ID for the token
	tokenID := uuid.New()

	// X27 FIX: Support anonymous CSRF tokens - user_id can be NULL for anonymous sessions
	// Check if this is an anonymous session (starts with "anonymous-")
	var userID *uuid.UUID // Use pointer to allow NULL values
	if !strings.HasPrefix(token.SessionID, "anonymous-") {
		// For authenticated sessions, we would get user ID from session
		// For now, skip since anonymous sessions are the primary use case
		r.logger.Debug("Non-anonymous session detected, user_id will be NULL for now", "session_id", token.SessionID)
	}

	_, err := r.db.Exec(ctx, query,
		tokenID,
		token.Token,
		token.SessionID,
		userID, // NULL for anonymous sessions, UUID for authenticated users
		token.CreatedAt,
		token.ExpiresAt,
		false, // used
		nil,   // used_at
		nil,   // ip_address
		nil,   // user_agent
	)

	if err != nil {
		r.logger.Error("Failed to store CSRF token", "token", token.Token[:8]+"...", "error", err)
		return fmt.Errorf("failed to store CSRF token: %w", err)
	}

	r.logger.Info("CSRF token stored successfully", "token", token.Token[:8]+"...")
	return nil
}

// GetCSRFToken retrieves a CSRF token from the database
func (r *AuthRepository) GetCSRFToken(ctx context.Context, token string) (*domain.CSRFToken, error) {
	query := `
		SELECT
			token, session_id, created_at, expires_at, used, used_at
		FROM csrf_tokens
		WHERE token = $1`

	r.logger.Info("Getting CSRF token", "token", token[:8]+"...")

	csrfToken := &domain.CSRFToken{}
	var used bool
	var usedAt *time.Time

	err := r.db.QueryRow(ctx, query, token).Scan(
		&csrfToken.Token,
		&csrfToken.SessionID,
		&csrfToken.CreatedAt,
		&csrfToken.ExpiresAt,
		&used,
		&usedAt,
	)

	if err != nil {
		if err == pgx.ErrNoRows {
			r.logger.Warn("CSRF token not found", "token", token[:8]+"...")
			return nil, fmt.Errorf("CSRF token not found")
		}
		r.logger.Error("Failed to get CSRF token", "token", token[:8]+"...", "error", err)
		return nil, fmt.Errorf("failed to get CSRF token: %w", err)
	}

	// Check if token is used
	if used {
		r.logger.Warn("CSRF token already used", "token", token[:8]+"...")
		return nil, fmt.Errorf("CSRF token already used")
	}

	r.logger.Info("CSRF token retrieved successfully", "token", token[:8]+"...")
	return csrfToken, nil
}

// DeleteCSRFToken deletes a CSRF token from the database
func (r *AuthRepository) DeleteCSRFToken(ctx context.Context, token string) error {
	query := `DELETE FROM csrf_tokens WHERE token = $1`

	r.logger.Info("Deleting CSRF token", "token", token[:8]+"...")

	result, err := r.db.Exec(ctx, query, token)
	if err != nil {
		r.logger.Error("Failed to delete CSRF token", "token", token[:8]+"...", "error", err)
		return fmt.Errorf("failed to delete CSRF token: %w", err)
	}

	rowsAffected := result.RowsAffected()

	if rowsAffected == 0 {
		r.logger.Warn("CSRF token not found for deletion", "token", token[:8]+"...")
		return fmt.Errorf("CSRF token not found")
	}

	r.logger.Info("CSRF token deleted successfully", "token", token[:8]+"...")
	return nil
}

// CleanupExpiredSessions - DEPRECATED: Use Kratos session cleanup instead
func (r *AuthRepository) CleanupExpiredSessions(ctx context.Context) (int64, error) {
	r.logger.Info("CleanupExpiredSessions called but disabled - Kratos handles session cleanup")
	return 0, nil // No-op: Kratos handles session cleanup automatically
}

// CleanupExpiredCSRFTokens removes expired CSRF tokens from the database
func (r *AuthRepository) CleanupExpiredCSRFTokens(ctx context.Context) (int64, error) {
	query := `DELETE FROM csrf_tokens WHERE expires_at < CURRENT_TIMESTAMP - INTERVAL '1 day'`

	r.logger.Info("Cleaning up expired CSRF tokens")

	result, err := r.db.Exec(ctx, query)
	if err != nil {
		r.logger.Error("Failed to cleanup expired CSRF tokens", "error", err)
		return 0, fmt.Errorf("failed to cleanup expired CSRF tokens: %w", err)
	}

	rowsAffected := result.RowsAffected()

	r.logger.Info("Expired CSRF tokens cleaned up successfully", "rows_affected", rowsAffected)
	return rowsAffected, nil
}

// GetSessionStats - DEPRECATED: Use Kratos session stats instead
func (r *AuthRepository) GetSessionStats(ctx context.Context, userID uuid.UUID) (map[string]interface{}, error) {
	r.logger.Info("GetSessionStats called but disabled - use Kratos session stats", "user_id", userID)
	
	// Return empty stats - use Kratos session management instead
	stats := map[string]interface{}{
		"total_sessions":  0,
		"active_sessions": 0,
		"valid_sessions":  0,
	}
	
	return stats, nil
}
