package interceptor

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"connectrpc.com/connect"
)

func newLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func newReqWithHeader(token string) connect.AnyRequest {
	r := connect.NewRequest(&struct{}{})
	if token != "" {
		r.Header().Set("X-Service-Token", token)
	}
	return r
}

func TestServiceAuthInterceptor_RejectsMissingHeader(t *testing.T) {
	t.Parallel()

	i := NewServiceAuthInterceptor(newLogger(), "correct-secret")
	called := false
	handler := connect.UnaryFunc(func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		called = true
		return connect.NewResponse(&struct{}{}), nil
	})

	_, err := i.WrapUnary(handler)(context.Background(), newReqWithHeader(""))

	if called {
		t.Fatal("handler was invoked despite missing token")
	}
	var cerr *connect.Error
	if !errors.As(err, &cerr) || cerr.Code() != connect.CodeUnauthenticated {
		t.Fatalf("expected CodeUnauthenticated, got %v", err)
	}
}

func TestServiceAuthInterceptor_RejectsWrongToken(t *testing.T) {
	t.Parallel()

	i := NewServiceAuthInterceptor(newLogger(), "correct-secret")
	handler := connect.UnaryFunc(func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		t.Fatal("handler must not be invoked")
		return nil, nil
	})

	_, err := i.WrapUnary(handler)(context.Background(), newReqWithHeader("wrong-secret"))

	var cerr *connect.Error
	if !errors.As(err, &cerr) || cerr.Code() != connect.CodeUnauthenticated {
		t.Fatalf("expected CodeUnauthenticated, got %v", err)
	}
}

func TestServiceAuthInterceptor_RejectsDifferingLengthToken(t *testing.T) {
	t.Parallel()

	i := NewServiceAuthInterceptor(newLogger(), "correct-secret")
	handler := connect.UnaryFunc(func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		t.Fatal("handler must not be invoked")
		return nil, nil
	})

	_, err := i.WrapUnary(handler)(context.Background(), newReqWithHeader("correct-secret-extended"))

	var cerr *connect.Error
	if !errors.As(err, &cerr) || cerr.Code() != connect.CodeUnauthenticated {
		t.Fatalf("expected CodeUnauthenticated, got %v", err)
	}
}

func TestServiceAuthInterceptor_AcceptsCorrectToken(t *testing.T) {
	t.Parallel()

	i := NewServiceAuthInterceptor(newLogger(), "correct-secret")
	called := false
	handler := connect.UnaryFunc(func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		called = true
		return connect.NewResponse(&struct{}{}), nil
	})

	_, err := i.WrapUnary(handler)(context.Background(), newReqWithHeader("correct-secret"))

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Fatal("handler was not invoked for valid token")
	}
}

func TestServiceAuthInterceptor_UnconfiguredSecretFailsClosed(t *testing.T) {
	t.Parallel()

	i := NewServiceAuthInterceptor(newLogger(), "")
	handler := connect.UnaryFunc(func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		t.Fatal("handler must not be invoked when SERVICE_SECRET is unset")
		return nil, nil
	})

	_, err := i.WrapUnary(handler)(context.Background(), newReqWithHeader("any-token"))
	if err == nil {
		t.Fatal("unconfigured secret must reject all requests")
	}
	var cerr *connect.Error
	if !errors.As(err, &cerr) || cerr.Code() != connect.CodeInternal {
		t.Fatalf("expected CodeInternal, got %v", err)
	}
}

func TestServiceAuthInterceptor_TrimsWhitespace(t *testing.T) {
	t.Parallel()

	i := NewServiceAuthInterceptor(newLogger(), "correct-secret")
	handler := connect.UnaryFunc(func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		t.Fatal("handler must not be invoked for whitespace-only token")
		return nil, nil
	})

	_, err := i.WrapUnary(handler)(context.Background(), newReqWithHeader("   "))

	var cerr *connect.Error
	if !errors.As(err, &cerr) || cerr.Code() != connect.CodeUnauthenticated {
		t.Fatalf("expected CodeUnauthenticated, got %v", err)
	}
}

func TestServiceAuthInterceptor_ImplementsConnectInterceptor(t *testing.T) {
	t.Parallel()
	var _ connect.Interceptor = NewServiceAuthInterceptor(newLogger(), "x")
}
