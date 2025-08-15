package gateway

import (
	"context"
	"fmt"
	"log/slog"

	"auth-service/app/domain"
	"auth-service/app/port"
	"auth-service/app/driver/kratos"

	"github.com/google/uuid"
)

// AuthGateway implements port.AuthGateway interface
// It acts as an anti-corruption layer between the domain and external auth services
type AuthGateway struct {
	kratosClient       port.KratosClient
	errorTransparifier *kratos.ErrorTransparifier
	logger             *slog.Logger
}

// NewAuthGateway creates a new AuthGateway instance
func NewAuthGateway(kratosClient port.KratosClient, logger *slog.Logger) *AuthGateway {
	return &AuthGateway{
		kratosClient:       kratosClient,
		errorTransparifier: kratos.NewErrorTransparifier(logger),
		logger:             logger.With("component", "auth_gateway"),
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
		
		// 🔄 Phase 4: Kratos エラー完全透過
		detailedError := g.errorTransparifier.TransparifyKratosError(err)
		g.logger.Error("login flow creation failed with detailed error",
			"error_type", detailedError.Type,
			"error_code", detailedError.ErrorCode,
			"is_retryable", detailedError.IsRetryable,
			"technical_info", detailedError.TechnicalInfo)
		
		return nil, fmt.Errorf("failed to create login flow [%s]: %s", detailedError.Type, detailedError.Message)
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
		
		// 🔄 Phase 4: Kratos エラー完全透過
		detailedError := g.errorTransparifier.TransparifyKratosError(err)
		g.logger.Error("registration flow creation failed with detailed error",
			"error_type", detailedError.Type,
			"error_code", detailedError.ErrorCode,
			"is_retryable", detailedError.IsRetryable,
			"technical_info", detailedError.TechnicalInfo)
		
		return nil, fmt.Errorf("failed to create registration flow [%s]: %s", detailedError.Type, detailedError.Message)
	}

	g.logger.Info("registration flow created successfully", "flow_id", kratosFlow.ID)
	return kratosFlow, nil
}

// SubmitLoginFlow submits a login flow to Kratos (X2.md Phase 2.3.1 強化)
func (g *AuthGateway) SubmitLoginFlow(ctx context.Context, flowID string, body interface{}) (*domain.KratosSession, error) {
	g.logger.Info("submitting login flow", 
		"flow_id", flowID,
		"body_type", fmt.Sprintf("%T", body))

	// 型安全な変換とログインボディ検証
	bodyMap, err := g.validateAndTransformLoginBody(body)
	if err != nil {
		g.logger.Error("invalid request body for login", 
			"flow_id", flowID, 
			"error", err,
			"body_type", fmt.Sprintf("%T", body))
		return nil, fmt.Errorf("invalid request body: %w", err)
	}

	// Kratos呼び出し
	kratosSession, err := g.kratosClient.SubmitLoginFlow(ctx, flowID, bodyMap)
	if err != nil {
		// エラーの詳細ログ
		g.logger.Error("kratos login flow submission failed",
			"flow_id", flowID,
			"error", err,
			"error_type", fmt.Sprintf("%T", err))
		
		// 🔄 Phase 4: Kratos エラー完全透過
		detailedError := g.errorTransparifier.TransparifyKratosError(err)
		g.logger.Error("login flow submission failed with detailed error",
			"flow_id", flowID,
			"error_type", detailedError.Type,
			"error_code", detailedError.ErrorCode,
			"is_retryable", detailedError.IsRetryable,
			"technical_info", detailedError.TechnicalInfo,
			"suggestions_count", len(detailedError.Suggestions))
			
		// ドメインエラーに変換（上位層への伝達）
		if domainErr, ok := err.(*domain.AuthError); ok {
			return nil, domainErr
		}
		
		return nil, fmt.Errorf("login flow submission failed [%s]: %s", detailedError.Type, detailedError.Message)
	}

	g.logger.Info("login flow submitted successfully",
		"flow_id", flowID,
		"session_id", kratosSession.ID,
		"user_id", kratosSession.Identity.ID,
		"identity_state", kratosSession.Identity.State)

	return kratosSession, nil
}

// SubmitRegistrationFlow submits a registration flow to Kratos (X2.md Phase 2.3.1 強化)
func (g *AuthGateway) SubmitRegistrationFlow(ctx context.Context, flowID string, body interface{}) (*domain.KratosSession, error) {
	g.logger.Info("submitting registration flow", 
		"flow_id", flowID,
		"body_type", fmt.Sprintf("%T", body))

	// 型安全な変換
	bodyMap, err := g.validateAndTransformBody(body)
	if err != nil {
		g.logger.Error("invalid request body for registration", 
			"flow_id", flowID, 
			"error", err,
			"body", fmt.Sprintf("%+v", body))
		return nil, fmt.Errorf("invalid request body: %w", err)
	}

	// Kratos呼び出し
	kratosSession, err := g.kratosClient.SubmitRegistrationFlow(ctx, flowID, bodyMap)
	if err != nil {
		// エラーの詳細ログ
		g.logger.Error("kratos registration flow submission failed",
			"flow_id", flowID,
			"error", err,
			"error_type", fmt.Sprintf("%T", err))
		
		// 🔄 Phase 4: Kratos エラー完全透過
		detailedError := g.errorTransparifier.TransparifyKratosError(err)
		g.logger.Error("registration flow submission failed with detailed error",
			"flow_id", flowID,
			"error_type", detailedError.Type,
			"error_code", detailedError.ErrorCode,
			"is_retryable", detailedError.IsRetryable,
			"technical_info", detailedError.TechnicalInfo,
			"suggestions_count", len(detailedError.Suggestions))
			
		// ドメインエラーに変換（上位層への伝達）
		if domainErr, ok := err.(*domain.AuthError); ok {
			return nil, domainErr
		}
		
		return nil, fmt.Errorf("registration flow submission failed [%s]: %s", detailedError.Type, detailedError.Message)
	}

	g.logger.Info("registration flow submitted successfully",
		"flow_id", flowID,
		"session_id", kratosSession.ID,
		"user_id", kratosSession.Identity.ID,
		"identity_state", kratosSession.Identity.State)

	return kratosSession, nil
}

// GetSession retrieves a session from Kratos (X2.md Phase 2.3.1 強化)
func (g *AuthGateway) GetSession(ctx context.Context, sessionToken string) (*domain.KratosSession, error) {
	g.logger.Info("retrieving session",
		"session_token_present", sessionToken != "")

	// セッショントークン検証
	if err := g.validateSessionToken(sessionToken); err != nil {
		g.logger.Error("invalid session token", "error", err)
		return nil, fmt.Errorf("invalid session token: %w", err)
	}

	kratosSession, err := g.kratosClient.GetSession(ctx, sessionToken)
	if err != nil {
		g.logger.Error("failed to retrieve session", 
			"error", err,
			"error_type", fmt.Sprintf("%T", err))
			
		// ドメインエラーに変換
		if domainErr, ok := err.(*domain.AuthError); ok {
			return nil, domainErr
		}
		
		return nil, fmt.Errorf("failed to retrieve session: %w", err)
	}

	// セッション検証
	if err := g.validateSessionIntegrity(kratosSession); err != nil {
		g.logger.Error("session integrity validation failed", 
			"session_id", kratosSession.ID,
			"error", err)
		return nil, fmt.Errorf("session validation failed: %w", err)
	}

	g.logger.Info("session retrieved successfully",
		"session_id", kratosSession.ID,
		"user_id", kratosSession.Identity.ID,
		"identity_state", kratosSession.Identity.State,
		"session_active", kratosSession.Active)

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

// X2.md Phase 2.3.1: バリデーション機能とヘルパー関数

// validateAndTransformBody validates and transforms request body for registration
func (g *AuthGateway) validateAndTransformBody(body interface{}) (map[string]interface{}, error) {
	bodyMap, ok := body.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("expected map[string]interface{}, got %T", body)
	}
	
	// 必須フィールドの検証
	if err := g.validateRegistrationBody(bodyMap); err != nil {
		return nil, err
	}
	
	// Kratos形式への変換確認
	if traits, ok := bodyMap["traits"]; ok {
		g.logger.Debug("registration traits present", "traits", traits)
	} else {
		g.logger.Warn("registration traits missing, may cause Kratos validation error")
	}
	
	return bodyMap, nil
}

// validateAndTransformLoginBody validates and transforms request body for login
func (g *AuthGateway) validateAndTransformLoginBody(body interface{}) (map[string]interface{}, error) {
	bodyMap, ok := body.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("expected map[string]interface{}, got %T", body)
	}
	
	// 必須フィールドの検証
	if err := g.validateLoginBody(bodyMap); err != nil {
		return nil, err
	}
	
	// ログイン情報のログ出力（パスワードは除外）
	g.logger.Debug("login body validated", 
		"has_identifier", bodyMap["identifier"] != nil,
		"has_password", bodyMap["password"] != nil,
		"method", bodyMap["method"])
	
	return bodyMap, nil
}

// validateRegistrationBody validates registration request body
func (g *AuthGateway) validateRegistrationBody(body map[string]interface{}) error {
	// traitsの存在確認
	traits, ok := body["traits"]
	if !ok {
		return domain.NewValidationError("traits", nil, "traits field is required")
	}
	
	traitsMap, ok := traits.(map[string]interface{})
	if !ok {
		return domain.NewValidationError("traits", traits, "traits must be an object")
	}
	
	// emailの存在確認
	if email, ok := traitsMap["email"]; !ok || email == "" {
		return domain.NewValidationError("email", email, "email is required in traits")
	}
	
	// passwordの存在確認
	if password, ok := body["password"]; !ok || password == "" {
		return domain.NewValidationError("password", nil, "password is required")
	}
	
	return nil
}

// validateLoginBody validates login request body
func (g *AuthGateway) validateLoginBody(body map[string]interface{}) error {
	// identifierの存在確認
	if identifier, ok := body["identifier"]; !ok || identifier == "" {
		return domain.NewValidationError("identifier", identifier, "identifier is required")
	}
	
	// passwordの存在確認
	if password, ok := body["password"]; !ok || password == "" {
		return domain.NewValidationError("password", nil, "password is required")
	}
	
	// methodの存在確認（デフォルトは"password"）
	if _, ok := body["method"]; !ok {
		body["method"] = "password"
	}
	
	return nil
}

// validateSessionToken validates session token format and presence
func (g *AuthGateway) validateSessionToken(sessionToken string) error {
	if sessionToken == "" {
		return domain.NewValidationError("session_token", sessionToken, "session token is required")
	}
	
	// 基本的なフォーマット検証（実装に応じて調整）
	if len(sessionToken) < 10 {
		return domain.NewValidationError("session_token", sessionToken, "session token format invalid")
	}
	
	return nil
}

// validateSessionIntegrity validates session object integrity
func (g *AuthGateway) validateSessionIntegrity(session *domain.KratosSession) error {
	if session == nil {
		return fmt.Errorf("session is nil")
	}
	
	if session.ID == "" {
		return fmt.Errorf("session missing ID")
	}
	
	if session.Identity == nil {
		return fmt.Errorf("session missing identity")
	}
	
	if session.Identity.ID == "" {
		return fmt.Errorf("session identity missing ID")
	}
	
	// セッション有効性の確認
	if !session.IsValid() {
		return domain.NewAuthError(domain.ErrCodeSessionExpired, "session is expired or inactive", nil)
	}
	
	return nil
}
