package bootstrap

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"slices"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

func testLogger() *slog.Logger {
	return slog.New(slog.DiscardHandler)
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
// for up to one job interval, so every task's stop must complete before the
// first server's shutdown hook is entered.
func TestSupervisorGracefulShutdown_StopsTasksBeforeServers(t *testing.T) {
	var mu sync.Mutex
	var order []string
	var tasksDone atomic.Int32
	record := func(s string) {
		mu.Lock()
		defer mu.Unlock()
		order = append(order, s)
	}

	sup := NewSupervisor(testLogger())
	for _, name := range []string{"rest", "connect", "operator", "ops"} {
		srvName := name
		sup.AddServer(srvName, func() error { return nil }, func(context.Context) error {
			if got := tasksDone.Load(); got != 2 {
				t.Errorf("server %s entered shutdown before all tasks finished (tasksDone=%d, want 2)", srvName, got)
			}
			record("server:" + srvName)
			return nil
		})
	}
	sup.AddTask("scheduler", func() {
		time.Sleep(15 * time.Millisecond)
		tasksDone.Add(1)
		record("task:scheduler")
	})
	sup.AddTask("flusher", func() {
		time.Sleep(10 * time.Millisecond)
		tasksDone.Add(1)
		record("task:flusher")
	})

	sup.GracefulShutdown(t.Context(), 500*time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if len(order) != 6 {
		t.Fatalf("shutdown order = %v, want 6 events", order)
	}
	if order[0] != "task:scheduler" || order[1] != "task:flusher" {
		t.Fatalf("shutdown order = %v, want task:scheduler then task:flusher before any server", order)
	}
	servers := order[2:]
	for _, want := range []string{"server:rest", "server:connect", "server:operator", "server:ops"} {
		if !slices.Contains(servers, want) {
			t.Fatalf("shutdown order = %v, missing %s after tasks", order, want)
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

// A slow listener (e.g. an open SSE stream on restSrv) must not block other
// listeners from draining on the shared timeout, and GracefulShutdown must not
// return until every listener has finished stopping.
func TestSupervisorGracefulShutdown_ShutsServersDownConcurrently(t *testing.T) {
	restEntered := make(chan struct{})
	restRelease := make(chan struct{})
	otherDone := make(chan string, 3)

	sup := NewSupervisor(testLogger())
	sup.AddServer("rest", func() error { return nil }, func(ctx context.Context) error {
		close(restEntered)
		select {
		case <-restRelease:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	})
	for _, name := range []string{"connect", "operator", "ops"} {
		srvName := name
		sup.AddServer(srvName, func() error { return nil }, func(context.Context) error {
			otherDone <- srvName
			return nil
		})
	}

	shutdownDone := make(chan struct{})
	go func() {
		sup.GracefulShutdown(t.Context(), 2*time.Second)
		close(shutdownDone)
	}()

	<-restEntered

	// All three sibling listeners must finish before rest is released.
	for i := range 3 {
		select {
		case <-otherDone:
		case <-time.After(300 * time.Millisecond):
			close(restRelease)
			t.Fatalf("only %d/3 sibling listeners finished while rest was draining; shutdown is still sequential", i)
		}
	}

	// Post-shutdown cleanup must still wait for all listeners.
	select {
	case <-shutdownDone:
		t.Fatal("GracefulShutdown returned while rest was still draining")
	default:
	}

	close(restRelease)

	select {
	case <-shutdownDone:
	case <-time.After(time.Second):
		t.Fatal("GracefulShutdown did not return after rest finished draining")
	}
}

// GracefulShutdown must not return until every server hook has returned,
// including when one hook fails with an error and another blocks until the
// shared shutdown context expires.
func TestSupervisorGracefulShutdown_WaitsForAllServersOnErrorAndTimeout(t *testing.T) {
	var fastReturned atomic.Bool
	var erroredReturned atomic.Bool
	var blockedReturned atomic.Bool
	var blockedCtxErr atomic.Value

	sup := NewSupervisor(testLogger())
	sup.AddServer("fast", func() error { return nil }, func(context.Context) error {
		fastReturned.Store(true)
		return nil
	})
	sup.AddServer("errored", func() error { return nil }, func(context.Context) error {
		erroredReturned.Store(true)
		return errors.New("shutdown failed")
	})
	sup.AddServer("blocked", func() error { return nil }, func(ctx context.Context) error {
		<-ctx.Done()
		blockedCtxErr.Store(ctx.Err())
		time.Sleep(15 * time.Millisecond)
		blockedReturned.Store(true)
		return ctx.Err()
	})

	sup.GracefulShutdown(t.Context(), 30*time.Millisecond)

	if !fastReturned.Load() {
		t.Fatal("fast server shutdown hook had not returned when GracefulShutdown exited")
	}
	if !erroredReturned.Load() {
		t.Fatal("errored server shutdown hook had not returned when GracefulShutdown exited")
	}
	if !blockedReturned.Load() {
		t.Fatal("blocked server shutdown hook had not returned when GracefulShutdown exited")
	}
	gotErr, _ := blockedCtxErr.Load().(error)
	if !errors.Is(gotErr, context.DeadlineExceeded) {
		t.Fatalf("blocked hook ctx.Err() = %v, want %v", gotErr, context.DeadlineExceeded)
	}
}
