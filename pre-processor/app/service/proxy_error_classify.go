package service

import (
	"context"
	"errors"
	"net"
)

// classifyProxyError maps transport failures to ProxyErrorType using typed
// error checks instead of substring matching on err.Error().
func classifyProxyError(err error) ProxyErrorType {
	if err == nil {
		return ProxyErrorConnection
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return ProxyErrorTimeout
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return ProxyErrorTimeout
	}
	return ProxyErrorConnection
}
