package service

import (
	"context"
	"errors"
	"net"
	"testing"
)

type timeoutNetError struct{ msg string }

func (e *timeoutNetError) Error() string   { return e.msg }
func (e *timeoutNetError) Timeout() bool   { return true }
func (e *timeoutNetError) Temporary() bool { return false }

var _ net.Error = (*timeoutNetError)(nil)

func TestClassifyProxyError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want ProxyErrorType
	}{
		{name: "nil", err: nil, want: ProxyErrorConnection},
		{name: "deadline exceeded", err: context.DeadlineExceeded, want: ProxyErrorTimeout},
		{name: "wrapped deadline", err: errors.Join(errors.New("do"), context.DeadlineExceeded), want: ProxyErrorTimeout},
		{name: "net timeout", err: &timeoutNetError{msg: "i/o timeout"}, want: ProxyErrorTimeout},
		{name: "connection refused string only", err: errors.New("connection refused"), want: ProxyErrorConnection},
		{name: "timeout substring without typed error", err: errors.New("timeout awaiting response headers"), want: ProxyErrorConnection},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := classifyProxyError(tt.err); got != tt.want {
				t.Fatalf("classifyProxyError() = %v, want %v", got, tt.want)
			}
		})
	}
}
