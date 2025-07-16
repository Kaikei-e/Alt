package postgres

import (
	"context"
	"testing"
	"time"

	"auth-service/app/domain"
	"auth-service/app/utils/logger"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/pashagolub/pgxmock/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Helper function to create a test auth repository with mocked database
func createTestAuthRepository(t *testing.T) (*AuthRepository, pgxmock.PgxPoolIface) {
	t.Helper()

	// Create mock database
	mockDB, err := pgxmock.NewPool()
	require.NoError(t, err)

	// Create logger
	testLogger, err := logger.New("debug")
	require.NoError(t, err)

	// Create repository
	repo := NewAuthRepository(mockDB, testLogger).(*AuthRepository)

	return repo, mockDB
}

// Helper function to create a test session
func createTestSession(t *testing.T) *domain.Session {
	t.Helper()

	userID := uuid.New()
	kratosSessionID := "kratos-session-123"
	duration := time.Hour

	session, err := domain.NewSession(userID, kratosSessionID, duration)
	require.NoError(t, err)

	return session
}

// Helper function to create a test CSRF token
func createTestCSRFToken(t *testing.T) *domain.CSRFToken {
	t.Helper()

	sessionID := "session-123"
	tokenLength := 32
	duration := 30 * time.Minute

	token, err := domain.NewCSRFToken(sessionID, tokenLength, duration)
	require.NoError(t, err)

	return token
}

func TestAuthRepository_CreateSession(t *testing.T) {
	tests := []struct {
		name     string
		session  *domain.Session
		setupDB  func(pgxmock.PgxPoolIface, *domain.Session)
		wantErr  bool
		errorMsg string
	}{
		{
			name:    "successful session creation",
			session: createTestSession(t),
			setupDB: func(mockDB pgxmock.PgxPoolIface, session *domain.Session) {
				mockDB.ExpectExec("INSERT INTO user_sessions").
					WithArgs(
						session.ID,
						session.UserID,
						session.KratosSessionID,
						session.Active,
						session.CreatedAt,
						session.ExpiresAt,
						session.UpdatedAt,
						session.LastActivityAt,
						nil, // ip_address
						nil, // user_agent
						nil, // device_info
						nil, // session_metadata
					).
					WillReturnResult(pgxmock.NewResult("INSERT", 1))
			},
			wantErr: false,
		},
		{
			name:    "database error during session creation",
			session: createTestSession(t),
			setupDB: func(mockDB pgxmock.PgxPoolIface, session *domain.Session) {
				mockDB.ExpectExec("INSERT INTO user_sessions").
					WithArgs(
						session.ID,
						session.UserID,
						session.KratosSessionID,
						session.Active,
						session.CreatedAt,
						session.ExpiresAt,
						session.UpdatedAt,
						session.LastActivityAt,
						nil, // ip_address
						nil, // user_agent
						nil, // device_info
						nil, // session_metadata
					).
					WillReturnError(pgx.ErrTxClosed)
			},
			wantErr:  true,
			errorMsg: "failed to create session",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, mockDB := createTestAuthRepository(t)
			defer mockDB.Close()

			tt.setupDB(mockDB, tt.session)

			err := repo.CreateSession(context.Background(), tt.session)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				assert.NoError(t, err)
			}

			assert.NoError(t, mockDB.ExpectationsWereMet())
		})
	}
}

func TestAuthRepository_GetSessionByKratosID(t *testing.T) {
	tests := []struct {
		name            string
		kratosSessionID string
		setupDB         func(pgxmock.PgxPoolIface, string)
		wantErr         bool
		errorMsg        string
		validateSession func(*testing.T, *domain.Session)
	}{
		{
			name:            "successful session retrieval",
			kratosSessionID: "kratos-session-123",
			setupDB: func(mockDB pgxmock.PgxPoolIface, kratosSessionID string) {
				testSession := createTestSession(t)
				testSession.KratosSessionID = kratosSessionID

				mockDB.ExpectQuery("SELECT(.+)FROM user_sessions WHERE kratos_session_id").
					WithArgs(kratosSessionID).
					WillReturnRows(
						pgxmock.NewRows([]string{
							"id", "user_id", "kratos_session_id", "active",
							"created_at", "expires_at", "updated_at", "last_activity_at",
						}).AddRow(
							testSession.ID,
							testSession.UserID,
							testSession.KratosSessionID,
							testSession.Active,
							testSession.CreatedAt,
							testSession.ExpiresAt,
							testSession.UpdatedAt,
							testSession.LastActivityAt,
						),
					)
			},
			wantErr: false,
			validateSession: func(t *testing.T, session *domain.Session) {
				assert.Equal(t, "kratos-session-123", session.KratosSessionID)
				assert.True(t, session.Active)
			},
		},
		{
			name:            "session not found",
			kratosSessionID: "non-existent-session",
			setupDB: func(mockDB pgxmock.PgxPoolIface, kratosSessionID string) {
				mockDB.ExpectQuery("SELECT(.+)FROM user_sessions WHERE kratos_session_id").
					WithArgs(kratosSessionID).
					WillReturnError(pgx.ErrNoRows)
			},
			wantErr:  true,
			errorMsg: "session not found",
		},
		{
			name:            "database error",
			kratosSessionID: "kratos-session-123",
			setupDB: func(mockDB pgxmock.PgxPoolIface, kratosSessionID string) {
				mockDB.ExpectQuery("SELECT(.+)FROM user_sessions WHERE kratos_session_id").
					WithArgs(kratosSessionID).
					WillReturnError(pgx.ErrTxClosed)
			},
			wantErr:  true,
			errorMsg: "failed to get session",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, mockDB := createTestAuthRepository(t)
			defer mockDB.Close()

			tt.setupDB(mockDB, tt.kratosSessionID)

			session, err := repo.GetSessionByKratosID(context.Background(), tt.kratosSessionID)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
				assert.Nil(t, session)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, session)
				if tt.validateSession != nil {
					tt.validateSession(t, session)
				}
			}

			assert.NoError(t, mockDB.ExpectationsWereMet())
		})
	}
}

func TestAuthRepository_GetActiveSessionByUserID(t *testing.T) {
	tests := []struct {
		name            string
		userID          string
		setupDB         func(pgxmock.PgxPoolIface, string)
		wantErr         bool
		errorMsg        string
		validateSession func(*testing.T, *domain.Session)
	}{
		{
			name:   "successful active session retrieval",
			userID: uuid.New().String(),
			setupDB: func(mockDB pgxmock.PgxPoolIface, userID string) {
				testSession := createTestSession(t)
				testSession.UserID = uuid.MustParse(userID)

				mockDB.ExpectQuery("SELECT(.+)FROM user_sessions WHERE user_id(.+)AND active = true AND expires_at > CURRENT_TIMESTAMP").
					WithArgs(userID).
					WillReturnRows(
						pgxmock.NewRows([]string{
							"id", "user_id", "kratos_session_id", "active",
							"created_at", "expires_at", "updated_at", "last_activity_at",
						}).AddRow(
							testSession.ID,
							testSession.UserID,
							testSession.KratosSessionID,
							testSession.Active,
							testSession.CreatedAt,
							testSession.ExpiresAt,
							testSession.UpdatedAt,
							testSession.LastActivityAt,
						),
					)
			},
			wantErr: false,
			validateSession: func(t *testing.T, session *domain.Session) {
				assert.True(t, session.Active)
				assert.False(t, session.IsExpired())
			},
		},
		{
			name:   "active session not found",
			userID: uuid.New().String(),
			setupDB: func(mockDB pgxmock.PgxPoolIface, userID string) {
				mockDB.ExpectQuery("SELECT(.+)FROM user_sessions WHERE user_id(.+)AND active = true AND expires_at > CURRENT_TIMESTAMP").
					WithArgs(userID).
					WillReturnError(pgx.ErrNoRows)
			},
			wantErr:  true,
			errorMsg: "active session not found",
		},
		{
			name:   "database error",
			userID: uuid.New().String(),
			setupDB: func(mockDB pgxmock.PgxPoolIface, userID string) {
				mockDB.ExpectQuery("SELECT(.+)FROM user_sessions WHERE user_id(.+)AND active = true AND expires_at > CURRENT_TIMESTAMP").
					WithArgs(userID).
					WillReturnError(pgx.ErrTxClosed)
			},
			wantErr:  true,
			errorMsg: "failed to get active session",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, mockDB := createTestAuthRepository(t)
			defer mockDB.Close()

			tt.setupDB(mockDB, tt.userID)

			session, err := repo.GetActiveSessionByUserID(context.Background(), tt.userID)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
				assert.Nil(t, session)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, session)
				if tt.validateSession != nil {
					tt.validateSession(t, session)
				}
			}

			assert.NoError(t, mockDB.ExpectationsWereMet())
		})
	}
}

func TestAuthRepository_UpdateSessionStatus(t *testing.T) {
	tests := []struct {
		name      string
		sessionID string
		active    bool
		setupDB   func(pgxmock.PgxPoolIface, string, bool)
		wantErr   bool
		errorMsg  string
	}{
		{
			name:      "successful session status update",
			sessionID: uuid.New().String(),
			active:    false,
			setupDB: func(mockDB pgxmock.PgxPoolIface, sessionID string, active bool) {
				mockDB.ExpectExec("UPDATE user_sessions SET active(.+)updated_at(.+)last_activity_at(.+)WHERE id").
					WithArgs(sessionID, active).
					WillReturnResult(pgxmock.NewResult("UPDATE", 1))
			},
			wantErr: false,
		},
		{
			name:      "session not found for update",
			sessionID: uuid.New().String(),
			active:    false,
			setupDB: func(mockDB pgxmock.PgxPoolIface, sessionID string, active bool) {
				mockDB.ExpectExec("UPDATE user_sessions SET active(.+)updated_at(.+)last_activity_at(.+)WHERE id").
					WithArgs(sessionID, active).
					WillReturnResult(pgxmock.NewResult("UPDATE", 0))
			},
			wantErr:  true,
			errorMsg: "session not found",
		},
		{
			name:      "database error during update",
			sessionID: uuid.New().String(),
			active:    false,
			setupDB: func(mockDB pgxmock.PgxPoolIface, sessionID string, active bool) {
				mockDB.ExpectExec("UPDATE user_sessions SET active(.+)updated_at(.+)last_activity_at(.+)WHERE id").
					WithArgs(sessionID, active).
					WillReturnError(pgx.ErrTxClosed)
			},
			wantErr:  true,
			errorMsg: "failed to update session status",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, mockDB := createTestAuthRepository(t)
			defer mockDB.Close()

			tt.setupDB(mockDB, tt.sessionID, tt.active)

			err := repo.UpdateSessionStatus(context.Background(), tt.sessionID, tt.active)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				assert.NoError(t, err)
			}

			assert.NoError(t, mockDB.ExpectationsWereMet())
		})
	}
}

func TestAuthRepository_DeleteSession(t *testing.T) {
	tests := []struct {
		name      string
		sessionID string
		setupDB   func(pgxmock.PgxPoolIface, string)
		wantErr   bool
		errorMsg  string
	}{
		{
			name:      "successful session deletion",
			sessionID: uuid.New().String(),
			setupDB: func(mockDB pgxmock.PgxPoolIface, sessionID string) {
				mockDB.ExpectExec("DELETE FROM user_sessions WHERE id").
					WithArgs(sessionID).
					WillReturnResult(pgxmock.NewResult("DELETE", 1))
			},
			wantErr: false,
		},
		{
			name:      "session not found for deletion",
			sessionID: uuid.New().String(),
			setupDB: func(mockDB pgxmock.PgxPoolIface, sessionID string) {
				mockDB.ExpectExec("DELETE FROM user_sessions WHERE id").
					WithArgs(sessionID).
					WillReturnResult(pgxmock.NewResult("DELETE", 0))
			},
			wantErr:  true,
			errorMsg: "session not found",
		},
		{
			name:      "database error during deletion",
			sessionID: uuid.New().String(),
			setupDB: func(mockDB pgxmock.PgxPoolIface, sessionID string) {
				mockDB.ExpectExec("DELETE FROM user_sessions WHERE id").
					WithArgs(sessionID).
					WillReturnError(pgx.ErrTxClosed)
			},
			wantErr:  true,
			errorMsg: "failed to delete session",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, mockDB := createTestAuthRepository(t)
			defer mockDB.Close()

			tt.setupDB(mockDB, tt.sessionID)

			err := repo.DeleteSession(context.Background(), tt.sessionID)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				assert.NoError(t, err)
			}

			assert.NoError(t, mockDB.ExpectationsWereMet())
		})
	}
}

func TestAuthRepository_StoreCSRFToken(t *testing.T) {
	tests := []struct {
		name     string
		token    *domain.CSRFToken
		setupDB  func(pgxmock.PgxPoolIface, *domain.CSRFToken)
		wantErr  bool
		errorMsg string
	}{
		{
			name:  "successful CSRF token storage",
			token: createTestCSRFToken(t),
			setupDB: func(mockDB pgxmock.PgxPoolIface, token *domain.CSRFToken) {
				mockDB.ExpectExec("INSERT INTO csrf_tokens").
					WithArgs(
						pgxmock.AnyArg(), // tokenID (generated UUID)
						token.Token,
						token.SessionID,
						pgxmock.AnyArg(), // userID (placeholder)
						token.CreatedAt,
						token.ExpiresAt,
						false, // used
						nil,   // used_at
						nil,   // ip_address
						nil,   // user_agent
					).
					WillReturnResult(pgxmock.NewResult("INSERT", 1))
			},
			wantErr: false,
		},
		{
			name:  "database error during CSRF token storage",
			token: createTestCSRFToken(t),
			setupDB: func(mockDB pgxmock.PgxPoolIface, token *domain.CSRFToken) {
				mockDB.ExpectExec("INSERT INTO csrf_tokens").
					WithArgs(
						pgxmock.AnyArg(), // tokenID (generated UUID)
						token.Token,
						token.SessionID,
						pgxmock.AnyArg(), // userID (placeholder)
						token.CreatedAt,
						token.ExpiresAt,
						false, // used
						nil,   // used_at
						nil,   // ip_address
						nil,   // user_agent
					).
					WillReturnError(pgx.ErrTxClosed)
			},
			wantErr:  true,
			errorMsg: "failed to store CSRF token",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, mockDB := createTestAuthRepository(t)
			defer mockDB.Close()

			tt.setupDB(mockDB, tt.token)

			err := repo.StoreCSRFToken(context.Background(), tt.token)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				assert.NoError(t, err)
			}

			assert.NoError(t, mockDB.ExpectationsWereMet())
		})
	}
}

func TestAuthRepository_GetCSRFToken(t *testing.T) {
	tests := []struct {
		name          string
		token         string
		setupDB       func(pgxmock.PgxPoolIface, string)
		wantErr       bool
		errorMsg      string
		validateToken func(*testing.T, *domain.CSRFToken)
	}{
		{
			name:  "successful CSRF token retrieval",
			token: "valid-csrf-token",
			setupDB: func(mockDB pgxmock.PgxPoolIface, token string) {
				testToken := createTestCSRFToken(t)
				testToken.Token = token

				mockDB.ExpectQuery("SELECT(.+)FROM csrf_tokens WHERE token").
					WithArgs(token).
					WillReturnRows(
						pgxmock.NewRows([]string{
							"token", "session_id", "created_at", "expires_at", "used", "used_at",
						}).AddRow(
							testToken.Token,
							testToken.SessionID,
							testToken.CreatedAt,
							testToken.ExpiresAt,
							false, // used
							nil,   // used_at
						),
					)
			},
			wantErr: false,
			validateToken: func(t *testing.T, token *domain.CSRFToken) {
				assert.Equal(t, "valid-csrf-token", token.Token)
				assert.Equal(t, "session-123", token.SessionID)
			},
		},
		{
			name:  "CSRF token not found",
			token: "non-existent-token",
			setupDB: func(mockDB pgxmock.PgxPoolIface, token string) {
				mockDB.ExpectQuery("SELECT(.+)FROM csrf_tokens WHERE token").
					WithArgs(token).
					WillReturnError(pgx.ErrNoRows)
			},
			wantErr:  true,
			errorMsg: "CSRF token not found",
		},
		{
			name:  "CSRF token already used",
			token: "used-csrf-token",
			setupDB: func(mockDB pgxmock.PgxPoolIface, token string) {
				testToken := createTestCSRFToken(t)
				testToken.Token = token
				usedAt := time.Now()

				mockDB.ExpectQuery("SELECT(.+)FROM csrf_tokens WHERE token").
					WithArgs(token).
					WillReturnRows(
						pgxmock.NewRows([]string{
							"token", "session_id", "created_at", "expires_at", "used", "used_at",
						}).AddRow(
							testToken.Token,
							testToken.SessionID,
							testToken.CreatedAt,
							testToken.ExpiresAt,
							true,    // used
							usedAt,  // used_at
						),
					)
			},
			wantErr:  true,
			errorMsg: "CSRF token already used",
		},
		{
			name:  "database error",
			token: "valid-csrf-token",
			setupDB: func(mockDB pgxmock.PgxPoolIface, token string) {
				mockDB.ExpectQuery("SELECT(.+)FROM csrf_tokens WHERE token").
					WithArgs(token).
					WillReturnError(pgx.ErrTxClosed)
			},
			wantErr:  true,
			errorMsg: "failed to get CSRF token",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, mockDB := createTestAuthRepository(t)
			defer mockDB.Close()

			tt.setupDB(mockDB, tt.token)

			token, err := repo.GetCSRFToken(context.Background(), tt.token)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
				assert.Nil(t, token)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, token)
				if tt.validateToken != nil {
					tt.validateToken(t, token)
				}
			}

			assert.NoError(t, mockDB.ExpectationsWereMet())
		})
	}
}

func TestAuthRepository_DeleteCSRFToken(t *testing.T) {
	tests := []struct {
		name     string
		token    string
		setupDB  func(pgxmock.PgxPoolIface, string)
		wantErr  bool
		errorMsg string
	}{
		{
			name:  "successful CSRF token deletion",
			token: "valid-csrf-token",
			setupDB: func(mockDB pgxmock.PgxPoolIface, token string) {
				mockDB.ExpectExec("DELETE FROM csrf_tokens WHERE token").
					WithArgs(token).
					WillReturnResult(pgxmock.NewResult("DELETE", 1))
			},
			wantErr: false,
		},
		{
			name:  "CSRF token not found for deletion",
			token: "non-existent-token",
			setupDB: func(mockDB pgxmock.PgxPoolIface, token string) {
				mockDB.ExpectExec("DELETE FROM csrf_tokens WHERE token").
					WithArgs(token).
					WillReturnResult(pgxmock.NewResult("DELETE", 0))
			},
			wantErr:  true,
			errorMsg: "CSRF token not found",
		},
		{
			name:  "database error during deletion",
			token: "valid-csrf-token",
			setupDB: func(mockDB pgxmock.PgxPoolIface, token string) {
				mockDB.ExpectExec("DELETE FROM csrf_tokens WHERE token").
					WithArgs(token).
					WillReturnError(pgx.ErrTxClosed)
			},
			wantErr:  true,
			errorMsg: "failed to delete CSRF token",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, mockDB := createTestAuthRepository(t)
			defer mockDB.Close()

			tt.setupDB(mockDB, tt.token)

			err := repo.DeleteCSRFToken(context.Background(), tt.token)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				assert.NoError(t, err)
			}

			assert.NoError(t, mockDB.ExpectationsWereMet())
		})
	}
}