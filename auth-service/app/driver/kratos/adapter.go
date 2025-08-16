package kratos

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"auth-service/app/domain"
	"auth-service/app/port"
	"github.com/google/uuid"
	kratosclient "github.com/ory/kratos-client-go"
)

// KratosClientAdapter adapts our kratos.Client to implement port.KratosClient
type KratosClientAdapter struct {
	client *Client
	logger *slog.Logger
}

// NewKratosClientAdapter creates a new adapter
func NewKratosClientAdapter(client *Client, logger *slog.Logger) port.KratosClient {
	return &KratosClientAdapter{
		client: client,
		logger: logger,
	}
}

// Flow management methods
func (a *KratosClientAdapter) CreateLoginFlow(ctx context.Context, tenantID uuid.UUID, refresh bool, returnTo string) (*domain.LoginFlow, error) {
	a.logger.Info("creating login flow in Kratos",
		"tenant_id", tenantID,
		"refresh", refresh,
		"return_to", returnTo)

	// Create login flow request
	req := a.client.PublicAPI().FrontendAPI.CreateBrowserLoginFlow(ctx)
	if refresh {
		req = req.Refresh(refresh)
	}
	if returnTo != "" {
		req = req.ReturnTo(returnTo)
	}

	// Execute the request
	resp, httpResp, err := req.Execute()
	if err != nil {
		a.logger.Error("kratos login flow creation failed",
			"tenant_id", tenantID,
			"error", err,
			"http_status", getHTTPStatus(httpResp))
		return nil, a.transformKratosError(err, httpResp, "login_flow_create")
	}

	a.logger.Info("login flow created successfully",
		"flow_id", resp.Id,
		"tenant_id", tenantID)

	// Transform response to domain login flow
	return a.transformKratosLoginFlowResponse(resp, tenantID)
}

func (a *KratosClientAdapter) GetLoginFlow(ctx context.Context, flowID string) (*domain.LoginFlow, error) {
	// TODO: Implement actual Kratos integration
	return &domain.LoginFlow{
		ID: flowID,
		// Add other required fields...
	}, nil
}

func (a *KratosClientAdapter) SubmitLoginFlow(ctx context.Context, flowID string, body map[string]interface{}) (*domain.KratosSession, error) {
	a.logger.Info("submitting login flow to Kratos",
		"flow_id", flowID,
		"body_fields", getBodyFieldNames(body))

	// Transform body to Kratos login body format
	kratosBody, err := a.transformToKratosLoginBody(body)
	if err != nil {
		a.logger.Error("failed to transform login body",
			"flow_id", flowID,
			"error", err)
		return nil, fmt.Errorf("failed to transform login body: %w", err)
	}

	// Submit login flow to Kratos
	// X18.md HAR Analysis Fix: Use correct Kratos client pattern
	passwordMethod, ok := kratosBody.(kratosclient.UpdateLoginFlowWithPasswordMethod)
	if !ok {
		return nil, fmt.Errorf("invalid login body type: %T", kratosBody)
	}

	// 🚨 CRITICAL: Pre-submission CSRF validation and logging
	a.logger.Info("preparing Kratos login submission",
		"flow_id", flowID,
		"method", passwordMethod.Method,
		"identifier_present", passwordMethod.Identifier != "",
		"password_present", passwordMethod.Password != "",
		"csrf_token_ptr_nil", passwordMethod.CsrfToken == nil,
		"csrf_token_value", func() string {
			if passwordMethod.CsrfToken != nil {
				return getSafePrefix(*passwordMethod.CsrfToken, 8) + "..." + getSafeSuffix(*passwordMethod.CsrfToken, 8)
			}
			return "nil"
		}())

	// Use correct Kratos client pattern with AsUpdateLoginFlowBody conversion
	resp, httpResp, err := a.client.PublicAPI().FrontendAPI.
		UpdateLoginFlow(ctx).
		Flow(flowID).
		UpdateLoginFlowBody(kratosclient.UpdateLoginFlowWithPasswordMethodAsUpdateLoginFlowBody(&passwordMethod)).
		Execute()

	if err != nil {
		a.logger.Error("kratos login flow submission failed",
			"flow_id", flowID,
			"error", err,
			"http_status", getHTTPStatus(httpResp))
		return nil, a.transformKratosError(err, httpResp, "login_flow_submit")
	}

	a.logger.Info("login flow submitted successfully",
		"flow_id", flowID,
		"session_id", getSessionID(resp))

	// Transform response to domain session
	return a.transformKratosSessionResponse(resp)
}

func (a *KratosClientAdapter) CreateRegistrationFlow(ctx context.Context, tenantID uuid.UUID, returnTo string) (*domain.RegistrationFlow, error) {
	a.logger.Info("creating registration flow in Kratos",
		"tenant_id", tenantID,
		"return_to", returnTo)

	// Create registration flow request
	req := a.client.PublicAPI().FrontendAPI.CreateBrowserRegistrationFlow(ctx)
	if returnTo != "" {
		req = req.ReturnTo(returnTo)
	}

	// Execute the request
	resp, httpResp, err := req.Execute()
	if err != nil {
		a.logger.Error("kratos registration flow creation failed",
			"tenant_id", tenantID,
			"error", err,
			"http_status", getHTTPStatus(httpResp))
		return nil, a.transformKratosError(err, httpResp, "registration_flow_create")
	}

	a.logger.Info("registration flow created successfully",
		"flow_id", resp.Id,
		"tenant_id", tenantID)

	// Transform response to domain registration flow
	return a.transformKratosRegistrationFlowResponse(resp, tenantID)
}

func (a *KratosClientAdapter) GetRegistrationFlow(ctx context.Context, flowID string) (*domain.RegistrationFlow, error) {
	// TODO: Implement actual Kratos integration
	return &domain.RegistrationFlow{
		ID: flowID,
		// Add other required fields...
	}, nil
}

func (a *KratosClientAdapter) SubmitRegistrationFlow(ctx context.Context, flowID string, body map[string]interface{}) (*domain.KratosSession, error) {
	a.logger.Info("submitting registration flow to Kratos",
		"flow_id", flowID,
		"body_fields", getBodyFieldNames(body))

	// Transform body to Kratos registration body format
	kratosBody, err := a.transformToKratosRegistrationBody(body)
	if err != nil {
		a.logger.Error("failed to transform registration body",
			"flow_id", flowID,
			"error", err)
		return nil, fmt.Errorf("failed to transform registration body: %w", err)
	}

	// Submit registration flow to Kratos
	// Fix: Use correct Kratos client pattern with AsUpdateRegistrationFlowBody conversion
	passwordMethod, ok := kratosBody.(kratosclient.UpdateRegistrationFlowWithPasswordMethod)
	if !ok {
		return nil, fmt.Errorf("invalid registration body type: %T", kratosBody)
	}

	resp, httpResp, err := a.client.PublicAPI().FrontendAPI.
		UpdateRegistrationFlow(ctx).
		Flow(flowID).
		UpdateRegistrationFlowBody(kratosclient.UpdateRegistrationFlowWithPasswordMethodAsUpdateRegistrationFlowBody(&passwordMethod)).
		Execute()

	if err != nil {
		a.logger.Error("kratos registration flow submission failed",
			"flow_id", flowID,
			"error", err,
			"http_status", getHTTPStatus(httpResp))
		return nil, a.transformKratosError(err, httpResp, "registration_flow_submit")
	}

	a.logger.Info("registration flow submitted successfully",
		"flow_id", flowID,
		"session_id", getSessionID(resp))

	// Transform response to domain session
	return a.transformKratosSessionResponse(resp)
}

func (a *KratosClientAdapter) CreateLogoutFlow(ctx context.Context, sessionToken string, tenantID uuid.UUID, returnTo string) (*domain.LogoutFlow, error) {
	// TODO: Implement actual Kratos integration
	return &domain.LogoutFlow{
		ID: uuid.New().String(),
		// Add other required fields...
	}, nil
}

func (a *KratosClientAdapter) SubmitLogoutFlow(ctx context.Context, token string, returnTo string) error {
	// TODO: Implement actual Kratos integration
	return nil
}

func (a *KratosClientAdapter) CreateRecoveryFlow(ctx context.Context, tenantID uuid.UUID, returnTo string) (*domain.RecoveryFlow, error) {
	// TODO: Implement actual Kratos integration
	return &domain.RecoveryFlow{
		ID: uuid.New().String(),
		// Add other required fields...
	}, nil
}

func (a *KratosClientAdapter) GetRecoveryFlow(ctx context.Context, flowID string) (*domain.RecoveryFlow, error) {
	// TODO: Implement actual Kratos integration
	return &domain.RecoveryFlow{
		ID: flowID,
		// Add other required fields...
	}, nil
}

func (a *KratosClientAdapter) SubmitRecoveryFlow(ctx context.Context, flowID string, body map[string]interface{}) (*domain.RecoveryFlow, error) {
	// TODO: Implement actual Kratos integration
	return &domain.RecoveryFlow{
		ID: flowID,
		// Add other required fields...
	}, nil
}

func (a *KratosClientAdapter) CreateVerificationFlow(ctx context.Context, tenantID uuid.UUID, returnTo string) (*domain.VerificationFlow, error) {
	// TODO: Implement actual Kratos integration
	return &domain.VerificationFlow{
		ID: uuid.New().String(),
		// Add other required fields...
	}, nil
}

func (a *KratosClientAdapter) GetVerificationFlow(ctx context.Context, flowID string) (*domain.VerificationFlow, error) {
	// TODO: Implement actual Kratos integration
	return &domain.VerificationFlow{
		ID: flowID,
		// Add other required fields...
	}, nil
}

func (a *KratosClientAdapter) SubmitVerificationFlow(ctx context.Context, flowID string, body map[string]interface{}) (*domain.VerificationFlow, error) {
	// TODO: Implement actual Kratos integration
	return &domain.VerificationFlow{
		ID: flowID,
		// Add other required fields...
	}, nil
}

func (a *KratosClientAdapter) CreateSettingsFlow(ctx context.Context, sessionToken string, tenantID uuid.UUID, returnTo string) (*domain.SettingsFlow, error) {
	// TODO: Implement actual Kratos integration
	return &domain.SettingsFlow{
		ID: uuid.New().String(),
		// Add other required fields...
	}, nil
}

func (a *KratosClientAdapter) GetSettingsFlow(ctx context.Context, flowID string) (*domain.SettingsFlow, error) {
	// TODO: Implement actual Kratos integration
	return &domain.SettingsFlow{
		ID: flowID,
		// Add other required fields...
	}, nil
}

func (a *KratosClientAdapter) SubmitSettingsFlow(ctx context.Context, flowID string, sessionToken string, body map[string]interface{}) (*domain.SettingsFlow, error) {
	// TODO: Implement actual Kratos integration
	return &domain.SettingsFlow{
		ID: flowID,
		// Add other required fields...
	}, nil
}

// Session management methods
func (a *KratosClientAdapter) CreateSession(ctx context.Context, identityID string, sessionToken string) (*domain.KratosSession, error) {
	// TODO: Implement actual Kratos integration
	return &domain.KratosSession{
		ID: uuid.New().String(),
		Identity: &domain.KratosIdentity{
			ID: identityID,
			Traits: map[string]interface{}{
				"email": "test@example.com",
			},
		},
		// Add other required fields...
	}, nil
}

func (a *KratosClientAdapter) GetSession(ctx context.Context, sessionToken string) (*domain.KratosSession, error) {
	a.logger.Info("getting session from Kratos",
		"session_token_present", sessionToken != "")

	// Get session from Kratos
	resp, httpResp, err := a.client.PublicAPI().FrontendAPI.
		ToSession(ctx).
		XSessionToken(sessionToken).
		Execute()

	if err != nil {
		a.logger.Error("kratos get session failed",
			"error", err,
			"http_status", getHTTPStatus(httpResp))
		return nil, a.transformKratosError(err, httpResp, "get_session")
	}

	a.logger.Info("session retrieved successfully",
		"session_id", resp.Id,
		"identity_id", getIdentityID(resp.Identity))

	// Transform response to domain session
	return a.transformSessionToDomain(resp)
}

func (a *KratosClientAdapter) WhoAmI(ctx context.Context, sessionToken string) (*domain.KratosSession, error) {
	// TODO: Implement actual Kratos integration
	return &domain.KratosSession{
		ID: uuid.New().String(),
		Identity: &domain.KratosIdentity{
			ID: uuid.New().String(),
			Traits: map[string]interface{}{
				"email": "test@example.com",
			},
		},
		// Add other required fields...
	}, nil
}

func (a *KratosClientAdapter) RevokeSession(ctx context.Context, sessionID string) error {
	a.logger.Info("revoking session in Kratos",
		"session_id", sessionID)

	// Revoke session in Kratos
	httpResp, err := a.client.AdminAPI().IdentityAPI.
		DisableSession(ctx, sessionID).
		Execute()

	if err != nil {
		a.logger.Error("kratos session revocation failed",
			"session_id", sessionID,
			"error", err,
			"http_status", getHTTPStatus(httpResp))
		return a.transformKratosError(err, httpResp, "revoke_session")
	}

	a.logger.Info("session revoked successfully",
		"session_id", sessionID)

	return nil
}

func (a *KratosClientAdapter) ListSessions(ctx context.Context, identityID string) ([]*domain.KratosSession, error) {
	// TODO: Implement actual Kratos integration
	return []*domain.KratosSession{}, nil
}

// Identity management methods
func (a *KratosClientAdapter) CreateIdentity(ctx context.Context, traits map[string]interface{}, schemaID string) (*domain.KratosIdentity, error) {
	// TODO: Implement actual Kratos integration
	return &domain.KratosIdentity{
		ID:     uuid.New().String(),
		Traits: traits,
		// Add other required fields...
	}, nil
}

func (a *KratosClientAdapter) GetIdentity(ctx context.Context, identityID string) (*domain.KratosIdentity, error) {
	// TODO: Implement actual Kratos integration
	return &domain.KratosIdentity{
		ID: identityID,
		Traits: map[string]interface{}{
			"email": "test@example.com",
		},
		// Add other required fields...
	}, nil
}

func (a *KratosClientAdapter) UpdateIdentity(ctx context.Context, identityID string, traits map[string]interface{}) (*domain.KratosIdentity, error) {
	// TODO: Implement actual Kratos integration
	return &domain.KratosIdentity{
		ID:     identityID,
		Traits: traits,
		// Add other required fields...
	}, nil
}

func (a *KratosClientAdapter) DeleteIdentity(ctx context.Context, identityID string) error {
	// TODO: Implement actual Kratos integration
	return nil
}

func (a *KratosClientAdapter) ListIdentities(ctx context.Context, page, perPage int) ([]*domain.KratosIdentity, error) {
	// TODO: Implement actual Kratos integration
	return []*domain.KratosIdentity{}, nil
}

// Administrative operations
func (a *KratosClientAdapter) AdminCreateIdentity(ctx context.Context, identity *domain.KratosIdentity) (*domain.KratosIdentity, error) {
	// TODO: Implement actual Kratos integration
	return identity, nil
}

func (a *KratosClientAdapter) AdminUpdateIdentity(ctx context.Context, identityID string, identity *domain.KratosIdentity) (*domain.KratosIdentity, error) {
	// TODO: Implement actual Kratos integration
	return identity, nil
}

func (a *KratosClientAdapter) AdminDeleteIdentity(ctx context.Context, identityID string) error {
	// TODO: Implement actual Kratos integration
	return nil
}

// Health and status
func (a *KratosClientAdapter) Health(ctx context.Context) error {
	return a.client.HealthCheck(ctx)
}

func (a *KratosClientAdapter) Ready(ctx context.Context) error {
	return a.client.HealthCheck(ctx)
}

func (a *KratosClientAdapter) Version(ctx context.Context) (map[string]interface{}, error) {
	// TODO: Implement actual Kratos integration
	return map[string]interface{}{
		"version": "dev",
	}, nil
}

// Utilities
func (a *KratosClientAdapter) ParseSessionFromRequest(r *http.Request) (string, error) {
	// TODO: Implement actual Kratos integration
	return "", nil
}

func (a *KratosClientAdapter) ParseSessionFromCookie(cookies []*http.Cookie) (string, error) {
	// TODO: Implement actual Kratos integration
	return "", nil
}