package domain

import "errors"

var (
	// テナント関連エラー
	ErrTenantNotFound           = errors.New("tenant not found")
	ErrTenantSlugExists         = errors.New("tenant slug already exists")
	ErrTenantUserLimitExceeded  = errors.New("tenant user limit exceeded")
	ErrTenantFeedLimitExceeded  = errors.New("tenant feed limit exceeded")
	ErrTenantQuotaExceeded      = errors.New("tenant quota exceeded")
	
	// 認証・認可エラー
	ErrUnauthorized             = errors.New("unauthorized")
	ErrForbidden                = errors.New("forbidden")
	ErrInvalidCredentials       = errors.New("invalid credentials")
	
	// ユーザー関連エラー
	ErrUserNotFound             = errors.New("user not found")
	ErrUserAlreadyExists        = errors.New("user already exists")
	ErrInvalidUserContext       = errors.New("invalid user context")
	
	// フィード関連エラー
	ErrFeedNotFound             = errors.New("feed not found")
	ErrFeedAlreadyExists        = errors.New("feed already exists")
	ErrFeedInvalid              = errors.New("feed is invalid")
	
	// 記事関連エラー
	ErrArticleNotFound          = errors.New("article not found")
	ErrArticleAlreadyExists     = errors.New("article already exists")
)