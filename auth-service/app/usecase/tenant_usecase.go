package usecase

import (
	"context"
	"errors"
	"fmt"
	"time"

	"auth-service/app/domain"
	"auth-service/app/port"

	"github.com/google/uuid"
	"log/slog"
)

type TenantUsecase struct {
	tenantGateway port.TenantGateway
	userGateway   port.UserGateway
	logger        *slog.Logger
}

type CreateTenantRequest struct {
	Name             string                       `json:"name" validate:"required,min=2,max=100"`
	Slug             string                       `json:"slug" validate:"required,slug,min=2,max=50"`
	Description      string                       `json:"description,omitempty"`
	SubscriptionTier domain.SubscriptionTier     `json:"subscription_tier"`
	AdminUser        CreateUserRequest           `json:"admin_user"`
}

type CreateUserRequest struct {
	Email    string `json:"email" validate:"required,email"`
	Name     string `json:"name" validate:"required,min=2,max=100"`
	Password string `json:"password" validate:"required,min=8"`
}

type InviteUserRequest struct {
	Email       string           `json:"email" validate:"required,email"`
	Name        string           `json:"name" validate:"required,min=2,max=100"`
	Role        domain.UserRole  `json:"role"`
	Message     string           `json:"message,omitempty"`
}

func NewTenantUsecase(
	tenantGateway port.TenantGateway,
	userGateway port.UserGateway,
	logger *slog.Logger,
) *TenantUsecase {
	return &TenantUsecase{
		tenantGateway: tenantGateway,
		userGateway:   userGateway,
		logger:        logger,
	}
}

func (u *TenantUsecase) CreateTenant(ctx context.Context, req CreateTenantRequest) (*domain.Tenant, error) {
	u.logger.Info("creating new tenant", "slug", req.Slug, "name", req.Name)

	// スラグの重複チェック
	existing, err := u.tenantGateway.GetTenantBySlug(ctx, req.Slug)
	if err != nil && !errors.Is(err, domain.ErrTenantNotFound) {
		return nil, fmt.Errorf("failed to check tenant slug: %w", err)
	}
	if existing != nil {
		return nil, domain.ErrTenantSlugExists
	}

	// テナント作成
	tenant := &domain.Tenant{
		ID:               uuid.New(),
		Name:             req.Name,
		Slug:             req.Slug,
		Description:      req.Description,
		Status:           domain.TenantStatusActive,
		SubscriptionTier: req.SubscriptionTier,
		Settings:         getDefaultSettingsStruct(req.SubscriptionTier),
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}

	if err := u.tenantGateway.CreateTenant(ctx, tenant); err != nil {
		return nil, fmt.Errorf("failed to create tenant: %w", err)
	}

	// 管理者ユーザー作成
	adminUser, err := u.createAdminUser(ctx, tenant.ID, req.AdminUser)
	if err != nil {
		// テナント作成ロールバック
		u.tenantGateway.DeleteTenant(ctx, tenant.ID)
		return nil, fmt.Errorf("failed to create admin user: %w", err)
	}

	u.logger.Info("tenant created successfully",
		"tenant_id", tenant.ID,
		"admin_user_id", adminUser.ID)

	return tenant, nil
}

func (u *TenantUsecase) InviteUser(ctx context.Context, tenantID uuid.UUID, req InviteUserRequest) error {
	// 現在のユーザーがテナント管理者か確認
	currentUser := getCurrentUser(ctx)
	if !currentUser.IsAdmin() || currentUser.TenantID != tenantID {
		return domain.ErrUnauthorized
	}

	// テナントの制限チェック
	tenant, err := u.tenantGateway.GetTenantByID(ctx, tenantID)
	if err != nil {
		return fmt.Errorf("failed to get tenant: %w", err)
	}

	userCount, err := u.userGateway.CountUsersByTenant(ctx, tenantID)
	if err != nil {
		return fmt.Errorf("failed to count users: %w", err)
	}

	if userCount >= tenant.Settings.Limits.MaxUsers {
		return domain.ErrTenantUserLimitExceeded
	}

	// 招待処理
	return u.userGateway.CreateUserInvitation(ctx, tenantID, req)
}

func (u *TenantUsecase) GetTenant(ctx context.Context, tenantID uuid.UUID) (*domain.Tenant, error) {
	tenant, err := u.tenantGateway.GetTenantByID(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to get tenant: %w", err)
	}

	return tenant, nil
}

func (u *TenantUsecase) UpdateTenant(ctx context.Context, tenantID uuid.UUID, updates domain.TenantUpdates) error {
	// 現在のユーザーがテナント管理者か確認
	currentUser := getCurrentUser(ctx)
	if !currentUser.IsAdmin() || currentUser.TenantID != tenantID {
		return domain.ErrUnauthorized
	}

	// Get existing tenant first
	existing, err := u.tenantGateway.GetTenantByID(ctx, tenantID)
	if err != nil {
		return fmt.Errorf("failed to get existing tenant: %w", err)
	}

	// Apply updates
	if updates.Name != nil {
		existing.Name = *updates.Name
	}
	if updates.Description != nil {
		existing.Description = *updates.Description
	}
	if updates.SubscriptionTier != nil {
		existing.SubscriptionTier = *updates.SubscriptionTier
	}
	if updates.Settings != nil {
		existing.Settings = *updates.Settings
	}
	existing.UpdatedAt = time.Now()

	return u.tenantGateway.UpdateTenant(ctx, existing)
}

func (u *TenantUsecase) createAdminUser(ctx context.Context, tenantID uuid.UUID, req CreateUserRequest) (*domain.User, error) {
	adminUser := &domain.User{
		ID:        uuid.New(),
		TenantID:  tenantID,
		Email:     req.Email,
		Name:      req.Name,
		Role:      domain.UserRoleTenantAdmin,
		Status:    domain.UserStatusActive,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	// パスワードハッシュ化
	hashedPassword, err := u.userGateway.HashPassword(req.Password)
	if err != nil {
		return nil, fmt.Errorf("failed to hash password: %w", err)
	}
	adminUser.PasswordHash = hashedPassword

	if err := u.userGateway.Create(ctx, adminUser); err != nil {
		return nil, fmt.Errorf("failed to create admin user: %w", err)
	}

	return adminUser, nil
}

func getMaxUsers(tier domain.SubscriptionTier) int {
	switch tier {
	case domain.SubscriptionTierFree:
		return 5
	case domain.SubscriptionTierBasic:
		return 25
	case domain.SubscriptionTierPremium:
		return 100
	case domain.SubscriptionTierBusiness:
		return 1000
	default:
		return 5
	}
}

func getMaxFeeds(tier domain.SubscriptionTier) int {
	switch tier {
	case domain.SubscriptionTierFree:
		return 50
	case domain.SubscriptionTierBasic:
		return 200
	case domain.SubscriptionTierPremium:
		return 1000
	case domain.SubscriptionTierBusiness:
		return 10000
	default:
		return 50
	}
}

func getDefaultSettingsStruct(tier domain.SubscriptionTier) domain.TenantSettings {
	features := []string{"rss_feeds", "ai_summary", "tags"}
	limits := domain.TenantLimits{
		MaxUsers: getMaxUsers(tier),
		MaxFeeds: getMaxFeeds(tier),
	}

	// ティア別設定
	switch tier {
	case domain.SubscriptionTierPremium:
		features = append(features, "analytics", "advanced_search")
	case domain.SubscriptionTierBusiness:
		features = append(features, "analytics", "advanced_search", "custom_branding", "api_access")
	}

	return domain.TenantSettings{
		Features: features,
		Limits:   limits,
		Timezone: "Asia/Tokyo",
		Language: "ja",
	}
}

func getCurrentUser(ctx context.Context) *domain.UserContext {
	// コンテキストからユーザー情報を取得
	user, ok := ctx.Value("user").(*domain.UserContext)
	if !ok {
		return nil
	}
	return user
}