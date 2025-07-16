package gateway

import (
	"context"
	"fmt"
	"log/slog"

	"auth-service/app/domain"
	"auth-service/app/port"

	"github.com/google/uuid"
)

// AuthGateway implements port.AuthGateway interface
// It acts as an anti-corruption layer between the domain and external auth services
type AuthGateway struct {
	kratosClient port.KratosClient
	logger       *slog.Logger
}

// NewAuthGateway creates a new AuthGateway instance
func NewAuthGateway(kratosClient port.KratosClient, logger *slog.Logger) *AuthGateway {
	return &AuthGateway{
		kratosClient: kratosClient,
		logger:       logger.With("component", "auth_gateway"),
	}
}

// CreateLoginFlow creates a new login flow with Kratos
func (g *AuthGateway) CreateLoginFlow(ctx context.Context) (*domain.LoginFlow, error) {
	g.logger.Info("creating login flow")

	// Default values for basic login flow
	tenantID := uuid.New() // TODO: Get tenant ID from context
	refresh := false
	returnTo := ""

	kratosFlow, err := g.kratosClient.CreateLoginFlow(ctx, tenantID, refresh, returnTo)
	if err != nil {
		g.logger.Error("failed to create login flow", "error", err)
		return nil, fmt.Errorf("failed to create login flow: %w", err)
	}

	g.logger.Info("login flow created successfully", "flow_id", kratosFlow.ID)
	return kratosFlow, nil
}

// CreateRegistrationFlow creates a new registration flow with Kratos
func (g *AuthGateway) CreateRegistrationFlow(ctx context.Context) (*domain.RegistrationFlow, error) {
	g.logger.Info("creating registration flow")

	// Default values for basic registration flow
	tenantID := uuid.New() // TODO: Get tenant ID from context
	returnTo := ""

	kratosFlow, err := g.kratosClient.CreateRegistrationFlow(ctx, tenantID, returnTo)
	if err != nil {
		g.logger.Error("failed to create registration flow", "error", err)
		return nil, fmt.Errorf("failed to create registration flow: %w", err)
	}

	g.logger.Info("registration flow created successfully", "flow_id", kratosFlow.ID)
	return kratosFlow, nil
}

// SubmitLoginFlow submits a login flow to Kratos
func (g *AuthGateway) SubmitLoginFlow(ctx context.Context, flowID string, body interface{}) (*domain.KratosSession, error) {
	g.logger.Info("submitting login flow", "flow_id", flowID)

	// Convert body to map[string]interface{} for Kratos client
	bodyMap, ok := body.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid body type: expected map[string]interface{}, got %T", body)
	}

	kratosSession, err := g.kratosClient.SubmitLoginFlow(ctx, flowID, bodyMap)
	if err != nil {
		g.logger.Error("failed to submit login flow", "flow_id", flowID, "error", err)
		return nil, fmt.Errorf("failed to submit login flow: %w", err)
	}

	g.logger.Info("login flow submitted successfully",
		"flow_id", flowID,
		"session_id", kratosSession.ID,
		"user_id", kratosSession.Identity.ID)

	return kratosSession, nil
}

// SubmitRegistrationFlow submits a registration flow to Kratos
func (g *AuthGateway) SubmitRegistrationFlow(ctx context.Context, flowID string, body interface{}) (*domain.KratosSession, error) {
	g.logger.Info("submitting registration flow", "flow_id", flowID)

	// Convert body to map[string]interface{} for Kratos client
	bodyMap, ok := body.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid body type: expected map[string]interface{}, got %T", body)
	}

	kratosSession, err := g.kratosClient.SubmitRegistrationFlow(ctx, flowID, bodyMap)
	if err != nil {
		g.logger.Error("failed to submit registration flow", "flow_id", flowID, "error", err)
		return nil, fmt.Errorf("failed to submit registration flow: %w", err)
	}

	g.logger.Info("registration flow submitted successfully",
		"flow_id", flowID,
		"session_id", kratosSession.ID,
		"user_id", kratosSession.Identity.ID)

	return kratosSession, nil
}

// GetSession retrieves a session from Kratos
func (g *AuthGateway) GetSession(ctx context.Context, sessionToken string) (*domain.KratosSession, error) {
	g.logger.Info("retrieving session")

	kratosSession, err := g.kratosClient.GetSession(ctx, sessionToken)
	if err != nil {
		g.logger.Error("failed to retrieve session", "error", err)
		return nil, fmt.Errorf("failed to retrieve session: %w", err)
	}

	g.logger.Info("session retrieved successfully",
		"session_id", kratosSession.ID,
		"user_id", kratosSession.Identity.ID)

	return kratosSession, nil
}

// RevokeSession revokes a session in Kratos
func (g *AuthGateway) RevokeSession(ctx context.Context, sessionID string) error {
	g.logger.Info("revoking session", "session_id", sessionID)

	err := g.kratosClient.RevokeSession(ctx, sessionID)
	if err != nil {
		g.logger.Error("failed to revoke session", "session_id", sessionID, "error", err)
		return fmt.Errorf("failed to revoke session: %w", err)
	}

	g.logger.Info("session revoked successfully", "session_id", sessionID)
	return nil
}
