package bootstrap

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// Shutdown reasons reported by Supervisor.Wait.
const (
	// ReasonSignal means SIGINT/SIGTERM arrived.
	ReasonSignal = "signal"
	// ReasonServerExit means a managed listener returned before we asked it to.
	ReasonServerExit = "server_exit"
	// ReasonContextDone means the caller's context was cancelled from elsewhere.
	ReasonContextDone = "context_done"
)

// Outcome describes why Wait returned.
type Outcome struct {
	Reason string
	Signal string
	Server string
	Err    error
}

type managedServer struct {
	name     string
	start    func() error
	shutdown func(context.Context) error
}

type managedTask struct {
	name string
	stop func()
}

type serverExit struct {
	name string
	err  error
}

// Supervisor owns the "run until signal or first listener death, then shut
// everything down in a fixed order" sequence that every alt-backend binary
// shares. It is the extracted form of main.go's serverExit channel plus the
// ordered Shutdown block, so all three binaries inherit the same crashed-
// listener detection instead of each re-deriving it.
type Supervisor struct {
	log     *slog.Logger
	servers []managedServer
	tasks   []managedTask

	exits chan serverExit
	// signals is a field rather than a local so tests can drive Wait without
	// sending a real signal to the test process.
	signals chan os.Signal
}

// NewSupervisor returns a Supervisor that logs lifecycle transitions to log.
func NewSupervisor(log *slog.Logger) *Supervisor {
	return &Supervisor{log: log}
}

// AddServer registers a listener. start blocks until the listener stops;
// shutdown performs the graceful stop. Registration order is shutdown order.
func (s *Supervisor) AddServer(name string, start func() error, shutdown func(context.Context) error) {
	s.servers = append(s.servers, managedServer{name: name, start: start, shutdown: shutdown})
}

// AddTask registers a non-listener worker (e.g. the job scheduler). stop must
// block until the worker's goroutines have finished. Tasks are stopped before
// any server, matching main.go's cancel() -> scheduler.Shutdown() -> listener
// shutdown order: reversing it makes the scheduler wait out a full job
// interval while the listeners are already gone.
func (s *Supervisor) AddTask(name string, stop func()) {
	s.tasks = append(s.tasks, managedTask{name: name, stop: stop})
}

// Start launches every registered server in its own goroutine. A server that
// returns — cleanly or not — is reported to Wait.
func (s *Supervisor) Start(ctx context.Context) {
	s.exits = make(chan serverExit, len(s.servers)+1)
	for _, srv := range s.servers {
		go func(srv managedServer) {
			s.log.InfoContext(ctx, "server starting", "server", srv.name)
			err := srv.start()
			switch {
			case err == nil:
				s.log.InfoContext(ctx, "server exited", "server", srv.name, "reason", "nil_return")
				s.exits <- serverExit{name: srv.name}
			case isServerClosed(err):
				s.log.InfoContext(ctx, "server exited", "server", srv.name, "reason", "server_closed")
				s.exits <- serverExit{name: srv.name}
			default:
				s.log.ErrorContext(ctx, "server exited with error", "server", srv.name, "error", err)
				s.exits <- serverExit{name: srv.name, err: err}
			}
		}(srv)
	}
}

// Wait blocks until a shutdown signal arrives, a managed server exits, or ctx
// is cancelled. Binaries with zero servers (a pure worker) still return on a
// signal because the signal case never depends on the exit channel.
func (s *Supervisor) Wait(ctx context.Context) Outcome {
	if s.signals == nil {
		s.signals = make(chan os.Signal, 1)
		signal.Notify(s.signals, os.Interrupt, syscall.SIGTERM)
		defer signal.Stop(s.signals)
	}
	if s.exits == nil {
		s.exits = make(chan serverExit, 1)
	}

	select {
	case sig := <-s.signals:
		s.log.WarnContext(ctx, "shutdown triggered by signal", "signal", sig.String())
		return Outcome{Reason: ReasonSignal, Signal: sig.String()}
	case exit := <-s.exits:
		if exit.err != nil {
			s.log.ErrorContext(ctx, "shutdown triggered by server exit", "server", exit.name, "error", exit.err)
		} else {
			s.log.WarnContext(ctx, "shutdown triggered by server exit", "server", exit.name, "error", nil)
		}
		return Outcome{Reason: ReasonServerExit, Server: exit.name, Err: exit.err}
	case <-ctx.Done():
		s.log.WarnContext(ctx, "shutdown triggered by context cancellation", "error", ctx.Err())
		return Outcome{Reason: ReasonContextDone, Err: ctx.Err()}
	}
}

// GracefulShutdown stops tasks first, then every server in registration order,
// bounded by timeout. A hook that fails is logged and the remaining hooks
// still run: one wedged listener must not leak the others.
func (s *Supervisor) GracefulShutdown(ctx context.Context, timeout time.Duration) {
	for _, task := range s.tasks {
		started := time.Now()
		task.stop()
		s.log.InfoContext(ctx, "task stopped", "task", task.name,
			"duration_ms", time.Since(started).Milliseconds())
	}

	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), timeout)
	defer cancel()

	for _, srv := range s.servers {
		if srv.shutdown == nil {
			continue
		}
		s.log.InfoContext(shutdownCtx, "server shutdown starting", "server", srv.name, "timeout", timeout.String())
		if err := srv.shutdown(shutdownCtx); err != nil {
			s.log.ErrorContext(shutdownCtx, "server shutdown failed", "server", srv.name, "error", err)
			continue
		}
		s.log.InfoContext(shutdownCtx, "server shutdown completed", "server", srv.name)
	}
}
