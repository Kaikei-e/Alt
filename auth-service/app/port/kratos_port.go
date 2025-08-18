package port

//go:generate mockgen -source=kratos_port.go -destination=../mocks/mock_kratos_port.go

import (
	"context"
	"net/http"

	"auth-service/app/domain"
	"github.com/google/uuid"
)

// KratosClient defines interface for Kratos client operations
type KratosClient interface {
	// Flow management
	CreateLoginFlow(ctx context.Context, tenantID uuid.UUID, refresh bool, returnTo string) (*domain.LoginFlow, error)
	GetLoginFlow(ctx context.Context, flowID string) (*domain.LoginFlow, error)
	SubmitLoginFlow(ctx context.Context, flowID string, body map[string]interface{}) (*domain.KratosSession, error)

	CreateRegistrationFlow(ctx context.Context, tenantID uuid.UUID, returnTo string) (*domain.RegistrationFlow, error)
	GetRegistrationFlow(ctx context.Context, flowID string) (*domain.RegistrationFlow, error)
	SubmitRegistrationFlow(ctx context.Context, flowID string, body map[string]interface{}) (*domain.KratosSession, error)

	CreateLogoutFlow(ctx context.Context, sessionToken string, tenantID uuid.UUID, returnTo string) (*domain.LogoutFlow, error)
	SubmitLogoutFlow(ctx context.Context, token string, returnTo string) error

	CreateRecoveryFlow(ctx context.Context, tenantID uuid.UUID, returnTo string) (*domain.RecoveryFlow, error)
	GetRecoveryFlow(ctx context.Context, flowID string) (*domain.RecoveryFlow, error)
	SubmitRecoveryFlow(ctx context.Context, flowID string, body map[string]interface{}) (*domain.RecoveryFlow, error)

	CreateVerificationFlow(ctx context.Context, tenantID uuid.UUID, returnTo string) (*domain.VerificationFlow, error)
	GetVerificationFlow(ctx context.Context, flowID string) (*domain.VerificationFlow, error)
	SubmitVerificationFlow(ctx context.Context, flowID string, body map[string]interface{}) (*domain.VerificationFlow, error)

	CreateSettingsFlow(ctx context.Context, sessionToken string, tenantID uuid.UUID, returnTo string) (*domain.SettingsFlow, error)
	GetSettingsFlow(ctx context.Context, flowID string) (*domain.SettingsFlow, error)
	SubmitSettingsFlow(ctx context.Context, flowID string, sessionToken string, body map[string]interface{}) (*domain.SettingsFlow, error)

	// Session management
	CreateSession(ctx context.Context, identityID string, sessionToken string) (*domain.KratosSession, error)
	GetSession(ctx context.Context, sessionToken string) (*domain.KratosSession, error)
	WhoAmI(ctx context.Context, cookieHeader string) (*domain.KratosSession, error)
	RevokeSession(ctx context.Context, sessionID string) error
	ListSessions(ctx context.Context, identityID string) ([]*domain.KratosSession, error)

	// Identity management
	CreateIdentity(ctx context.Context, traits map[string]interface{}, schemaID string) (*domain.KratosIdentity, error)
	GetIdentity(ctx context.Context, identityID string) (*domain.KratosIdentity, error)
	UpdateIdentity(ctx context.Context, identityID string, traits map[string]interface{}) (*domain.KratosIdentity, error)
	DeleteIdentity(ctx context.Context, identityID string) error
	ListIdentities(ctx context.Context, page, perPage int) ([]*domain.KratosIdentity, error)

	// Administrative operations
	AdminCreateIdentity(ctx context.Context, identity *domain.KratosIdentity) (*domain.KratosIdentity, error)
	AdminUpdateIdentity(ctx context.Context, identityID string, identity *domain.KratosIdentity) (*domain.KratosIdentity, error)
	AdminDeleteIdentity(ctx context.Context, identityID string) error

	// Health and status
	Health(ctx context.Context) error
	Ready(ctx context.Context) error
	Version(ctx context.Context) (map[string]interface{}, error)

	// Utilities
	ParseSessionFromRequest(r *http.Request) (string, error)
	ParseSessionFromCookie(cookies []*http.Cookie) (string, error)
}

// KratosAdminClient defines interface for Kratos admin client operations
type KratosAdminClient interface {
	// Identity management
	CreateIdentity(ctx context.Context, traits map[string]interface{}, schemaID string) (*domain.KratosIdentity, error)
	GetIdentity(ctx context.Context, identityID string) (*domain.KratosIdentity, error)
	UpdateIdentity(ctx context.Context, identityID string, traits map[string]interface{}) (*domain.KratosIdentity, error)
	DeleteIdentity(ctx context.Context, identityID string) error
	ListIdentities(ctx context.Context, page, perPage int) ([]*domain.KratosIdentity, error)

	// Session management
	CreateSession(ctx context.Context, identityID string) (*domain.KratosSession, error)
	GetSession(ctx context.Context, sessionID string) (*domain.KratosSession, error)
	RevokeSession(ctx context.Context, sessionID string) error
	ListSessions(ctx context.Context, identityID string) ([]*domain.KratosSession, error)

	// Flow management
	GetLoginFlow(ctx context.Context, flowID string) (*domain.LoginFlow, error)
	GetRegistrationFlow(ctx context.Context, flowID string) (*domain.RegistrationFlow, error)
	GetRecoveryFlow(ctx context.Context, flowID string) (*domain.RecoveryFlow, error)
	GetVerificationFlow(ctx context.Context, flowID string) (*domain.VerificationFlow, error)
	GetSettingsFlow(ctx context.Context, flowID string) (*domain.SettingsFlow, error)

	// Health and status
	Health(ctx context.Context) error
	Ready(ctx context.Context) error
	Version(ctx context.Context) (map[string]interface{}, error)
}

// KratosPublicClient defines interface for Kratos public client operations
type KratosPublicClient interface {
	// Flow management
	CreateLoginFlow(ctx context.Context, refresh bool, returnTo string) (*domain.LoginFlow, error)
	GetLoginFlow(ctx context.Context, flowID string) (*domain.LoginFlow, error)
	SubmitLoginFlow(ctx context.Context, flowID string, body map[string]interface{}) (*domain.KratosSession, error)

	CreateRegistrationFlow(ctx context.Context, returnTo string) (*domain.RegistrationFlow, error)
	GetRegistrationFlow(ctx context.Context, flowID string) (*domain.RegistrationFlow, error)
	SubmitRegistrationFlow(ctx context.Context, flowID string, body map[string]interface{}) (*domain.KratosSession, error)

	CreateLogoutFlow(ctx context.Context, sessionToken string, returnTo string) (*domain.LogoutFlow, error)
	SubmitLogoutFlow(ctx context.Context, token string, returnTo string) error

	CreateRecoveryFlow(ctx context.Context, returnTo string) (*domain.RecoveryFlow, error)
	GetRecoveryFlow(ctx context.Context, flowID string) (*domain.RecoveryFlow, error)
	SubmitRecoveryFlow(ctx context.Context, flowID string, body map[string]interface{}) (*domain.RecoveryFlow, error)

	CreateVerificationFlow(ctx context.Context, returnTo string) (*domain.VerificationFlow, error)
	GetVerificationFlow(ctx context.Context, flowID string) (*domain.VerificationFlow, error)
	SubmitVerificationFlow(ctx context.Context, flowID string, body map[string]interface{}) (*domain.VerificationFlow, error)

	CreateSettingsFlow(ctx context.Context, sessionToken string, returnTo string) (*domain.SettingsFlow, error)
	GetSettingsFlow(ctx context.Context, flowID string) (*domain.SettingsFlow, error)
	SubmitSettingsFlow(ctx context.Context, flowID string, sessionToken string, body map[string]interface{}) (*domain.SettingsFlow, error)

	// Session management
	WhoAmI(ctx context.Context, cookieHeader string) (*domain.KratosSession, error)

	// Health and status
	Health(ctx context.Context) error
	Ready(ctx context.Context) error
	Version(ctx context.Context) (map[string]interface{}, error)
}
