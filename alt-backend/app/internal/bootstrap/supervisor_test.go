package bootstrap

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"sync"
	"syscall"
	"testing"
	"time"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// The supervisor replaces main.go's serverExit channel. A server that returns
// (cleanly or with an error) must wake Wait, otherwise a crashed listener
// leaves the process running with a dead surface — the failure mode the
// original select{signal | serverExitCh} existed to prevent.
func TestSupervisorWait_ReturnsWhenAServerExits(t *testing.T) {
	tests := []struct {
		name     string
		startErr error
		wantName string
	}{
		{name: "server exits with error", startErr: errors.New("listen: address in use"), wantName: "rest"},
		{name: "server exits cleanly", startErr: nil, wantName: "rest"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sup := NewSupervisor(testLogger())
			sup.AddServer("rest", func() error { return tt.startErr }, func(context.Context) error { return nil })

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			sup.Start(ctx)

			done := make(chan Outcome, 1)
			go func() { done <- sup.Wait(ctx) }()

			select {
			case got := <-done:
				if got.Reason != ReasonServerExit {
					t.Fatalf("Reason = %q, want %q", got.Reason, ReasonServerExit)
				}
				if got.Server != tt.wantName {
					t.Errorf("Server = %q, want %q", got.Server, tt.wantName)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("Wait did not return after the server exited")
			}
		})
	}
}

// harvester registers zero servers. Wait must still return on a signal rather
// than blocking forever on an empty exit channel (plan R13).
func TestSupervisorWait_ReturnsOnSignalWithNoServers(t *testing.T) {
	sup := NewSupervisor(testLogger())
	sup.signals = make(chan os.Signal, 1)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sup.Start(ctx)

	done := make(chan Outcome, 1)
	go func() { done <- sup.Wait(ctx) }()

	sup.signals <- syscall.SIGTERM

	select {
	case got := <-done:
		if got.Reason != ReasonSignal {
			t.Fatalf("Reason = %q, want %q", got.Reason, ReasonSignal)
		}
		if got.Signal != syscall.SIGTERM.String() {
			t.Errorf("Signal = %q, want %q", got.Signal, syscall.SIGTERM.String())
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Wait blocked forever with no servers registered")
	}
}

// main.go cancels the root context, waits for the job scheduler, and only then
// shuts the listeners down. Reversing that order made scheduler.Shutdown block
// for up to one job interval, so the order is pinned here.
func TestSupervisorGracefulShutdown_StopsTasksBeforeServers(t *testing.T) {
	var mu sync.Mutex
	var order []string
	record := func(s string) {
		mu.Lock()
		defer mu.Unlock()
		order = append(order, s)
	}

	sup := NewSupervisor(testLogger())
	sup.AddServer("rest", func() error { return nil }, func(context.Context) error {
		record("server:rest")
		return nil
	})
	sup.AddServer("connect", func() error { return nil }, func(context.Context) error {
		record("server:connect")
		return nil
	})
	sup.AddTask("scheduler", func() { record("task:scheduler") })

	sup.GracefulShutdown(context.Background(), 100*time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	want := []string{"task:scheduler", "server:rest", "server:connect"}
	if len(order) != len(want) {
		t.Fatalf("shutdown order = %v, want %v", order, want)
	}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("shutdown order = %v, want %v", order, want)
		}
	}
}

// A shutdown hook that fails must not stop the remaining hooks from running:
// one wedged listener would otherwise leak the others.
func TestSupervisorGracefulShutdown_ContinuesAfterShutdownError(t *testing.T) {
	stopped := 0
	sup := NewSupervisor(testLogger())
	sup.AddServer("broken", func() error { return nil }, func(context.Context) error {
		return errors.New("shutdown failed")
	})
	sup.AddServer("healthy", func() error { return nil }, func(context.Context) error {
		stopped++
		return nil
	})

	sup.GracefulShutdown(context.Background(), 100*time.Millisecond)

	if stopped != 1 {
		t.Fatalf("healthy server shutdown ran %d times, want 1", stopped)
	}
}
