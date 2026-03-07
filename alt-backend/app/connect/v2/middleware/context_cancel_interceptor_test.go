package middleware

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"testing"

	"connectrpc.com/connect"
)

func TestUnary_Success_PassThrough(t *testing.T) {
	interceptor := NewContextCancelInterceptor(slog.Default())
	i := interceptor.Interceptor()

	wrapped := i.WrapUnary(func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		return nil, nil
	})

	resp, err := wrapped(context.Background(), nil)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if resp != nil {
		t.Fatalf("expected nil response, got %v", resp)
	}
}

func TestUnary_Error_NoCancel_PassThrough(t *testing.T) {
	interceptor := NewContextCancelInterceptor(slog.Default())
	i := interceptor.Interceptor()

	origErr := fmt.Errorf("some internal error")
	wrapped := i.WrapUnary(func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		return nil, origErr
	})

	_, err := wrapped(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if err != origErr {
		t.Fatalf("expected original error %v, got %v", origErr, err)
	}
}

func TestUnary_Error_ContextCanceled_ReturnsCanceled(t *testing.T) {
	interceptor := NewContextCancelInterceptor(slog.Default())
	i := interceptor.Interceptor()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	wrapped := i.WrapUnary(func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		return nil, fmt.Errorf("db query failed: %w", context.Canceled)
	})

	_, err := wrapped(ctx, nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	var connectErr *connect.Error
	if !errors.As(err, &connectErr) {
		t.Fatalf("expected *connect.Error, got %T: %v", err, err)
	}
	if connectErr.Code() != connect.CodeCanceled {
		t.Fatalf("expected CodeCanceled, got %v", connectErr.Code())
	}
}

func TestUnary_Error_DeadlineExceeded_ReturnsDeadlineExceeded(t *testing.T) {
	interceptor := NewContextCancelInterceptor(slog.Default())
	i := interceptor.Interceptor()

	ctx, cancel := context.WithTimeout(context.Background(), 0) // immediately expired
	defer cancel()

	wrapped := i.WrapUnary(func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		return nil, fmt.Errorf("request took too long: %w", context.DeadlineExceeded)
	})

	_, err := wrapped(ctx, nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	var connectErr *connect.Error
	if !errors.As(err, &connectErr) {
		t.Fatalf("expected *connect.Error, got %T: %v", err, err)
	}
	if connectErr.Code() != connect.CodeDeadlineExceeded {
		t.Fatalf("expected CodeDeadlineExceeded, got %v", connectErr.Code())
	}
}

func TestStreaming_Success_PassThrough(t *testing.T) {
	interceptor := NewContextCancelInterceptor(slog.Default())
	i := interceptor.Interceptor()

	wrapped := i.WrapStreamingHandler(func(ctx context.Context, conn connect.StreamingHandlerConn) error {
		return nil
	})

	err := wrapped(context.Background(), nil)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestStreaming_Error_NoCancel_PassThrough(t *testing.T) {
	interceptor := NewContextCancelInterceptor(slog.Default())
	i := interceptor.Interceptor()

	origErr := fmt.Errorf("streaming failed")
	wrapped := i.WrapStreamingHandler(func(ctx context.Context, conn connect.StreamingHandlerConn) error {
		return origErr
	})

	err := wrapped(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if err != origErr {
		t.Fatalf("expected original error %v, got %v", origErr, err)
	}
}

func TestStreaming_Error_ContextCanceled_ReturnsNil(t *testing.T) {
	interceptor := NewContextCancelInterceptor(slog.Default())
	i := interceptor.Interceptor()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	wrapped := i.WrapStreamingHandler(func(ctx context.Context, conn connect.StreamingHandlerConn) error {
		return fmt.Errorf("write failed: %w", context.Canceled)
	})

	err := wrapped(ctx, nil)
	if err != nil {
		t.Fatalf("expected nil error for cancelled streaming, got %v", err)
	}
}

func TestStreaming_Error_DeadlineExceeded_ReturnsNil(t *testing.T) {
	interceptor := NewContextCancelInterceptor(slog.Default())
	i := interceptor.Interceptor()

	ctx, cancel := context.WithTimeout(context.Background(), 0) // immediately expired
	defer cancel()

	wrapped := i.WrapStreamingHandler(func(ctx context.Context, conn connect.StreamingHandlerConn) error {
		return fmt.Errorf("timed out: %w", context.DeadlineExceeded)
	})

	err := wrapped(ctx, nil)
	if err != nil {
		t.Fatalf("expected nil error for timed-out streaming, got %v", err)
	}
}
