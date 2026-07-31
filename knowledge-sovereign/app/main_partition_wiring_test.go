package main

import (
	"context"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"sync"
	"testing"
	"time"
)

type fakePartitionRunner struct {
	mu    sync.Mutex
	runs  int
	err   error
	fired chan struct{}
	// block, when non-nil, holds RunOnce until it is closed — the stand-in
	// for a CREATE TABLE ... PARTITION OF queued behind ACCESS EXCLUSIVE.
	block chan struct{}
}

func (f *fakePartitionRunner) RunOnce(context.Context) error {
	f.mu.Lock()
	f.runs++
	f.mu.Unlock()
	if f.fired != nil {
		select {
		case f.fired <- struct{}{}:
		default:
		}
	}
	if f.block != nil {
		<-f.block
	}
	return f.err
}

func (f *fakePartitionRunner) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.runs
}

// waitFired fails the test if the maintainer does not ensure within a beat.
func waitFired(t *testing.T, runner *fakePartitionRunner, what string) {
	t.Helper()
	select {
	case <-runner.fired:
	case <-time.After(2 * time.Second):
		t.Fatalf("expected the maintainer to %s", what)
	}
}

// The ensure-step must run at startup, not only after the first tick: the tick
// is hours long, and a replica that boots into a month with no partition would
// otherwise keep writing into the default partition until then.
//
// Deliberately a wait on the fake rather than a count check on return: the
// assertion is "the startup ensure happens promptly", not "startup blocks on
// it" (see TestStartPartitionMaintainer_DoesNotGateStartup).
func TestStartPartitionMaintainer_EnsuresOnceAtStartup(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runner := &fakePartitionRunner{fired: make(chan struct{}, 1)}
	var wg sync.WaitGroup
	startPartitionMaintainer(ctx, &wg, runner, time.Hour)

	waitFired(t, runner, "ensure once at startup")

	cancel()
	wg.Wait()
	if got := runner.count(); got != 1 {
		t.Fatalf("expected exactly one startup ensure, got %d", got)
	}
}

// The ensure-step must never gate the rest of main(). Everything after this
// call — both projectors, the branch planner, the projection_health exporter,
// the "started" log and signal.Notify — is sequenced behind it, while the DDL
// it runs takes ACCESS EXCLUSIVE on knowledge_events and (because a DEFAULT
// partition exists) rescans that partition. Queued behind a long reader that
// leaves both HTTP servers answering /health 200 with no projection loop
// running and no graceful-shutdown handler installed: the silent-degradation
// shape of ADR-000928 / rule 8.
func TestStartPartitionMaintainer_DoesNotGateStartup(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	release := make(chan struct{})
	runner := &fakePartitionRunner{fired: make(chan struct{}, 1), block: release}
	var wg sync.WaitGroup

	returned := make(chan struct{})
	go func() {
		startPartitionMaintainer(ctx, &wg, runner, time.Hour)
		close(returned)
	}()

	waitFired(t, runner, "start the startup ensure")
	select {
	case <-returned:
	case <-time.After(2 * time.Second):
		close(release)
		t.Fatal("startPartitionMaintainer blocked on the startup ensure: a DDL waiting for ACCESS EXCLUSIVE would hold up every projector and signal.Notify")
	}

	close(release)
	cancel()
	wg.Wait()
}

// A DDL failure must not take the process down — the default partition still
// accepts writes, and a crashloop would be worse than a loud log.
func TestStartPartitionMaintainer_SurvivesEnsureFailure(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runner := &fakePartitionRunner{
		err:   errors.New("default partition would be violated"),
		fired: make(chan struct{}, 1),
	}
	var wg sync.WaitGroup
	startPartitionMaintainer(ctx, &wg, runner, time.Hour)

	waitFired(t, runner, "ensure once at startup despite the failure")

	cancel()
	wg.Wait()
	if got := runner.count(); got != 1 {
		t.Fatalf("expected exactly one startup ensure, got %d", got)
	}
}

// The maintainer keeps ensuring on its tick, so a replica that stays up across
// a month boundary still creates the next months.
func TestStartPartitionMaintainer_EnsuresOnTick(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runner := &fakePartitionRunner{fired: make(chan struct{}, 1)}
	var wg sync.WaitGroup
	startPartitionMaintainer(ctx, &wg, runner, 5*time.Millisecond)

	<-runner.fired // startup
	select {
	case <-runner.fired: // first tick
	case <-time.After(2 * time.Second):
		t.Fatal("expected the maintainer to ensure again on its tick")
	}

	cancel()
	wg.Wait()
}

// The defect this whole file exists for: GeneratePartitionDDL sat in the driver
// with callers only in its own test file, so no partition was ever created
// after the migrations ran out at 2026-05-01. Assert against main.go's AST that
// the ensure-step has a real production caller — a passing unit test on an
// unwired helper is exactly what hid this for three months.
func TestMain_WiresPartitionMaintainerIntoStartup(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "main.go", nil, 0)
	if err != nil {
		t.Fatalf("parse main.go: %v", err)
	}

	var mainFunc *ast.FuncDecl
	for _, decl := range file.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Name.Name == "main" && fn.Recv == nil {
			mainFunc = fn
			break
		}
	}
	if mainFunc == nil {
		t.Fatal("main.go has no func main")
	}

	var startsMaintainer, constructsMaintainer bool
	ast.Inspect(mainFunc.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		switch fn := call.Fun.(type) {
		case *ast.Ident:
			if fn.Name == "startPartitionMaintainer" {
				startsMaintainer = true
			}
		case *ast.SelectorExpr:
			if pkg, ok := fn.X.(*ast.Ident); ok && pkg.Name == "partition_maintainer" && fn.Sel.Name == "New" {
				constructsMaintainer = true
			}
		}
		return true
	})

	if !constructsMaintainer {
		t.Error("func main does not construct partition_maintainer.New — the ensure-step has no production caller")
	}
	if !startsMaintainer {
		t.Error("func main does not call startPartitionMaintainer — the ensure-step never runs")
	}
}
