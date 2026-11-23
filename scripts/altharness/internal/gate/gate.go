// Package gate runs harness steps on a bounded worker pool.
// Results are emitted in catalog order. A step that scans nothing successfully
// is the caller's responsibility to reject; the gate never turns a non-zero
// code or a panic into success.
package gate

import (
	"context"
	"errors"
	"fmt"
	"io"
	"runtime"
	"sort"
	"sync"
	"time"
)

// Step is one check. Run must be safe to call in parallel with other steps
// whose Parallel is true. A nil Run fails closed.
type Step struct {
	Name     string
	Parallel bool
	Timeout  time.Duration
	Run      func(ctx context.Context) (code int, err error)
}

// Options controls the pool. Workers < 1 selects a small default.
type Options struct {
	Workers        int
	DefaultTimeout time.Duration
	Stdout         io.Writer
	Stderr         io.Writer
	// Now is injectable for tests. Nil means time.Now.
	Now func() time.Time
}

// Report is the deterministic summary of one gate run.
type Report struct {
	Results []StepResult
	Failed  int
	Elapsed time.Duration
}

// StepResult is one finished step.
type StepResult struct {
	Name     string
	Code     int
	Err      error
	Elapsed  time.Duration
	TimedOut bool
}

// Run executes steps. Serial steps run one at a time in catalog order and
// never overlap a parallel step. Parallel steps that form a contiguous group
// run together, bounded by Workers. Later groups wait.
func Run(ctx context.Context, steps []Step, opt Options) (Report, error) {
	if ctx == nil {
		return Report{}, errors.New("gate: nil context")
	}
	if opt.Workers < 1 {
		opt.Workers = defaultWorkers()
	}
	if opt.DefaultTimeout <= 0 {
		opt.DefaultTimeout = 15 * time.Minute
	}
	now := opt.Now
	if now == nil {
		now = time.Now
	}
	start := now()
	results := make([]StepResult, len(steps))
	for i := 0; i < len(steps); {
		j := i + 1
		if steps[i].Parallel {
			for j < len(steps) && steps[j].Parallel {
				j++
			}
			runParallel(ctx, steps[i:j], results[i:j], opt)
		} else {
			results[i] = runOne(ctx, steps[i], opt)
		}
		i = j
		if ctx.Err() != nil {
			for k := i; k < len(steps); k++ {
				results[k] = StepResult{Name: steps[k].Name, Code: 1, Err: ctx.Err()}
			}
			break
		}
	}
	rep := Report{Results: results, Elapsed: now().Sub(start)}
	for _, r := range results {
		if r.Code != 0 || r.Err != nil {
			rep.Failed++
		}
	}
	return rep, nil
}

func runParallel(ctx context.Context, steps []Step, out []StepResult, opt Options) {
	sem := make(chan struct{}, opt.Workers)
	var wg sync.WaitGroup
	for i := range steps {
		if ctx.Err() != nil {
			out[i] = StepResult{Name: steps[i].Name, Code: 1, Err: ctx.Err()}
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(i int) {
			defer wg.Done()
			defer func() { <-sem }()
			out[i] = runOne(ctx, steps[i], opt)
		}(i)
	}
	wg.Wait()
}

func runOne(ctx context.Context, step Step, opt Options) (res StepResult) {
	res.Name = step.Name
	timeout := step.Timeout
	if timeout <= 0 {
		timeout = opt.DefaultTimeout
	}
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	now := opt.Now
	if now == nil {
		now = time.Now
	}
	start := now()
	defer func() {
		res.Elapsed = now().Sub(start)
		if rec := recover(); rec != nil {
			res.Code = 1
			res.Err = fmt.Errorf("gate: step %s panicked: %v", step.Name, rec)
		}
	}()
	if step.Run == nil {
		res.Code = 2
		res.Err = fmt.Errorf("gate: step %s has no implementation", step.Name)
		return res
	}
	code, err := step.Run(cctx)
	if errors.Is(cctx.Err(), context.DeadlineExceeded) && ctx.Err() == nil {
		res.TimedOut = true
		res.Code = 1
		res.Err = fmt.Errorf("gate: step %s timed out after %s", step.Name, timeout)
		return res
	}
	if err != nil && code == 0 {
		code = 1
	}
	res.Code = code
	res.Err = err
	return res
}

func defaultWorkers() int {
	n := runtime.GOMAXPROCS(0)
	if n > 6 {
		n = 6
	}
	if n < 2 {
		return 2
	}
	return n
}

// FormatSummary writes PASS/FAIL lines in catalog order.
func FormatSummary(w io.Writer, rep Report) {
	names := make([]int, len(rep.Results))
	for i := range names {
		names[i] = i
	}
	sort.SliceStable(names, func(a, b int) bool { return names[a] < names[b] })
	for _, i := range names {
		r := rep.Results[i]
		label := "PASS"
		if r.Code != 0 || r.Err != nil {
			label = "FAIL"
		}
		fmt.Fprintf(w, "%s: %s (%s)\n", label, r.Name, r.Elapsed.Round(time.Millisecond))
		if r.Err != nil && label == "FAIL" {
			fmt.Fprintf(w, "  error: %s\n", r.Err)
		}
	}
}
