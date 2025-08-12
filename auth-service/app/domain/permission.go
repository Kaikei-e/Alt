// auth-service/app/domain/permission.go
package domain

import (
	"time"
	"github.com/google/uuid"
)

type Permission string

const (
	// フィード権限
	PermissionFeedRead   Permission = "feed:read"
	PermissionFeedCreate Permission = "feed:create"
	PermissionFeedUpdate Permission = "feed:update"
	PermissionFeedDelete Permission = "feed:delete"

	// 記事権限
	PermissionArticleRead Permission = "article:read"
	PermissionArticleTag  Permission = "article:tag"

	// ユーザー管理権限
	PermissionUserRead   Permission = "user:read"
	PermissionUserInvite Permission = "user:invite"
	PermissionUserManage Permission = "user:manage"

	// テナント管理権限
	PermissionTenantManage Permission = "tenant:manage"
	PermissionTenantConfig Permission = "tenant:config"

	// 管理者権限
	PermissionAdminAll Permission = "admin:all"
)

type Role struct {
	ID          uuid.UUID    `json:"id"`
	TenantID    uuid.UUID    `json:"tenant_id"`
	Name        string       `json:"name"`
	Description string       `json:"description"`
	Permissions []Permission `json:"permissions"`
	IsSystem    bool         `json:"is_system"` // システム定義ロール
	CreatedAt   time.Time    `json:"created_at"`
	UpdatedAt   time.Time    `json:"updated_at"`
}

type UserRoleAssignment struct {
	UserID    uuid.UUID `json:"user_id"`
	RoleID    uuid.UUID `json:"role_id"`
	TenantID  uuid.UUID `json:"tenant_id"`
	GrantedBy uuid.UUID `json:"granted_by"`
	GrantedAt time.Time `json:"granted_at"`
}

// システムロール定義
var SystemRoles = map[string]Role{
	"tenant_admin": {
		Name:        "テナント管理者",
		Description: "テナント内の全権限",
		Permissions: []Permission{
			PermissionFeedRead, PermissionFeedCreate, PermissionFeedUpdate, PermissionFeedDelete,
			PermissionArticleRead, PermissionArticleTag,
			PermissionUserRead, PermissionUserInvite, PermissionUserManage,
			PermissionTenantConfig,
		},
		IsSystem: true,
	},
	"user": {
		Name:        "一般ユーザー",
		Description: "基本的な読み取り・作成権限",
		Permissions: []Permission{
			PermissionFeedRead, PermissionFeedCreate,
			PermissionArticleRead, PermissionArticleTag,
		},
		IsSystem: true,
	},
	"readonly": {
		Name:        "読み取り専用ユーザー",
		Description: "読み取り専用権限",
		Permissions: []Permission{
			PermissionFeedRead,
			PermissionArticleRead,
		},
		IsSystem: true,
	},
}

// 権限チェックヘルパー関数
func (r *Role) HasPermission(permission Permission) bool {
	for _, p := range r.Permissions {
		if p == permission {
			return true
		}
		// 管理者権限は全てを含む
		if p == PermissionAdminAll {
			return true
		}
	}
	return false
}

// UserContext represents the authenticated user context for permission checks
type UserContext struct {
	UserID   uuid.UUID `json:"user_id"`
	TenantID uuid.UUID `json:"tenant_id"`
	Role     UserRole  `json:"role"`
}

// ユーザーが特定の権限を持っているかチェック
func (u *UserContext) HasPermission(permission Permission) bool {
	// システムロールから権限をチェック
	if role, exists := SystemRoles[string(u.Role)]; exists {
		return role.HasPermission(permission)
	}
	return false
}

// テナント管理者かどうかをチェック
func (u *UserContext) IsAdmin() bool {
	return u.Role == UserRoleTenantAdmin || u.HasPermission(PermissionAdminAll)
}

// 監査ログ用の構造体
type AuditLog struct {
	ID        uuid.UUID `json:"id"`
	UserID    uuid.UUID `json:"user_id"`
	TenantID  uuid.UUID `json:"tenant_id"`
	Action    string    `json:"action"`
	Resource  string    `json:"resource"`
	Path      string    `json:"path"`
	Method    string    `json:"method"`
	IPAddress string    `json:"ip_address"`
	UserAgent string    `json:"user_agent"`
	Success   bool      `json:"success"`
	Error     string    `json:"error,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}