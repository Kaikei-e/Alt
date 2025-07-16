package domain

import (
	"time"

	"github.com/google/uuid"
)

// FlowUI represents the UI configuration for authentication flows
type FlowUI struct {
	Action   string        `json:"action"`
	Method   string        `json:"method"`
	Nodes    []FlowNode    `json:"nodes"`
	Messages []FlowMessage `json:"messages,omitempty"`
}

// UI represents the UI configuration for authentication flows (alias for domain compatibility)
type UI = FlowUI

// KratosUI represents the UI configuration from Kratos (alias for gateway compatibility)
type KratosUI = FlowUI

// FlowNode represents a form field in the authentication flow
type FlowNode struct {
	Type       string                 `json:"type"`
	Group      string                 `json:"group"`
	Attributes FlowNodeAttributes     `json:"attributes"`
	Messages   []FlowMessage          `json:"messages,omitempty"`
	Meta       map[string]interface{} `json:"meta,omitempty"`
}

// FlowNodeAttributes represents attributes of a flow node
type FlowNodeAttributes struct {
	Name     string      `json:"name"`
	Type     string      `json:"type"`
	Value    interface{} `json:"value,omitempty"`
	Required bool        `json:"required,omitempty"`
	Disabled bool        `json:"disabled,omitempty"`
	NodeType string      `json:"node_type"`
	Pattern  string      `json:"pattern,omitempty"`
	Label    *FlowText   `json:"label,omitempty"`
	OnClick  string      `json:"onclick,omitempty"`
}

// FlowMessage represents a message in the authentication flow
type FlowMessage struct {
	ID      int                    `json:"id"`
	Text    string                 `json:"text"`
	Type    string                 `json:"type"`
	Context map[string]interface{} `json:"context,omitempty"`
}

// FlowText represents localized text
type FlowText struct {
	ID      int                    `json:"id"`
	Text    string                 `json:"text"`
	Type    string                 `json:"type"`
	Context map[string]interface{} `json:"context,omitempty"`
}

// Device represents a device used for authentication
type Device struct {
	ID         string    `json:"id"`
	IPAddress  string    `json:"ip_address"`
	UserAgent  string    `json:"user_agent"`
	Location   string    `json:"location,omitempty"`
	LastSeenAt time.Time `json:"last_seen_at"`
	CreatedAt  time.Time `json:"created_at"`
}

// UserProfile represents user profile information
type UserProfile struct {
	ID          uuid.UUID       `json:"id"`
	Email       string          `json:"email"`
	Name        string          `json:"name"`
	TenantID    uuid.UUID       `json:"tenant_id"`
	Role        UserRole        `json:"role"`
	Status      UserStatus      `json:"status"`
	Preferences UserPreferences `json:"preferences"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
	LastLoginAt *time.Time      `json:"last_login_at,omitempty"`
}

// CreateUserRequest represents user creation request
type CreateUserRequest struct {
	KratosIdentityID uuid.UUID `json:"kratos_identity_id" validate:"required"`
	TenantID         uuid.UUID `json:"tenant_id" validate:"required"`
	Email            string    `json:"email" validate:"required,email"`
	Name             string    `json:"name,omitempty"`
}

// UpdateUserProfileRequest represents user profile update request
type UpdateUserProfileRequest struct {
	Name        string          `json:"name,omitempty"`
	Preferences UserPreferences `json:"preferences,omitempty"`
}

// KratosFlowDTO represents a Kratos flow data transfer object
type KratosFlowDTO struct {
	ID         string    `json:"id"`
	Type       string    `json:"type"`
	ExpiresAt  time.Time `json:"expires_at"`
	IssuedAt   time.Time `json:"issued_at"`
	RequestURL string    `json:"request_url"`
	Active     bool      `json:"active"`
	UI         *KratosUI `json:"ui"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// KratosSessionDTO represents a Kratos session data transfer object
type KratosSessionDTO struct {
	ID                          string                 `json:"id"`
	Active                      bool                   `json:"active"`
	ExpiresAt                   time.Time              `json:"expires_at"`
	AuthenticatedAt             time.Time              `json:"authenticated_at"`
	AuthenticatorAssuranceLevel string                 `json:"authenticator_assurance_level"`
	AuthenticationMethods       []AuthenticationMethod `json:"authentication_methods"`
	IssuedAt                    time.Time              `json:"issued_at"`
	Identity                    KratosIdentity         `json:"identity"`
	Devices                     []Device               `json:"devices,omitempty"`
}

// KratosIdentityDTO represents a Kratos identity data transfer object
type KratosIdentityDTO struct {
	ID                  string                 `json:"id"`
	SchemaID            string                 `json:"schema_id"`
	SchemaURL           string                 `json:"schema_url"`
	State               string                 `json:"state"`
	StateChangedAt      time.Time              `json:"state_changed_at"`
	Traits              map[string]interface{} `json:"traits"`
	VerifiableAddresses []VerifiableAddress    `json:"verifiable_addresses"`
	RecoveryAddresses   []RecoveryAddress      `json:"recovery_addresses"`
	MetadataPublic      map[string]interface{} `json:"metadata_public,omitempty"`
	MetadataAdmin       map[string]interface{} `json:"metadata_admin,omitempty"`
	CreatedAt           time.Time              `json:"created_at"`
	UpdatedAt           time.Time              `json:"updated_at"`
}

// CreateUserRequest.ToUserProfile converts CreateUserRequest to UserProfile
func (r *CreateUserRequest) ToUserProfile() *UserProfile {
	now := time.Now()

	return &UserProfile{
		ID:       uuid.New(),
		Email:    r.Email,
		Name:     r.Name,
		TenantID: r.TenantID,
		Role:     UserRoleUser,
		Status:   UserStatusActive,
		Preferences: UserPreferences{
			Theme:    "light",
			Language: "ja",
			Notifications: NotificationSettings{
				Email: true,
				Push:  false,
			},
			FeedSettings: FeedSettings{
				AutoMarkRead:  true,
				SummaryLength: "medium",
			},
			CustomSettings: make(map[string]interface{}),
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
}
