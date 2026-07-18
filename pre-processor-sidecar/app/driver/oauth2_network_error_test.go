package driver

import (
	"errors"
	"net"
	"testing"
)

type tempNetError struct{}

func (e *tempNetError) Error() string   { return "temporary" }
func (e *tempNetError) Timeout() bool   { return false }
func (e *tempNetError) Temporary() bool { return true }

var _ net.Error = (*tempNetError)(nil)

func TestIsNetworkError_IgnoresDeprecatedTemporary(t *testing.T) {
	if isNetworkError(&tempNetError{}) {
		t.Fatal("Temporary()-only net.Error must not be treated as network error")
	}
}

type timeoutOnlyNetError struct{}

func (e *timeoutOnlyNetError) Error() string   { return "i/o timeout" }
func (e *timeoutOnlyNetError) Timeout() bool   { return true }
func (e *timeoutOnlyNetError) Temporary() bool { return false }

func TestIsNetworkError_TimeoutStillMatches(t *testing.T) {
	if !isNetworkError(&timeoutOnlyNetError{}) {
		t.Fatal("Timeout() net.Error must still match")
	}
	if !isNetworkError(errors.New("connection refused")) {
		t.Fatal("connection refused string should still match via existing checks")
	}
}
