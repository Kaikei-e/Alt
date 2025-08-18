package port

//go:generate mockgen -source=auth_port.go -destination=../mocks/mock_auth_port.go

import (
	"context"

	"auth-service/app/domain"
)

// AuthUsecase defines authentication business logic interface
type AuthUsecase interface {
	// Authentication flows
	InitiateLogin(ctx context.Context) (*domain.LoginFlow, error)
	InitiateRegistration(ctx context.Context) (*domain.RegistrationFlow, error)
	CompleteLogin(ctx context.Context, flowID string, body interface{}) (*domain.SessionContext, error)
	CompleteRegistration(ctx context.Context, flowID string, body interface{}) (*domain.SessionContext, error)
	Logout(ctx context.Context, sessionID string) error

	// Session management
	ValidateSession(ctx context.Context, sessionToken string) (*domain.SessionContext, error)
	ValidateSessionWithCookie(ctx context.Context, cookieHeader string) (*domain.SessionContext, error)
	RefreshSession(ctx context.Context, sessionID string) (*domain.SessionContext, error)

	// CSRF protection
	GenerateCSRFToken(ctx context.Context, sessionID string) (*domain.CSRFToken, error)
	ValidateCSRFToken(ctx context.Context, token, sessionID string) error
}

// AuthGateway defines authentication gateway interface
type AuthGateway interface {
	// Kratos integration
	CreateLoginFlow(ctx context.Context) (*domain.LoginFlow, error)
	CreateRegistrationFlow(ctx context.Context) (*domain.RegistrationFlow, error)
	SubmitLoginFlow(ctx context.Context, flowID string, body interface{}) (*domain.KratosSession, error)
	SubmitRegistrationFlow(ctx context.Context, flowID string, body interface{}) (*domain.KratosSession, error)
	GetSession(ctx context.Context, sessionToken string) (*domain.KratosSession, error)
	WhoAmI(ctx context.Context, cookieHeader string) (*domain.KratosSession, error)
	RevokeSession(ctx context.Context, sessionID string) error
}

// AuthRepository defines authentication data access interface
type AuthRepository interface {
	// Session management
	CreateSession(ctx context.Context, session *domain.Session) error
	GetSessionByKratosID(ctx context.Context, kratosSessionID string) (*domain.Session, error)
	GetActiveSessionByUserID(ctx context.Context, userID string) (*domain.Session, error)
	UpdateSessionStatus(ctx context.Context, sessionID string, active bool) error
	DeleteSession(ctx context.Context, sessionID string) error

	// CSRF token management
	StoreCSRFToken(ctx context.Context, token *domain.CSRFToken) error
	GetCSRFToken(ctx context.Context, token string) (*domain.CSRFToken, error)
	DeleteCSRFToken(ctx context.Context, token string) error
}
