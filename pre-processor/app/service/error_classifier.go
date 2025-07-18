// ABOUTME: This file classifies errors for retry decisions
// ABOUTME: Distinguishes between temporary and permanent errors for resilient processing
package service

import (
	"context"
	"errors"
	"fmt"
	"net"
	"syscall"
)

// HTTPError represents an HTTP error with status code
type HTTPError struct {
	StatusCode int
	Message    string
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("HTTP %d: %s", e.StatusCode, e.Message)
}

// IsRetryableError determines if an error should trigger a retry
func IsRetryableError(err error) bool {
	if err == nil {
		return false
	}

	// コンテキストエラーは基本的にリトライ不可
	if errors.Is(err, context.Canceled) {
		return false
	}

	// タイムアウトは一時的エラーとしてリトライ
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}

	// システムコールエラー・OpErrorのチェック（優先）
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		if opErr.Err != nil {
			// syscall.Errnoを直接チェック
			if errno, ok := opErr.Err.(syscall.Errno); ok {
				return errno == syscall.ECONNREFUSED ||
					errno == syscall.ECONNRESET ||
					errno == syscall.ETIMEDOUT
			}
		}
		// OpError自体がTemporary/Timeoutを実装している場合
		if opErr.Temporary() || opErr.Timeout() {
			return true
		}
	}

	// ネットワークエラーのチェック
	var netErr net.Error
	if errors.As(err, &netErr) {
		// タイムアウトまたは一時的なネットワークエラー
		return netErr.Timeout() || netErr.Temporary()
	}

	// HTTPレスポンスエラーのチェック
	if httpErr := extractHTTPError(err); httpErr != nil {
		return isRetryableHTTPStatus(httpErr.StatusCode)
	}

	// その他は永続的エラーとみなす
	return false
}

// extractHTTPError extracts HTTPError from error chain
func extractHTTPError(err error) *HTTPError {
	var httpErr *HTTPError
	if errors.As(err, &httpErr) {
		return httpErr
	}

	return nil
}

// isRetryableHTTPStatus determines if HTTP status code is retryable
func isRetryableHTTPStatus(status int) bool {
	switch {
	case status >= 500 && status <= 599:
		// 5xxサーバーエラーはリトライ可能
		return true
	case status == 408: // Request Timeout
		return true
	case status == 429: // Too Many Requests
		return true
	case status == 502: // Bad Gateway
		return true
	case status == 503: // Service Unavailable
		return true
	case status == 504: // Gateway Timeout
		return true
	default:
		// 4xxクライアントエラーは基本的にリトライ不可
		return false
	}
}
