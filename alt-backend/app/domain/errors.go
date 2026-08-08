package domain

import (
	"errors"
	"fmt"
	"time"
)

var (
	// テナント関連エラー
	ErrTenantNotFound          = errors.New("tenant not found")
	ErrTenantSlugExists        = errors.New("tenant slug already exists")
	ErrTenantUserLimitExceeded = errors.New("tenant user limit exceeded")
	ErrTenantFeedLimitExceeded = errors.New("tenant feed limit exceeded")
	ErrTenantQuotaExceeded     = errors.New("tenant quota exceeded")

	// 認証・認可エラー
	ErrUnauthorized       = errors.New("unauthorized")
	ErrForbidden          = errors.New("forbidden")
	ErrInvalidCredentials = errors.New("invalid credentials")

	// ユーザー関連エラー
	ErrUserNotFound       = errors.New("user not found")
	ErrUserAlreadyExists  = errors.New("user already exists")
	ErrInvalidUserContext = errors.New("invalid user context")

	// フィード関連エラー
	ErrFeedNotFound      = errors.New("feed not found")
	ErrFeedAlreadyExists = errors.New("feed already exists")
	ErrFeedInvalid       = errors.New("feed is invalid")
	ErrNoSubscriptions   = errors.New("no feeds found")

	// 記事関連エラー
	ErrArticleNotFound      = errors.New("article not found")
	ErrArticleAlreadyExists = errors.New("article already exists")

	// インフラ関連エラー
	ErrRateLimited        = errors.New("rate limited")
	ErrServiceUnavailable = errors.New("service unavailable")
	ErrTimeout            = errors.New("request timeout")
	ErrDatabaseError      = errors.New("database error")

	// 検索関連エラー
	ErrSearchUnavailable = errors.New("search service unavailable")

	// Knowledge Home 関連エラー
	ErrKnowledgeEventNotFound = errors.New("knowledge event not found")
	ErrProjectionStale        = errors.New("projection is stale")
)

type ComplianceError struct {
	Code    int
	Message string
}

func (e *ComplianceError) Error() string {
	return e.Message
}

// RateLimitedError is a transient refusal: a politeness gate (robots.txt
// Crawl-delay, host rate limiter) has not elapsed yet.
//
// Deliberately distinct from ComplianceError, which is permanent. Collapsing
// the two is what let a timing condition be recorded as a user decision.
type RateLimitedError struct {
	Message    string
	RetryAfter time.Duration
}

func (e *RateLimitedError) Error() string {
	return e.Message
}

// UpstreamFetchError means a request to a third-party site never completed:
// its deadline expired, the host did not resolve, the connection failed, or the
// politeness wait for that host ran out before its turn came.
//
// Deliberately distinct from ExternalHTTPError, which is the site answering
// with a status we did not want. Here there is no answer at all — and nothing
// in this system is broken, so it must not reach the client as an internal
// fault or reach the error log as one.
type UpstreamFetchError struct {
	URL   string
	Cause error
}

func (e *UpstreamFetchError) Error() string {
	return fmt.Sprintf("upstream fetch did not complete for %q: %v", e.URL, e.Cause)
}

func (e *UpstreamFetchError) Unwrap() error {
	return e.Cause
}

// ExternalHTTPError represents an unexpected HTTP status from an external site.
type ExternalHTTPError struct {
	StatusCode int
	URL        string
}

func (e *ExternalHTTPError) Error() string {
	return fmt.Sprintf("unexpected status code %d for %q", e.StatusCode, e.URL)
}
