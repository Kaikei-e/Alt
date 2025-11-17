package domain

import (
	"context"
	"fmt"
	"github.com/google/uuid"
)

// AuthServiceClient defines the interface for communicating with auth-service
type AuthServiceClient interface {
	GetUser(ctx context.Context, userID uuid.UUID) (*User, error)
	ValidateUserExists(ctx context.Context, userID uuid.UUID) error
}

// User represents a user entity from auth-service
type User struct {
	ID       uuid.UUID `json:"id"`
	Email    string    `json:"email"`
	Name     string    `json:"name"`
	TenantID uuid.UUID `json:"tenant_id"`
	Status   string    `json:"status"`
}

// UserValidator handles user existence validation and cross-database integrity
type UserValidator struct {
	authClient AuthServiceClient
}

// NewUserValidator creates a new UserValidator instance
func NewUserValidator(authClient AuthServiceClient) *UserValidator {
	return &UserValidator{
		authClient: authClient,
	}
}

// ValidateUserExists checks if a user exists in the auth-service
func (v *UserValidator) ValidateUserExists(ctx context.Context, userID uuid.UUID) error {
	user, err := v.authClient.GetUser(ctx, userID)
	if err != nil {
		return fmt.Errorf("failed to validate user: %w", err)
	}

	if user == nil {
		return fmt.Errorf("user not found: %s", userID)
	}

	return nil
}

// ValidateUserInTenant checks if a user belongs to a specific tenant
func (v *UserValidator) ValidateUserInTenant(ctx context.Context, userID, tenantID uuid.UUID) error {
	user, err := v.authClient.GetUser(ctx, userID)
	if err != nil {
		return fmt.Errorf("failed to validate user: %w", err)
	}

	if user == nil {
		return fmt.Errorf("user not found: %s", userID)
	}

	if user.TenantID != tenantID {
		return fmt.Errorf("user %s does not belong to tenant %s", userID, tenantID)
	}

	return nil
}

// ValidateUserActive checks if a user is active
func (v *UserValidator) ValidateUserActive(ctx context.Context, userID uuid.UUID) error {
	user, err := v.authClient.GetUser(ctx, userID)
	if err != nil {
		return fmt.Errorf("failed to validate user: %w", err)
	}

	if user == nil {
		return fmt.Errorf("user not found: %s", userID)
	}

	if user.Status != "active" {
		return fmt.Errorf("user %s is not active (status: %s)", userID, user.Status)
	}

	return nil
}

// IsLegacyUser checks if the provided user ID is the legacy dummy user
func (v *UserValidator) IsLegacyUser(userID uuid.UUID) bool {
	legacyUserID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	return userID == legacyUserID
}

// ValidateUserOrLegacy validates a user exists or is the legacy user
func (v *UserValidator) ValidateUserOrLegacy(ctx context.Context, userID uuid.UUID) error {
	// Allow legacy user for backward compatibility
	if v.IsLegacyUser(userID) {
		return nil
	}

	// Validate regular user
	return v.ValidateUserExists(ctx, userID)
}
