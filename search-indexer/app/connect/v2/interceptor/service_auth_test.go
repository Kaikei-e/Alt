package interceptor

import (
	"context"
	"errors"
	"os"
	"testing"

	"connectrpc.com/connect"

	"search-indexer/logger"
)

func TestMain(m *testing.M) {
	logger.Init()
	os.Exit(m.Run())
}

// fakeRequest is a minimal connect.AnyRequest for header inspection.
type fakeRequest struct {
	connect.Request[struct{}]
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

	interceptor := NewServiceAuthInterceptor("correct-secret")
	called := false
	handler := connect.UnaryFunc(func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		called = true
		return connect.NewResponse(&struct{}{}), nil
	})

	_, err := interceptor.WrapUnary(handler)(context.Background(), newReqWithHeader(""))

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

	interceptor := NewServiceAuthInterceptor("correct-secret")
	handler := connect.UnaryFunc(func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		t.Fatal("handler must not be invoked")
		return nil, nil
	})

	_, err := interceptor.WrapUnary(handler)(context.Background(), newReqWithHeader("wrong-secret"))

	var cerr *connect.Error
	if !errors.As(err, &cerr) || cerr.Code() != connect.CodeUnauthenticated {
		t.Fatalf("expected CodeUnauthenticated, got %v", err)
	}
}

func TestServiceAuthInterceptor_AcceptsCorrectToken(t *testing.T) {
	t.Parallel()

	interceptor := NewServiceAuthInterceptor("correct-secret")
	called := false
	handler := connect.UnaryFunc(func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		called = true
		return connect.NewResponse(&struct{}{}), nil
	})

	_, err := interceptor.WrapUnary(handler)(context.Background(), newReqWithHeader("correct-secret"))

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Fatal("handler was not invoked for valid token")
	}
}

func TestServiceAuthInterceptor_UnconfiguredSecretFailsClosed(t *testing.T) {
	t.Parallel()

	interceptor := NewServiceAuthInterceptor("")
	handler := connect.UnaryFunc(func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		t.Fatal("handler must not be invoked when SERVICE_TOKEN is unset")
		return nil, nil
	})

	_, err := interceptor.WrapUnary(handler)(context.Background(), newReqWithHeader("any"))
	if err == nil {
		t.Fatal("unconfigured secret must reject all requests")
	}
}
