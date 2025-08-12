package domain

import (
	"context"
	"fmt"
	"net/mail"
	"time"

	"github.com/google/uuid"
)

// UserRole represents the role of a user
type UserRole string

const (
	UserRoleAdmin       UserRole = "admin"
	UserRoleUser        UserRole = "user"
	UserRoleReadonly    UserRole = "readonly"
	UserRoleTenantAdmin UserRole = "tenant_admin"
)

// UserStatus represents the status of a user
type UserStatus string

const (
	UserStatusActive    UserStatus = "active"
	UserStatusInactive  UserStatus = "inactive"
	UserStatusSuspended UserStatus = "suspended"
)

// UserPreferences holds user-specific preferences
type UserPreferences struct {
	Theme            string                 `json:"theme"`
	Language         string                 `json:"language"`
	Notifications    NotificationSettings   `json:"notifications"`
	FeedSettings     FeedSettings          `json:"feed_settings"`
	CustomSettings   map[string]interface{} `json:"custom_settings,omitempty"`
}

// NotificationSettings holds notification preferences
type NotificationSettings struct {
	Email bool `json:"email"`
	Push  bool `json:"push"`
}

// FeedSettings holds feed-related preferences
type FeedSettings struct {
	AutoMarkRead   bool   `json:"auto_mark_read"`
	SummaryLength  string `json:"summary_length"`
}

// User represents a user in the system
type User struct {
	ID               uuid.UUID       `json:"id"`
	KratosID         uuid.UUID       `json:"kratos_id"`
	TenantID         uuid.UUID       `json:"tenant_id"`
	Email            string          `json:"email"`
	Name             string          `json:"name"`
	PasswordHash     string          `json:"-"`                         // Exclude from JSON
	Role             UserRole        `json:"role"`
	Status           UserStatus      `json:"status"`
	Preferences      UserPreferences `json:"preferences"`
	CreatedAt        time.Time       `json:"created_at"`
	UpdatedAt        time.Time       `json:"updated_at"`
	LastLoginAt      *time.Time      `json:"last_login_at,omitempty"`
	DeletedAt        *time.Time      `json:"deleted_at,omitempty"`
	EmailVerified    bool            `json:"email_verified"`
	PhoneVerified    bool            `json:"phone_verified"`
	TwoFactorEnabled bool            `json:"two_factor_enabled"`
}

// NewUser creates a new user with validation
func NewUser(email string, tenantID, kratosID uuid.UUID) (*User, error) {
	// Validate email
	if email == "" {
		return nil, fmt.Errorf("email is required")
	}

	if _, err := mail.ParseAddress(email); err != nil {
		return nil, fmt.Errorf("invalid email format: %w", err)
	}

	// Validate tenant ID
	if tenantID == (uuid.UUID{}) {
		return nil, fmt.Errorf("tenant ID is required")
	}

	// Validate Kratos ID
	if kratosID == (uuid.UUID{}) {
		return nil, fmt.Errorf("kratos ID is required")
	}

	now := time.Now()

	user := &User{
		ID:       uuid.New(),
		KratosID: kratosID,
		TenantID: tenantID,
		Email:    email,
		Role:     UserRoleUser,
		Status:   UserStatusActive,
		Preferences: UserPreferences{
			Theme:    "auto",
			Language: "en",
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

	return user, nil
}

// UpdateProfile updates the user's profile information
func (u *User) UpdateProfile(name string, preferences UserPreferences) error {
	u.Name = name
	u.Preferences = preferences
	u.UpdatedAt = time.Now()
	return nil
}

// RecordLogin records the last login time
func (u *User) RecordLogin(loginTime time.Time) {
	u.LastLoginAt = &loginTime
	u.UpdatedAt = time.Now()
}

// ChangeRole changes the user's role with validation
func (u *User) ChangeRole(role UserRole) error {
	validRoles := []UserRole{UserRoleAdmin, UserRoleUser, UserRoleReadonly, UserRoleTenantAdmin}
	
	for _, validRole := range validRoles {
		if role == validRole {
			u.Role = role
			u.UpdatedAt = time.Now()
			return nil
		}
	}

	return fmt.Errorf("invalid role: %s", role)
}

// ChangeStatus changes the user's status
func (u *User) ChangeStatus(status UserStatus) error {
	validStatuses := []UserStatus{UserStatusActive, UserStatusInactive, UserStatusSuspended}
	
	for _, validStatus := range validStatuses {
		if status == validStatus {
			u.Status = status
			u.UpdatedAt = time.Now()
			return nil
		}
	}

	return fmt.Errorf("invalid status: %s", status)
}

// IsActive returns true if the user is active
func (u *User) IsActive() bool {
	return u.Status == UserStatusActive
}

// SoftDelete marks the user as deleted
func (u *User) SoftDelete() {
	now := time.Now()
	u.DeletedAt = &now
	u.Status = UserStatusInactive
	u.UpdatedAt = now
}

// IsDeleted returns true if the user is soft deleted
func (u *User) IsDeleted() bool {
	return u.DeletedAt != nil
}

// IsAdmin returns true if the user has admin role
func (u *User) IsAdmin() bool {
	return u.Role == UserRoleAdmin || u.Role == UserRoleTenantAdmin
}

// GetUserFromContext extracts user context from context
func GetUserFromContext(ctx context.Context) (*UserContext, error) {
	user, ok := ctx.Value("user").(*UserContext)
	if !ok {
		return nil, ErrUnauthorized
	}
	return user, nil
}

// GetTenantFromContext extracts tenant context from context
func GetTenantFromContext(ctx context.Context) (*Tenant, error) {
	tenant, ok := ctx.Value("tenant").(*Tenant)
	if !ok {
		return nil, ErrTenantNotFound
	}
	return tenant, nil
}

// SetTenantContext sets tenant context in context
func SetTenantContext(ctx context.Context, tenant *Tenant) context.Context {
	return context.WithValue(ctx, "tenant", tenant)
}