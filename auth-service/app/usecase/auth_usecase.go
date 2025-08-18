package usecase

import (
	"context"
	"fmt"
	"time"

	"auth-service/app/domain"
	"auth-service/app/port"

	"github.com/google/uuid"
)

// AuthUseCase implements authentication business logic
type AuthUseCase struct {
	authRepo    port.AuthRepository
	authGateway port.AuthGateway
}

// NewAuthUseCase creates a new AuthUseCase instance
func NewAuthUseCase(authRepo port.AuthRepository, authGateway port.AuthGateway) *AuthUseCase {
	return &AuthUseCase{
		authRepo:    authRepo,
		authGateway: authGateway,
	}
}

// CreateSession creates a new session for a user
func (uc *AuthUseCase) CreateSession(ctx context.Context, userID uuid.UUID, kratosSessionID string, duration time.Duration) (*domain.Session, error) {
	// Create session using domain logic
	session, err := domain.NewSession(userID, kratosSessionID, duration)
	if err != nil {
		return nil, err
	}

	// Store session in repository
	err = uc.authRepo.CreateSession(ctx, session)
	if err != nil {
		return nil, err
	}

	return session, nil
}

// ValidateSession validates a session and returns session context
func (uc *AuthUseCase) ValidateSession(ctx context.Context, sessionToken string) (*domain.SessionContext, error) {
	// Get session from Kratos
	kratosSession, err := uc.authGateway.GetSession(ctx, sessionToken)
	if err != nil {
		return nil, err
	}

	return uc.buildSessionContext(ctx, kratosSession)
}

// ValidateSessionWithCookie validates a session using Cookie header (TODO.md修正)
func (uc *AuthUseCase) ValidateSessionWithCookie(ctx context.Context, cookieHeader string) (*domain.SessionContext, error) {
	// whoami call with Cookie (per TODO.md instructions)
	kratosSession, err := uc.authGateway.WhoAmI(ctx, cookieHeader)
	if err != nil {
		return nil, err
	}

	return uc.buildSessionContext(ctx, kratosSession)
}

// buildSessionContext builds session context from Kratos session (共通ロジック)
func (uc *AuthUseCase) buildSessionContext(ctx context.Context, kratosSession *domain.KratosSession) (*domain.SessionContext, error) {
	// Get our session
	session, err := uc.authRepo.GetSessionByKratosID(ctx, kratosSession.ID)
	if err != nil {
		return nil, err
	}

	// Check if session is valid
	if !session.IsValid() {
		return nil, fmt.Errorf("session is not valid")
	}

	// Create session context
	sessionContext := &domain.SessionContext{
		UserID:          session.UserID,
		TenantID:        uuid.Nil, // TODO: Get from user profile
		Email:           kratosSession.Identity.GetEmail(),
		Name:            kratosSession.Identity.GetName(),
		Role:            domain.UserRoleUser, // TODO: Get from user profile
		SessionID:       session.ID.String(),
		KratosSessionID: session.KratosSessionID,
		IsActive:        session.Active,
		ExpiresAt:       session.ExpiresAt,
		LastActivityAt:  session.LastActivityAt,
	}

	return sessionContext, nil
}

// DeactivateSession deactivates a session
func (uc *AuthUseCase) DeactivateSession(ctx context.Context, kratosSessionID string) error {
	// Get session from repository
	session, err := uc.authRepo.GetSessionByKratosID(ctx, kratosSessionID)
	if err != nil {
		return err
	}

	// Update session status to inactive
	err = uc.authRepo.UpdateSessionStatus(ctx, session.ID.String(), false)
	if err != nil {
		return err
	}

	return nil
}

// InitiateLogin creates a new login flow
func (uc *AuthUseCase) InitiateLogin(ctx context.Context) (*domain.LoginFlow, error) {
	return uc.authGateway.CreateLoginFlow(ctx)
}

// InitiateRegistration creates a new registration flow
func (uc *AuthUseCase) InitiateRegistration(ctx context.Context) (*domain.RegistrationFlow, error) {
	return uc.authGateway.CreateRegistrationFlow(ctx)
}

// CompleteLogin completes a login flow
func (uc *AuthUseCase) CompleteLogin(ctx context.Context, flowID string, body interface{}) (*domain.SessionContext, error) {
	// Submit login flow to Kratos
	kratosSession, err := uc.authGateway.SubmitLoginFlow(ctx, flowID, body)
	if err != nil {
		return nil, err
	}

	// Create session in our database
	userID, err := uuid.Parse(kratosSession.Identity.ID)
	if err != nil {
		return nil, err
	}

	session, err := uc.CreateSession(ctx, userID, kratosSession.ID, 24*time.Hour)
	if err != nil {
		return nil, err
	}

	// Create session context
	sessionContext := &domain.SessionContext{
		UserID:          userID,
		TenantID:        uuid.Nil, // TODO: Get from user profile
		Email:           kratosSession.Identity.GetEmail(),
		Name:            kratosSession.Identity.GetName(),
		Role:            domain.UserRoleUser, // TODO: Get from user profile
		SessionID:       session.ID.String(),
		KratosSessionID: session.KratosSessionID,
		IsActive:        session.Active,
		ExpiresAt:       session.ExpiresAt,
		LastActivityAt:  session.LastActivityAt,
	}

	return sessionContext, nil
}

// CompleteRegistration completes a registration flow
func (uc *AuthUseCase) CompleteRegistration(ctx context.Context, flowID string, body interface{}) (*domain.SessionContext, error) {
	// Submit registration flow to Kratos
	kratosSession, err := uc.authGateway.SubmitRegistrationFlow(ctx, flowID, body)
	if err != nil {
		return nil, err
	}

	// Create session in our database
	userID, err := uuid.Parse(kratosSession.Identity.ID)
	if err != nil {
		return nil, err
	}

	session, err := uc.CreateSession(ctx, userID, kratosSession.ID, 24*time.Hour)
	if err != nil {
		return nil, err
	}

	// Create session context
	sessionContext := &domain.SessionContext{
		UserID:          userID,
		TenantID:        uuid.Nil, // TODO: Get from user profile
		Email:           kratosSession.Identity.GetEmail(),
		Name:            kratosSession.Identity.GetName(),
		Role:            domain.UserRoleUser, // TODO: Get from user profile
		SessionID:       session.ID.String(),
		KratosSessionID: session.KratosSessionID,
		IsActive:        session.Active,
		ExpiresAt:       session.ExpiresAt,
		LastActivityAt:  session.LastActivityAt,
	}

	return sessionContext, nil
}

// Logout logs out a user by deactivating their session
func (uc *AuthUseCase) Logout(ctx context.Context, sessionID string) error {
	// Revoke session in Kratos
	err := uc.authGateway.RevokeSession(ctx, sessionID)
	if err != nil {
		return err
	}

	// Deactivate session in our database
	return uc.DeactivateSession(ctx, sessionID)
}

// RefreshSession refreshes a session
func (uc *AuthUseCase) RefreshSession(ctx context.Context, sessionID string) (*domain.SessionContext, error) {
	// Get session from Kratos
	kratosSession, err := uc.authGateway.GetSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	// Get our session
	session, err := uc.authRepo.GetSessionByKratosID(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	// Update session activity
	session.UpdateActivity()
	err = uc.authRepo.UpdateSessionStatus(ctx, session.ID.String(), true)
	if err != nil {
		return nil, err
	}

	// Create session context
	sessionContext := &domain.SessionContext{
		UserID:          session.UserID,
		TenantID:        uuid.Nil, // TODO: Get from user profile
		Email:           kratosSession.Identity.GetEmail(),
		Name:            kratosSession.Identity.GetName(),
		Role:            domain.UserRoleUser, // TODO: Get from user profile
		SessionID:       session.ID.String(),
		KratosSessionID: session.KratosSessionID,
		IsActive:        session.Active,
		ExpiresAt:       session.ExpiresAt,
		LastActivityAt:  session.LastActivityAt,
	}

	return sessionContext, nil
}

// GenerateCSRFToken generates a new CSRF token for a session
func (uc *AuthUseCase) GenerateCSRFToken(ctx context.Context, sessionID string) (*domain.CSRFToken, error) {
	// Create CSRF token using domain logic
	csrfToken, err := domain.NewCSRFToken(sessionID, 32, 1*time.Hour)
	if err != nil {
		return nil, err
	}

	// Store token in repository
	err = uc.authRepo.StoreCSRFToken(ctx, csrfToken)
	if err != nil {
		return nil, err
	}

	return csrfToken, nil
}

// ValidateCSRFToken validates a CSRF token
func (uc *AuthUseCase) ValidateCSRFToken(ctx context.Context, token, sessionID string) error {
	// Get token from repository
	csrfToken, err := uc.authRepo.GetCSRFToken(ctx, token)
	if err != nil {
		return err
	}

	// Validate token
	err = csrfToken.Validate(token)
	if err != nil {
		return err
	}

	// Delete token after validation (one-time use)
	return uc.authRepo.DeleteCSRFToken(ctx, token)
}
