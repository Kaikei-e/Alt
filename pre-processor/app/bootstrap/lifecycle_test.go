package bootstrap

import (
	"context"
	"errors"
	"testing"

	"pre-processor/utils/otel"
)

// TestResolveOtelShutdown_NilFallsBackToNoOp reproduces the shutdown panic:
// otel.InitProvider returns (nil, err) when initialization fails, and the
// deferred shutdown in Run previously called that nil func unconditionally.
func TestResolveOtelShutdown_NilFallsBackToNoOp(t *testing.T) {
	got := resolveOtelShutdown(nil)
	if got == nil {
		t.Fatal("resolveOtelShutdown(nil) returned nil, want a callable no-op")
	}

	if err := got(context.Background()); err != nil {
		t.Fatalf("no-op shutdown returned error: %v", err)
	}
}

func TestResolveOtelShutdown_PassesThroughNonNil(t *testing.T) {
	wantErr := errors.New("boom")
	called := false
	fn := otel.ShutdownFunc(func(context.Context) error {
		called = true
		return wantErr
	})

	got := resolveOtelShutdown(fn)
	if err := got(context.Background()); !errors.Is(err, wantErr) {
		t.Fatalf("got error %v, want %v", err, wantErr)
	}
	if !called {
		t.Fatal("resolveOtelShutdown replaced a non-nil ShutdownFunc instead of passing it through")
	}
}
