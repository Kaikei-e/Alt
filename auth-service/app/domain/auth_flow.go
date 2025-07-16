package domain

import (
	"fmt"
	"time"

	"github.com/google/uuid"
)

// AuthFlowType represents the type of authentication flow
type AuthFlowType string

const (
	AuthFlowTypeLogin        AuthFlowType = "login"
	AuthFlowTypeRegistration AuthFlowType = "registration"
	AuthFlowTypeRecovery     AuthFlowType = "recovery"
	AuthFlowTypeVerification AuthFlowType = "verification"
	AuthFlowTypeSettings     AuthFlowType = "settings"
	AuthFlowTypeLogout       AuthFlowType = "logout"
)

// AuthFlowState represents the state of an authentication flow
type AuthFlowState string

const (
	AuthFlowStateActive     AuthFlowState = "active"
	AuthFlowStateCompleted  AuthFlowState = "completed"
	AuthFlowStateFailed     AuthFlowState = "failed"
	AuthFlowStateExpired    AuthFlowState = "expired"
	AuthFlowStateCancelled  AuthFlowState = "cancelled"
)

// AuthFlow represents a generic authentication flow
type AuthFlow struct {
	ID               string                 `json:"id"`
	Type             AuthFlowType           `json:"type"`
	ExpiresAt        time.Time              `json:"expires_at"`
	IssuedAt         time.Time              `json:"issued_at"`
	RequestURL       string                 `json:"request_url"`
	ReturnTo         string                 `json:"return_to,omitempty"`
	Active           string                 `json:"active,omitempty"`
	State            AuthFlowState          `json:"state"`
	UI               *AuthFlowUI            `json:"ui"`
	Methods          map[string]interface{} `json:"methods,omitempty"`
	TenantID         uuid.UUID              `json:"tenant_id,omitempty"`
	CreatedAt        time.Time              `json:"created_at"`
	UpdatedAt        time.Time              `json:"updated_at"`
}

// AuthFlowUI represents the UI configuration for an auth flow
type AuthFlowUI struct {
	Action   string               `json:"action"`
	Method   string               `json:"method"`
	Nodes    []*AuthFlowNode      `json:"nodes"`
	Messages []*AuthFlowMessage   `json:"messages,omitempty"`
}

// AuthFlowNode represents a UI node in an auth flow
type AuthFlowNode struct {
	Type       string                 `json:"type"`
	Group      string                 `json:"group"`
	Attributes map[string]interface{} `json:"attributes"`
	Messages   []*AuthFlowMessage     `json:"messages,omitempty"`
	Meta       map[string]interface{} `json:"meta,omitempty"`
}

// AuthFlowMessage represents a message in an auth flow
type AuthFlowMessage struct {
	ID      int64                  `json:"id"`
	Text    string                 `json:"text"`
	Type    string                 `json:"type"`
	Context map[string]interface{} `json:"context,omitempty"`
}

// LoginFlow represents a login flow
type LoginFlow struct {
	ID               string                 `json:"id"`
	Type             string                 `json:"type"`
	ExpiresAt        time.Time              `json:"expires_at"`
	IssuedAt         time.Time              `json:"issued_at"`
	RequestURL       string                 `json:"request_url"`
	ReturnTo         string                 `json:"return_to,omitempty"`
	Active           string                 `json:"active,omitempty"`
	UI               *AuthFlowUI            `json:"ui"`
	CreatedBy        string                 `json:"created_by,omitempty"`
	Forced           bool                   `json:"forced,omitempty"`
	Refresh          bool                   `json:"refresh,omitempty"`
	RequestedAAL     string                 `json:"requested_aal,omitempty"`
	TenantID         uuid.UUID              `json:"tenant_id,omitempty"`
}

// RegistrationFlow represents a registration flow
type RegistrationFlow struct {
	ID               string                 `json:"id"`
	Type             string                 `json:"type"`
	ExpiresAt        time.Time              `json:"expires_at"`
	IssuedAt         time.Time              `json:"issued_at"`
	RequestURL       string                 `json:"request_url"`
	ReturnTo         string                 `json:"return_to,omitempty"`
	Active           string                 `json:"active,omitempty"`
	UI               *AuthFlowUI            `json:"ui"`
	TenantID         uuid.UUID              `json:"tenant_id,omitempty"`
}

// LogoutFlow represents a logout flow
type LogoutFlow struct {
	ID               string    `json:"id"`
	LogoutURL        string    `json:"logout_url"`
	LogoutToken      string    `json:"logout_token"`
	RequestURL       string    `json:"request_url"`
	TenantID         uuid.UUID `json:"tenant_id,omitempty"`
}

// RecoveryFlow represents a recovery flow
type RecoveryFlow struct {
	ID               string                 `json:"id"`
	Type             string                 `json:"type"`
	ExpiresAt        time.Time              `json:"expires_at"`
	IssuedAt         time.Time              `json:"issued_at"`
	RequestURL       string                 `json:"request_url"`
	ReturnTo         string                 `json:"return_to,omitempty"`
	Active           string                 `json:"active,omitempty"`
	UI               *AuthFlowUI            `json:"ui"`
	State            AuthFlowState          `json:"state"`
	TenantID         uuid.UUID              `json:"tenant_id,omitempty"`
}

// VerificationFlow represents a verification flow
type VerificationFlow struct {
	ID               string                 `json:"id"`
	Type             string                 `json:"type"`
	ExpiresAt        time.Time              `json:"expires_at"`
	IssuedAt         time.Time              `json:"issued_at"`
	RequestURL       string                 `json:"request_url"`
	ReturnTo         string                 `json:"return_to,omitempty"`
	Active           string                 `json:"active,omitempty"`
	UI               *AuthFlowUI            `json:"ui"`
	State            AuthFlowState          `json:"state"`
	TenantID         uuid.UUID              `json:"tenant_id,omitempty"`
}

// SettingsFlow represents a settings flow
type SettingsFlow struct {
	ID               string                 `json:"id"`
	Type             string                 `json:"type"`
	ExpiresAt        time.Time              `json:"expires_at"`
	IssuedAt         time.Time              `json:"issued_at"`
	RequestURL       string                 `json:"request_url"`
	ReturnTo         string                 `json:"return_to,omitempty"`
	Active           string                 `json:"active,omitempty"`
	UI               *AuthFlowUI            `json:"ui"`
	Identity         *KratosIdentity        `json:"identity,omitempty"`
	State            AuthFlowState          `json:"state"`
	TenantID         uuid.UUID              `json:"tenant_id,omitempty"`
}

// KratosIdentity represents a Kratos identity
type KratosIdentity struct {
	ID           string                 `json:"id"`
	SchemaID     string                 `json:"schema_id"`
	SchemaURL    string                 `json:"schema_url"`
	State        string                 `json:"state"`
	StateChangedAt time.Time            `json:"state_changed_at"`
	Traits       map[string]interface{} `json:"traits"`
	VerifiableAddresses []*VerifiableAddress `json:"verifiable_addresses,omitempty"`
	RecoveryAddresses   []*RecoveryAddress   `json:"recovery_addresses,omitempty"`
	MetadataPublic      map[string]interface{} `json:"metadata_public,omitempty"`
	MetadataAdmin       map[string]interface{} `json:"metadata_admin,omitempty"`
	CreatedAt           time.Time              `json:"created_at"`
	UpdatedAt           time.Time              `json:"updated_at"`
}

// VerifiableAddress represents a verifiable address
type VerifiableAddress struct {
	ID        string    `json:"id"`
	Value     string    `json:"value"`
	Verified  bool      `json:"verified"`
	Via       string    `json:"via"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// RecoveryAddress represents a recovery address
type RecoveryAddress struct {
	ID        string    `json:"id"`
	Value     string    `json:"value"`
	Via       string    `json:"via"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// KratosSession represents a Kratos session
type KratosSession struct {
	ID           string          `json:"id"`
	Active       bool            `json:"active"`
	ExpiresAt    time.Time       `json:"expires_at"`
	AuthenticatedAt time.Time    `json:"authenticated_at"`
	AuthenticationMethods []AuthenticationMethod `json:"authentication_methods"`
	AuthenticatorAssuranceLevel string `json:"authenticator_assurance_level"`
	Identity     *KratosIdentity `json:"identity"`
	Devices      []*SessionDevice `json:"devices,omitempty"`
	IssuedAt     time.Time       `json:"issued_at"`
	TokenizedAt  time.Time       `json:"tokenized_at,omitempty"`
}

// AuthenticationMethod represents an authentication method
type AuthenticationMethod struct {
	Method      string                 `json:"method"`
	AAL         string                 `json:"aal"`
	CompletedAt time.Time              `json:"completed_at"`
	Provider    string                 `json:"provider,omitempty"`
	Config      map[string]interface{} `json:"config,omitempty"`
}

// SessionDevice represents a session device
type SessionDevice struct {
	ID           string    `json:"id"`
	IPAddress    string    `json:"ip_address"`
	UserAgent    string    `json:"user_agent"`
	Location     string    `json:"location,omitempty"`
	SeenAt       time.Time `json:"seen_at"`
	CreatedAt    time.Time `json:"created_at"`
}

// LoginRequest represents a login request
type LoginRequest struct {
	Email      string    `json:"email"`
	Password   string    `json:"password"`
	RememberMe bool      `json:"remember_me"`
	TenantID   uuid.UUID `json:"tenant_id"`
	ReturnTo   string    `json:"return_to,omitempty"`
}

// RegistrationRequest represents a registration request
type RegistrationRequest struct {
	Email     string                 `json:"email"`
	Password  string                 `json:"password"`
	Name      string                 `json:"name"`
	TenantID  uuid.UUID              `json:"tenant_id"`
	ReturnTo  string                 `json:"return_to,omitempty"`
	Traits    map[string]interface{} `json:"traits,omitempty"`
}

// LogoutRequest represents a logout request
type LogoutRequest struct {
	SessionToken string    `json:"session_token"`
	TenantID     uuid.UUID `json:"tenant_id"`
	ReturnTo     string    `json:"return_to,omitempty"`
}

// RecoveryRequest represents a recovery request
type RecoveryRequest struct {
	Email    string    `json:"email"`
	TenantID uuid.UUID `json:"tenant_id"`
	ReturnTo string    `json:"return_to,omitempty"`
}

// VerificationRequest represents a verification request
type VerificationRequest struct {
	Email    string    `json:"email"`
	Code     string    `json:"code,omitempty"`
	TenantID uuid.UUID `json:"tenant_id"`
	ReturnTo string    `json:"return_to,omitempty"`
}

// SettingsRequest represents a settings request
type SettingsRequest struct {
	SessionToken string                 `json:"session_token"`
	TenantID     uuid.UUID              `json:"tenant_id"`
	Password     string                 `json:"password,omitempty"`
	Traits       map[string]interface{} `json:"traits,omitempty"`
	ReturnTo     string                 `json:"return_to,omitempty"`
}

// IsExpired returns true if the login flow is expired
func (lf *LoginFlow) IsExpired() bool {
	return time.Now().After(lf.ExpiresAt)
}

// IsValid returns true if the login flow is valid
func (lf *LoginFlow) IsValid() bool {
	return !lf.IsExpired()
}

// IsExpired returns true if the registration flow is expired
func (rf *RegistrationFlow) IsExpired() bool {
	return time.Now().After(rf.ExpiresAt)
}

// IsValid returns true if the registration flow is valid
func (rf *RegistrationFlow) IsValid() bool {
	return !rf.IsExpired()
}

// IsExpired returns true if the recovery flow is expired
func (rf *RecoveryFlow) IsExpired() bool {
	return time.Now().After(rf.ExpiresAt)
}

// IsValid returns true if the recovery flow is valid
func (rf *RecoveryFlow) IsValid() bool {
	return !rf.IsExpired()
}

// IsExpired returns true if the verification flow is expired
func (vf *VerificationFlow) IsExpired() bool {
	return time.Now().After(vf.ExpiresAt)
}

// IsValid returns true if the verification flow is valid
func (vf *VerificationFlow) IsValid() bool {
	return !vf.IsExpired()
}

// IsExpired returns true if the settings flow is expired
func (sf *SettingsFlow) IsExpired() bool {
	return time.Now().After(sf.ExpiresAt)
}

// IsValid returns true if the settings flow is valid
func (sf *SettingsFlow) IsValid() bool {
	return !sf.IsExpired()
}

// IsExpired returns true if the Kratos session is expired
func (ks *KratosSession) IsExpired() bool {
	return time.Now().After(ks.ExpiresAt)
}

// IsValid returns true if the Kratos session is active and not expired
func (ks *KratosSession) IsValid() bool {
	return ks.Active && !ks.IsExpired()
}

// GetEmail returns the email from the identity traits
func (ki *KratosIdentity) GetEmail() string {
	if ki.Traits == nil {
		return ""
	}
	
	if email, ok := ki.Traits["email"].(string); ok {
		return email
	}
	
	return ""
}

// GetName returns the name from the identity traits
func (ki *KratosIdentity) GetName() string {
	if ki.Traits == nil {
		return ""
	}
	
	if name, ok := ki.Traits["name"].(string); ok {
		return name
	}
	
	return ""
}

// ValidateLoginRequest validates a login request
func (lr *LoginRequest) Validate() error {
	if lr.Email == "" {
		return fmt.Errorf("email is required")
	}
	
	if lr.Password == "" {
		return fmt.Errorf("password is required")
	}
	
	if lr.TenantID == (uuid.UUID{}) {
		return fmt.Errorf("tenant ID is required")
	}
	
	return nil
}

// ValidateRegistrationRequest validates a registration request
func (rr *RegistrationRequest) Validate() error {
	if rr.Email == "" {
		return fmt.Errorf("email is required")
	}
	
	if rr.Password == "" {
		return fmt.Errorf("password is required")
	}
	
	if rr.Name == "" {
		return fmt.Errorf("name is required")
	}
	
	if rr.TenantID == (uuid.UUID{}) {
		return fmt.Errorf("tenant ID is required")
	}
	
	return nil
}

// ValidateLogoutRequest validates a logout request
func (lr *LogoutRequest) Validate() error {
	if lr.SessionToken == "" {
		return fmt.Errorf("session token is required")
	}
	
	if lr.TenantID == (uuid.UUID{}) {
		return fmt.Errorf("tenant ID is required")
	}
	
	return nil
}

// ValidateRecoveryRequest validates a recovery request
func (rr *RecoveryRequest) Validate() error {
	if rr.Email == "" {
		return fmt.Errorf("email is required")
	}
	
	if rr.TenantID == (uuid.UUID{}) {
		return fmt.Errorf("tenant ID is required")
	}
	
	return nil
}

// ValidateVerificationRequest validates a verification request
func (vr *VerificationRequest) Validate() error {
	if vr.Email == "" {
		return fmt.Errorf("email is required")
	}
	
	if vr.TenantID == (uuid.UUID{}) {
		return fmt.Errorf("tenant ID is required")
	}
	
	return nil
}

// ValidateSettingsRequest validates a settings request
func (sr *SettingsRequest) Validate() error {
	if sr.SessionToken == "" {
		return fmt.Errorf("session token is required")
	}
	
	if sr.TenantID == (uuid.UUID{}) {
		return fmt.Errorf("tenant ID is required")
	}
	
	return nil
}

// Helper functions for testing

// NewExpirationTime creates a time.Time for expiration testing
func NewExpirationTime(minutesFromNow int) time.Time {
	return time.Now().Add(time.Duration(minutesFromNow) * time.Minute)
}

// NewCurrentTime creates a current time.Time for testing
func NewCurrentTime() time.Time {
	return time.Now()
}