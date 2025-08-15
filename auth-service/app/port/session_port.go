package port

import (
	"context"

	"auth-service/app/domain"
	"github.com/google/uuid"
)

// SessionUsecase defines session management business logic interface
type SessionUsecase interface {
	// Session management
	CreateSession(ctx context.Context, userID uuid.UUID, kratosSessionID string) (*domain.Session, error)
	GetSession(ctx context.Context, sessionID string) (*domain.Session, error)
	GetSessionByKratosID(ctx context.Context, kratosSessionID string) (*domain.Session, error)
	ValidateSession(ctx context.Context, sessionID string) (*domain.SessionContext, error)
	RefreshSession(ctx context.Context, sessionID string) (*domain.Session, error)
	DeactivateSession(ctx context.Context, sessionID string) error
	CleanupExpiredSessions(ctx context.Context) error

	// Activity tracking
	UpdateSessionActivity(ctx context.Context, sessionID string) error
	GetActiveSessions(ctx context.Context, userID uuid.UUID) ([]*domain.Session, error)
}

// SessionGateway defines session gateway interface
type SessionGateway interface {
	// Session operations
	CreateSession(ctx context.Context, session *domain.Session) error
	GetSessionByID(ctx context.Context, sessionID string) (*domain.Session, error)
	GetSessionByKratosID(ctx context.Context, kratosSessionID string) (*domain.Session, error)
	UpdateSession(ctx context.Context, session *domain.Session) error
	DeleteSession(ctx context.Context, sessionID string) error
	ListActiveSessionsByUser(ctx context.Context, userID uuid.UUID) ([]*domain.Session, error)
	DeleteExpired(ctx context.Context) (int, error)
}

// SessionRepositoryPort defines session data access interface
type SessionRepositoryPort interface {
	Create(ctx context.Context, session *domain.Session) error
	GetByID(ctx context.Context, sessionID string) (*domain.Session, error)
	GetByKratosID(ctx context.Context, kratosSessionID string) (*domain.Session, error)
	Update(ctx context.Context, session *domain.Session) error
	Delete(ctx context.Context, sessionID string) error
	ListActiveByUser(ctx context.Context, userID uuid.UUID) ([]*domain.Session, error)
	DeleteExpired(ctx context.Context) error
	// 🔄 Phase 3.2: セッション同期関連メソッド
	GetActiveSessions(ctx context.Context) ([]*domain.Session, error)
	GetSessionCount(ctx context.Context) (int, error)
	GetSessionsBySyncStatus(ctx context.Context, status domain.SessionSyncStatus) ([]*domain.Session, error)
}

// SessionRepository is an alias for backward compatibility
type SessionRepository = SessionRepositoryPort
