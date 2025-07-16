package gateway

import (
	"context"
	"fmt"
	"log/slog"

	"auth-service/app/domain"
	"auth-service/app/port"
	"github.com/google/uuid"
)

// UserGateway implements port.UserGateway interface
// It acts as an anti-corruption layer between the domain and user repository
type UserGateway struct {
	userRepo port.UserRepositoryPort
	logger   *slog.Logger
}

// NewUserGateway creates a new UserGateway instance
func NewUserGateway(userRepo port.UserRepositoryPort, logger *slog.Logger) *UserGateway {
	return &UserGateway{
		userRepo: userRepo,
		logger:   logger.With("component", "user_gateway"),
	}
}

// CreateUser creates a new user in the repository
func (g *UserGateway) CreateUser(ctx context.Context, user *domain.User) error {
	g.logger.Info("creating user",
		"user_id", user.ID,
		"email", user.Email,
		"tenant_id", user.TenantID)

	// Validate user data before creating
	if err := g.validateUser(user); err != nil {
		g.logger.Error("user validation failed",
			"user_id", user.ID,
			"error", err)
		return fmt.Errorf("user validation failed: %w", err)
	}

	if err := g.userRepo.Create(ctx, user); err != nil {
		g.logger.Error("failed to create user",
			"user_id", user.ID,
			"error", err)
		return fmt.Errorf("failed to create user: %w", err)
	}

	g.logger.Info("user created successfully",
		"user_id", user.ID,
		"email", user.Email)

	return nil
}

// GetUserByID retrieves a user by ID
func (g *UserGateway) GetUserByID(ctx context.Context, userID uuid.UUID) (*domain.User, error) {
	g.logger.Info("retrieving user by ID", "user_id", userID)

	user, err := g.userRepo.GetByID(ctx, userID)
	if err != nil {
		g.logger.Error("failed to retrieve user by ID",
			"user_id", userID,
			"error", err)
		return nil, fmt.Errorf("failed to retrieve user by ID: %w", err)
	}

	g.logger.Info("user retrieved successfully",
		"user_id", userID,
		"email", user.Email)

	return user, nil
}

// GetUserByEmail retrieves a user by email and tenant ID
func (g *UserGateway) GetUserByEmail(ctx context.Context, email string, tenantID uuid.UUID) (*domain.User, error) {
	g.logger.Info("retrieving user by email",
		"email", email,
		"tenant_id", tenantID)

	user, err := g.userRepo.GetByEmail(ctx, email, tenantID)
	if err != nil {
		g.logger.Error("failed to retrieve user by email",
			"email", email,
			"tenant_id", tenantID,
			"error", err)
		return nil, fmt.Errorf("failed to retrieve user by email: %w", err)
	}

	g.logger.Info("user retrieved by email successfully",
		"user_id", user.ID,
		"email", email)

	return user, nil
}

// GetUserByKratosID retrieves a user by Kratos ID
func (g *UserGateway) GetUserByKratosID(ctx context.Context, kratosID uuid.UUID) (*domain.User, error) {
	g.logger.Info("retrieving user by Kratos ID", "kratos_id", kratosID)

	user, err := g.userRepo.GetByKratosID(ctx, kratosID)
	if err != nil {
		g.logger.Error("failed to retrieve user by Kratos ID",
			"kratos_id", kratosID,
			"error", err)
		return nil, fmt.Errorf("failed to retrieve user by Kratos ID: %w", err)
	}

	g.logger.Info("user retrieved by Kratos ID successfully",
		"user_id", user.ID,
		"kratos_id", kratosID)

	return user, nil
}

// UpdateUser updates an existing user
func (g *UserGateway) UpdateUser(ctx context.Context, user *domain.User) error {
	g.logger.Info("updating user",
		"user_id", user.ID,
		"email", user.Email)

	// Validate user data before updating
	if err := g.validateUser(user); err != nil {
		g.logger.Error("user validation failed",
			"user_id", user.ID,
			"error", err)
		return fmt.Errorf("user validation failed: %w", err)
	}

	if err := g.userRepo.Update(ctx, user); err != nil {
		g.logger.Error("failed to update user",
			"user_id", user.ID,
			"error", err)
		return fmt.Errorf("failed to update user: %w", err)
	}

	g.logger.Info("user updated successfully",
		"user_id", user.ID,
		"email", user.Email)

	return nil
}

// DeleteUser deletes a user by ID
func (g *UserGateway) DeleteUser(ctx context.Context, userID uuid.UUID) error {
	g.logger.Info("deleting user", "user_id", userID)

	if err := g.userRepo.Delete(ctx, userID); err != nil {
		g.logger.Error("failed to delete user",
			"user_id", userID,
			"error", err)
		return fmt.Errorf("failed to delete user: %w", err)
	}

	g.logger.Info("user deleted successfully", "user_id", userID)
	return nil
}

// ListUsersByTenant lists users by tenant with pagination
func (g *UserGateway) ListUsersByTenant(ctx context.Context, tenantID uuid.UUID, limit, offset int) ([]*domain.User, error) {
	g.logger.Info("listing users by tenant",
		"tenant_id", tenantID,
		"limit", limit,
		"offset", offset)

	users, err := g.userRepo.ListByTenant(ctx, tenantID, limit, offset)
	if err != nil {
		g.logger.Error("failed to list users by tenant",
			"tenant_id", tenantID,
			"error", err)
		return nil, fmt.Errorf("failed to list users by tenant: %w", err)
	}

	g.logger.Info("users listed successfully",
		"tenant_id", tenantID,
		"count", len(users))

	return users, nil
}

// validateUser validates user data
func (g *UserGateway) validateUser(user *domain.User) error {
	if user == nil {
		return fmt.Errorf("user cannot be nil")
	}

	if user.ID == uuid.Nil {
		return fmt.Errorf("user ID cannot be empty")
	}

	if user.KratosID == uuid.Nil {
		return fmt.Errorf("Kratos ID cannot be empty")
	}

	if user.TenantID == uuid.Nil {
		return fmt.Errorf("tenant ID cannot be empty")
	}

	if user.Email == "" {
		return fmt.Errorf("email cannot be empty")
	}

	if user.Status == "" {
		return fmt.Errorf("user status cannot be empty")
	}

	return nil
}