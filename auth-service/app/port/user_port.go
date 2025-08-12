package port

//go:generate mockgen -source=user_port.go -destination=../mocks/mock_user_port.go

import (
	"context"

	"auth-service/app/domain"
	"github.com/google/uuid"
)

// UserUsecase defines user management business logic interface
type UserUsecase interface {
	// User management
	CreateUser(ctx context.Context, req *domain.CreateUserRequest) (*domain.User, error)
	GetUserByID(ctx context.Context, userID uuid.UUID) (*domain.User, error)
	GetUserByEmail(ctx context.Context, email string, tenantID uuid.UUID) (*domain.User, error)
	GetUserByKratosID(ctx context.Context, kratosID uuid.UUID) (*domain.User, error)
	UpdateUserProfile(ctx context.Context, userID uuid.UUID, profile *domain.UserProfile) error
	DeleteUser(ctx context.Context, userID uuid.UUID) error

	// User queries
	ListUsersByTenant(ctx context.Context, tenantID uuid.UUID, limit, offset int) ([]*domain.User, error)
	GetUserProfile(ctx context.Context, userID uuid.UUID) (*domain.UserProfile, error)
}

// UserGateway defines user gateway interface
type UserGateway interface {
	// User operations
	Create(ctx context.Context, user *domain.User) error
	CreateUser(ctx context.Context, user *domain.User) error
	GetUserByID(ctx context.Context, userID uuid.UUID) (*domain.User, error)
	GetUserByEmail(ctx context.Context, email string, tenantID uuid.UUID) (*domain.User, error)
	GetUserByKratosID(ctx context.Context, kratosID uuid.UUID) (*domain.User, error)
	UpdateUser(ctx context.Context, user *domain.User) error
	DeleteUser(ctx context.Context, userID uuid.UUID) error
	ListUsersByTenant(ctx context.Context, tenantID uuid.UUID, limit, offset int) ([]*domain.User, error)
	
	// Tenant specific operations
	CountUsersByTenant(ctx context.Context, tenantID uuid.UUID) (int, error)
	CreateUserInvitation(ctx context.Context, tenantID uuid.UUID, req interface{}) error
	HashPassword(password string) (string, error)
}

// UserRepositoryPort defines user data access interface
type UserRepositoryPort interface {
	Create(ctx context.Context, user *domain.User) error
	GetByID(ctx context.Context, userID uuid.UUID) (*domain.User, error)
	GetByEmail(ctx context.Context, email string, tenantID uuid.UUID) (*domain.User, error)
	GetByKratosID(ctx context.Context, kratosID uuid.UUID) (*domain.User, error)
	Update(ctx context.Context, user *domain.User) error
	Delete(ctx context.Context, userID uuid.UUID) error
	ListByTenant(ctx context.Context, tenantID uuid.UUID, limit, offset int) ([]*domain.User, error)
}
