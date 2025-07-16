package postgres

import (
	"context"
	"fmt"
	"log/slog"
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

// CreateSession creates a new session in the database
func (r *AuthRepository) CreateSession(ctx context.Context, session *domain.Session) error {
	query := `
		INSERT INTO user_sessions (
			id, user_id, kratos_session_id, active, created_at,
			expires_at, updated_at, last_activity_at, ip_address,
			user_agent, device_info, session_metadata
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12
		)`

	r.logger.Info("Creating session", "session_id", session.ID, "user_id", session.UserID)

	_, err := r.db.Exec(ctx, query,
		session.ID,
		session.UserID,
		session.KratosSessionID,
		session.Active,
		session.CreatedAt,
		session.ExpiresAt,
		session.UpdatedAt,
		session.LastActivityAt,
		nil, // ip_address - would be populated from request context
		nil, // user_agent - would be populated from request context
		nil, // device_info - would be populated from request context
		nil, // session_metadata - would be populated from request context
	)

	if err != nil {
		r.logger.Error("Failed to create session", "session_id", session.ID, "error", err)
		return fmt.Errorf("failed to create session: %w", err)
	}

	r.logger.Info("Session created successfully", "session_id", session.ID)
	return nil
}

// GetSessionByKratosID retrieves a session by Kratos session ID
func (r *AuthRepository) GetSessionByKratosID(ctx context.Context, kratosSessionID string) (*domain.Session, error) {
	query := `
		SELECT
			id, user_id, kratos_session_id, active, created_at,
			expires_at, updated_at, last_activity_at
		FROM user_sessions
		WHERE kratos_session_id = $1`

	r.logger.Info("Getting session by Kratos ID", "kratos_session_id", kratosSessionID)

	session := &domain.Session{}
	err := r.db.QueryRow(ctx, query, kratosSessionID).Scan(
		&session.ID,
		&session.UserID,
		&session.KratosSessionID,
		&session.Active,
		&session.CreatedAt,
		&session.ExpiresAt,
		&session.UpdatedAt,
		&session.LastActivityAt,
	)

	if err != nil {
		if err == pgx.ErrNoRows {
			r.logger.Warn("Session not found", "kratos_session_id", kratosSessionID)
			return nil, fmt.Errorf("session not found")
		}
		r.logger.Error("Failed to get session by Kratos ID", "kratos_session_id", kratosSessionID, "error", err)
		return nil, fmt.Errorf("failed to get session: %w", err)
	}

	r.logger.Info("Session retrieved successfully", "kratos_session_id", kratosSessionID)
	return session, nil
}

// GetActiveSessionByUserID retrieves the active session for a user
func (r *AuthRepository) GetActiveSessionByUserID(ctx context.Context, userID string) (*domain.Session, error) {
	query := `
		SELECT
			id, user_id, kratos_session_id, active, created_at,
			expires_at, updated_at, last_activity_at
		FROM user_sessions
		WHERE user_id = $1 AND active = true AND expires_at > CURRENT_TIMESTAMP
		ORDER BY created_at DESC
		LIMIT 1`

	r.logger.Info("Getting active session by user ID", "user_id", userID)

	session := &domain.Session{}
	err := r.db.QueryRow(ctx, query, userID).Scan(
		&session.ID,
		&session.UserID,
		&session.KratosSessionID,
		&session.Active,
		&session.CreatedAt,
		&session.ExpiresAt,
		&session.UpdatedAt,
		&session.LastActivityAt,
	)

	if err != nil {
		if err == pgx.ErrNoRows {
			r.logger.Warn("Active session not found", "user_id", userID)
			return nil, fmt.Errorf("active session not found")
		}
		r.logger.Error("Failed to get active session by user ID", "user_id", userID, "error", err)
		return nil, fmt.Errorf("failed to get active session: %w", err)
	}

	r.logger.Info("Active session retrieved successfully", "user_id", userID)
	return session, nil
}

// UpdateSessionStatus updates the session status
func (r *AuthRepository) UpdateSessionStatus(ctx context.Context, sessionID string, active bool) error {
	query := `
		UPDATE user_sessions
		SET active = $2, updated_at = CURRENT_TIMESTAMP, last_activity_at = CURRENT_TIMESTAMP
		WHERE id = $1`

	r.logger.Info("Updating session status", "session_id", sessionID, "active", active)

	result, err := r.db.Exec(ctx, query, sessionID, active)
	if err != nil {
		r.logger.Error("Failed to update session status", "session_id", sessionID, "error", err)
		return fmt.Errorf("failed to update session status: %w", err)
	}

	rowsAffected := result.RowsAffected()

	if rowsAffected == 0 {
		r.logger.Warn("Session not found for update", "session_id", sessionID)
		return fmt.Errorf("session not found")
	}

	r.logger.Info("Session status updated successfully", "session_id", sessionID)
	return nil
}

// DeleteSession deletes a session from the database
func (r *AuthRepository) DeleteSession(ctx context.Context, sessionID string) error {
	query := `DELETE FROM user_sessions WHERE id = $1`

	r.logger.Info("Deleting session", "session_id", sessionID)

	result, err := r.db.Exec(ctx, query, sessionID)
	if err != nil {
		r.logger.Error("Failed to delete session", "session_id", sessionID, "error", err)
		return fmt.Errorf("failed to delete session: %w", err)
	}

	rowsAffected := result.RowsAffected()

	if rowsAffected == 0 {
		r.logger.Warn("Session not found for deletion", "session_id", sessionID)
		return fmt.Errorf("session not found")
	}

	r.logger.Info("Session deleted successfully", "session_id", sessionID)
	return nil
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

	// For now, we'll use a placeholder user ID since the domain.CSRFToken doesn't have it
	// This would need to be resolved by getting the user ID from the session
	var userID uuid.UUID

	_, err := r.db.Exec(ctx, query,
		tokenID,
		token.Token,
		token.SessionID,
		userID, // This would need to be resolved
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

// CleanupExpiredSessions removes expired sessions from the database
func (r *AuthRepository) CleanupExpiredSessions(ctx context.Context) (int64, error) {
	query := `DELETE FROM user_sessions WHERE expires_at < CURRENT_TIMESTAMP - INTERVAL '7 days'`

	r.logger.Info("Cleaning up expired sessions")

	result, err := r.db.Exec(ctx, query)
	if err != nil {
		r.logger.Error("Failed to cleanup expired sessions", "error", err)
		return 0, fmt.Errorf("failed to cleanup expired sessions: %w", err)
	}

	rowsAffected := result.RowsAffected()

	r.logger.Info("Expired sessions cleaned up successfully", "rows_affected", rowsAffected)
	return rowsAffected, nil
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

// GetSessionStats returns session statistics
func (r *AuthRepository) GetSessionStats(ctx context.Context, userID uuid.UUID) (map[string]interface{}, error) {
	query := `
		SELECT
			COUNT(*) as total_sessions,
			COUNT(CASE WHEN active = true THEN 1 END) as active_sessions,
			COUNT(CASE WHEN expires_at > CURRENT_TIMESTAMP THEN 1 END) as valid_sessions,
			MAX(last_activity_at) as last_activity
		FROM user_sessions
		WHERE user_id = $1`

	r.logger.Info("Getting session stats", "user_id", userID)

	var stats map[string]interface{}
	var totalSessions, activeSessions, validSessions int
	var lastActivity *time.Time

	err := r.db.QueryRow(ctx, query, userID).Scan(
		&totalSessions,
		&activeSessions,
		&validSessions,
		&lastActivity,
	)

	if err != nil {
		r.logger.Error("Failed to get session stats", "user_id", userID, "error", err)
		return nil, fmt.Errorf("failed to get session stats: %w", err)
	}

	stats = map[string]interface{}{
		"total_sessions":  totalSessions,
		"active_sessions": activeSessions,
		"valid_sessions":  validSessions,
	}

	if lastActivity != nil {
		stats["last_activity"] = *lastActivity
	}

	r.logger.Info("Session stats retrieved successfully", "user_id", userID)
	return stats, nil
}
