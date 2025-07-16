package handlers

import (
	"log/slog"
	"net/http"
	"strconv"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"auth-service/app/domain"
	"auth-service/app/port"
)

// UserHandler handles user management HTTP requests
type UserHandler struct {
	userUsecase port.UserUsecase
	logger      *slog.Logger
}

// NewUserHandler creates a new user handler
func NewUserHandler(userUsecase port.UserUsecase, logger *slog.Logger) *UserHandler {
	return &UserHandler{
		userUsecase: userUsecase,
		logger:      logger,
	}
}

// GetProfile gets the current user's profile
// @Summary Get user profile
// @Description Get the current authenticated user's profile
// @Tags user
// @Accept json
// @Produce json
// @Success 200 {object} domain.UserProfile
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /v1/user/profile [get]
func (h *UserHandler) GetProfile(c echo.Context) error {
	ctx := c.Request().Context()

	// Extract user ID from context (set by auth middleware)
	userID, err := h.extractUserIDFromContext(c)
	if err != nil {
		return c.JSON(http.StatusUnauthorized, ErrorResponse{
			Error: "user not authenticated",
		})
	}

	profile, err := h.userUsecase.GetUserProfile(ctx, userID)
	if err != nil {
		h.logger.Error("failed to get user profile", "userId", userID, "error", err)
		return c.JSON(http.StatusNotFound, ErrorResponse{
			Error: "user profile not found",
		})
	}

	return c.JSON(http.StatusOK, profile)
}

// UpdateProfile updates the current user's profile
// @Summary Update user profile
// @Description Update the current authenticated user's profile
// @Tags user
// @Accept json
// @Produce json
// @Param body body UpdateProfileRequest true "Profile update request"
// @Success 200 {object} SuccessResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /v1/user/profile [put]
func (h *UserHandler) UpdateProfile(c echo.Context) error {
	ctx := c.Request().Context()

	// Extract user ID from context
	userID, err := h.extractUserIDFromContext(c)
	if err != nil {
		return c.JSON(http.StatusUnauthorized, ErrorResponse{
			Error: "user not authenticated",
		})
	}

	var req UpdateProfileRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: "invalid request body",
		})
	}

	// Convert request to domain profile
	profile := &domain.UserProfile{
		Name:        req.Name,
		Preferences: req.Preferences,
	}

	if err := h.userUsecase.UpdateUserProfile(ctx, userID, profile); err != nil {
		h.logger.Error("failed to update user profile", "userId", userID, "error", err)
		return c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: "failed to update profile",
		})
	}

	return c.JSON(http.StatusOK, SuccessResponse{
		Message: "profile updated successfully",
	})
}

// GetUserByID gets a user by ID (admin only)
// @Summary Get user by ID
// @Description Get a user by their ID (admin access required)
// @Tags user
// @Accept json
// @Produce json
// @Param userId path string true "User ID"
// @Success 200 {object} domain.User
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /v1/user/{userId} [get]
func (h *UserHandler) GetUserByID(c echo.Context) error {
	ctx := c.Request().Context()

	// Check if user is admin (this would be done by middleware)
	if !h.isAdmin(c) {
		return c.JSON(http.StatusForbidden, ErrorResponse{
			Error: "admin access required",
		})
	}

	userIDStr := c.Param("userId")
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: "invalid user ID format",
		})
	}

	user, err := h.userUsecase.GetUserByID(ctx, userID)
	if err != nil {
		h.logger.Error("failed to get user by ID", "userId", userID, "error", err)
		return c.JSON(http.StatusNotFound, ErrorResponse{
			Error: "user not found",
		})
	}

	return c.JSON(http.StatusOK, user)
}

// ListUsers lists users for a tenant (admin only)
// @Summary List users
// @Description List users for the current tenant (admin access required)
// @Tags user
// @Accept json
// @Produce json
// @Param limit query int false "Limit" default(50)
// @Param offset query int false "Offset" default(0)
// @Success 200 {object} UserListResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Router /v1/user [get]
func (h *UserHandler) ListUsers(c echo.Context) error {
	ctx := c.Request().Context()

	// Check if user is admin
	if !h.isAdmin(c) {
		return c.JSON(http.StatusForbidden, ErrorResponse{
			Error: "admin access required",
		})
	}

	// Extract tenant ID from context
	tenantID, err := h.extractTenantIDFromContext(c)
	if err != nil {
		return c.JSON(http.StatusUnauthorized, ErrorResponse{
			Error: "tenant not identified",
		})
	}

	// Parse query parameters
	limit, err := strconv.Atoi(c.QueryParam("limit"))
	if err != nil || limit <= 0 {
		limit = 50
	}

	offset, err := strconv.Atoi(c.QueryParam("offset"))
	if err != nil || offset < 0 {
		offset = 0
	}

	users, err := h.userUsecase.ListUsersByTenant(ctx, tenantID, limit, offset)
	if err != nil {
		h.logger.Error("failed to list users", "tenantId", tenantID, "error", err)
		return c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: "failed to list users",
		})
	}

	response := UserListResponse{
		Users:  users,
		Total:  len(users),
		Limit:  limit,
		Offset: offset,
	}

	return c.JSON(http.StatusOK, response)
}

// CreateUser creates a new user (admin only)
// @Summary Create user
// @Description Create a new user (admin access required)
// @Tags user
// @Accept json
// @Produce json
// @Param body body CreateUserRequest true "User creation request"
// @Success 201 {object} domain.User
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 409 {object} ErrorResponse
// @Router /v1/user [post]
func (h *UserHandler) CreateUser(c echo.Context) error {
	ctx := c.Request().Context()

	// Check if user is admin
	if !h.isAdmin(c) {
		return c.JSON(http.StatusForbidden, ErrorResponse{
			Error: "admin access required",
		})
	}

	var req CreateUserRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: "invalid request body",
		})
	}

	// Extract tenant ID from context if not provided
	if req.TenantID == uuid.Nil {
		tenantID, err := h.extractTenantIDFromContext(c)
		if err != nil {
			return c.JSON(http.StatusBadRequest, ErrorResponse{
				Error: "tenant ID required",
			})
		}
		req.TenantID = tenantID
	}

	// Convert to domain request
	domainReq := &domain.CreateUserRequest{
		KratosIdentityID: req.KratosIdentityID,
		TenantID:         req.TenantID,
		Email:            req.Email,
		Name:             req.Name,
	}

	user, err := h.userUsecase.CreateUser(ctx, domainReq)
	if err != nil {
		h.logger.Error("failed to create user", "email", req.Email, "error", err)
		return c.JSON(http.StatusConflict, ErrorResponse{
			Error: "failed to create user",
		})
	}

	return c.JSON(http.StatusCreated, user)
}

// DeleteUser deletes a user (admin only)
// @Summary Delete user
// @Description Delete a user by ID (admin access required)
// @Tags user
// @Accept json
// @Produce json
// @Param userId path string true "User ID"
// @Success 200 {object} SuccessResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /v1/user/{userId} [delete]
func (h *UserHandler) DeleteUser(c echo.Context) error {
	ctx := c.Request().Context()

	// Check if user is admin
	if !h.isAdmin(c) {
		return c.JSON(http.StatusForbidden, ErrorResponse{
			Error: "admin access required",
		})
	}

	userIDStr := c.Param("userId")
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: "invalid user ID format",
		})
	}

	if err := h.userUsecase.DeleteUser(ctx, userID); err != nil {
		h.logger.Error("failed to delete user", "userId", userID, "error", err)
		return c.JSON(http.StatusNotFound, ErrorResponse{
			Error: "failed to delete user",
		})
	}

	return c.JSON(http.StatusOK, SuccessResponse{
		Message: "user deleted successfully",
	})
}

// Helper methods
func (h *UserHandler) extractUserIDFromContext(c echo.Context) (uuid.UUID, error) {
	userIDStr := c.Get("user_id")
	if userIDStr == nil {
		return uuid.Nil, echo.NewHTTPError(http.StatusUnauthorized, "user ID not found in context")
	}

	userID, err := uuid.Parse(userIDStr.(string))
	if err != nil {
		return uuid.Nil, echo.NewHTTPError(http.StatusBadRequest, "invalid user ID format")
	}

	return userID, nil
}

func (h *UserHandler) extractTenantIDFromContext(c echo.Context) (uuid.UUID, error) {
	tenantIDStr := c.Get("tenant_id")
	if tenantIDStr == nil {
		return uuid.Nil, echo.NewHTTPError(http.StatusUnauthorized, "tenant ID not found in context")
	}

	tenantID, err := uuid.Parse(tenantIDStr.(string))
	if err != nil {
		return uuid.Nil, echo.NewHTTPError(http.StatusBadRequest, "invalid tenant ID format")
	}

	return tenantID, nil
}

func (h *UserHandler) isAdmin(c echo.Context) bool {
	role := c.Get("user_role")
	if role == nil {
		return false
	}

	return role.(string) == string(domain.UserRoleAdmin)
}

// Request/Response types
type UpdateProfileRequest struct {
	Name        string                  `json:"name,omitempty"`
	Preferences domain.UserPreferences `json:"preferences,omitempty"`
}

type CreateUserRequest struct {
	KratosIdentityID uuid.UUID `json:"kratos_identity_id" validate:"required"`
	TenantID         uuid.UUID `json:"tenant_id,omitempty"`
	Email            string    `json:"email" validate:"required,email"`
	Name             string    `json:"name,omitempty"`
}

type UserListResponse struct {
	Users  []*domain.User `json:"users"`
	Total  int            `json:"total"`
	Limit  int            `json:"limit"`
	Offset int            `json:"offset"`
}