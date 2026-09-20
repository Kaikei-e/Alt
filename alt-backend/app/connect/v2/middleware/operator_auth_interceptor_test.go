package middleware

import (
	"context"
	"net/http"
	"testing"

	"connectrpc.com/connect"
)

// noopUnaryRequest satisfies connect.AnyRequest with a settable header, which
// is all this interceptor reads.
type noopUnaryRequest struct {
	connect.AnyRequest
	header http.Header
}

func (r *noopUnaryRequest) Header() http.Header { return r.header }
func (r *noopUnaryRequest) Spec() connect.Spec  { return connect.Spec{} }

func newRequest(headers http.Header) connect.AnyRequest {
	return &noopUnaryRequest{header: headers}
}

func TestOperatorAuthInterceptor_Disabled_PassesThroughWithoutHeader(t *testing.T) {
	interceptor := NewOperatorAuthInterceptor("", false)
	wrapped := interceptor.Interceptor().WrapUnary(func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		return nil, nil
	})

	_, err := wrapped(context.Background(), newRequest(http.Header{}))
	if err != nil {
		t.Fatalf("expected pass-through when disabled, got error: %v", err)
	}
}

func TestOperatorAuthInterceptor_Enabled_MissingHeader_Unauthenticated(t *testing.T) {
	interceptor := NewOperatorAuthInterceptor("correct-token", true)
	called := false
	wrapped := interceptor.Interceptor().WrapUnary(func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		called = true
		return nil, nil
	})

	_, err := wrapped(context.Background(), newRequest(http.Header{}))
	if err == nil {
		t.Fatal("expected error for missing Authorization header")
	}
	if connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Errorf("expected CodeUnauthenticated, got %v", connect.CodeOf(err))
	}
	if called {
		t.Error("handler must not run when auth fails")
	}
}

func TestOperatorAuthInterceptor_Enabled_WrongToken_Unauthenticated(t *testing.T) {
	interceptor := NewOperatorAuthInterceptor("correct-token", true)
	wrapped := interceptor.Interceptor().WrapUnary(func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		return nil, nil
	})

	headers := http.Header{}
	headers.Set("Authorization", "Bearer wrong-token")
	_, err := wrapped(context.Background(), newRequest(headers))
	if err == nil {
		t.Fatal("expected error for wrong token")
	}
	if connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Errorf("expected CodeUnauthenticated, got %v", connect.CodeOf(err))
	}
}

func TestOperatorAuthInterceptor_Enabled_CorrectToken_PassesThrough(t *testing.T) {
	interceptor := NewOperatorAuthInterceptor("correct-token", true)
	called := false
	wrapped := interceptor.Interceptor().WrapUnary(func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		called = true
		return nil, nil
	})

	headers := http.Header{}
	headers.Set("Authorization", "Bearer correct-token")
	_, err := wrapped(context.Background(), newRequest(headers))
	if err != nil {
		t.Fatalf("expected success with correct token, got error: %v", err)
	}
	if !called {
		t.Error("handler must run when auth succeeds")
	}
}

func TestOperatorAuthInterceptor_Enabled_MalformedHeader_Unauthenticated(t *testing.T) {
	interceptor := NewOperatorAuthInterceptor("correct-token", true)
	wrapped := interceptor.Interceptor().WrapUnary(func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		return nil, nil
	})

	headers := http.Header{}
	headers.Set("Authorization", "correct-token") // missing "Bearer " prefix
	_, err := wrapped(context.Background(), newRequest(headers))
	if err == nil {
		t.Fatal("expected error for header missing Bearer prefix")
	}
}
