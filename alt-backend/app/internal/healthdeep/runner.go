package healthdeep

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Status tokens from docs/runbooks/health-deep-contract.md.
const (
	StatusPass = "pass"
	StatusWarn = "warn"
	StatusFail = "fail"
)

// Opaque reasons. Never interpolate probe errors into these.
const (
	ReasonTimeout     = "timeout"
	ReasonUnavailable = "unavailable"
	ReasonNotReady    = "not_ready"
)

// Sentinel errors probes may return. The handler maps them to reasons
// without reading err.Error(), so a DSN or path in the wrapped error
// cannot leak into JSON.
var (
	ErrTimeout     = errors.New(ReasonTimeout)
	ErrUnavailable = errors.New(ReasonUnavailable)
	ErrNotReady    = errors.New(ReasonNotReady)
)

const (
	DefaultCacheTTL     = 3 * time.Second
	DefaultPerCheck     = 250 * time.Millisecond
	DefaultGlobalBudget = 800 * time.Millisecond
	maxCacheTTL         = 5 * time.Second
	minCacheTTL         = 2 * time.Second
	statusFailValue     = 0.0
	statusWarnValue     = 1.0
	statusPassValue     = 2.0
)

var (
	statusGauge = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "health_deep_status",
		Help: "Deep health overall status: 0=fail, 1=warn, 2=pass.",
	}, []string{"service"})
	latencyGauge = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "health_deep_latency_seconds",
		Help: "Wall time of the last deep-health run, including cache hits.",
	}, []string{"service"})
)

// Probe is a single dependency check. It must not log or return secrets.
type Probe func(ctx context.Context) error

// Check is one named probe.
type Check struct {
	Name     string
	Critical bool
	Probe    Probe
}

// Config is the runner's knobs. Zero values become the contract defaults.
type Config struct {
	Service  string
	Checks   []Check
	CacheTTL time.Duration
	PerCheck time.Duration
	Budget   time.Duration
	Now      func() time.Time
}

// Report is the JSON envelope.
type Report struct {
	Status    string        `json:"status"`
	Service   string        `json:"service"`
	Checks    []CheckResult `json:"checks"`
	LatencyMS int64         `json:"latency_ms"`
	Cached    bool          `json:"cached"`
}

// CheckResult is one row of the envelope.
type CheckResult struct {
	Name      string `json:"name"`
	Status    string `json:"status"`
	Critical  bool   `json:"critical"`
	LatencyMS int64  `json:"latency_ms"`
	Reason    string `json:"reason,omitempty"`
}

type cacheEntry struct {
	report Report
	until  time.Time
}

// Runner fans out checks under a global budget, caches, and coalesces.
type Runner struct {
	cfg      Config
	mu       sync.Mutex
	cache    *cacheEntry
	inflight *flight
}

type flight struct {
	done   chan struct{}
	report Report
}

// NewRunner panics if Service is empty or a check has no name/probe —
// miswiring must be loud at boot (CLAUDE.md rule 8).
func NewRunner(cfg Config) *Runner {
	if cfg.Service == "" {
		panic("healthdeep: service name is required")
	}
	for _, c := range cfg.Checks {
		if c.Name == "" || c.Probe == nil {
			panic("healthdeep: every check needs a name and a probe")
		}
	}
	if cfg.CacheTTL == 0 {
		cfg.CacheTTL = DefaultCacheTTL
	}
	if cfg.CacheTTL < minCacheTTL {
		cfg.CacheTTL = minCacheTTL
	}
	if cfg.CacheTTL > maxCacheTTL {
		cfg.CacheTTL = maxCacheTTL
	}
	if cfg.PerCheck == 0 {
		cfg.PerCheck = DefaultPerCheck
	}
	if cfg.Budget == 0 {
		cfg.Budget = DefaultGlobalBudget
	}
	if cfg.Budget >= time.Second {
		cfg.Budget = DefaultGlobalBudget
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &Runner{cfg: cfg}
}

// Handler serves GET /health/deep.
func (r *Runner) Handler() http.Handler {
	return http.HandlerFunc(r.ServeHTTP)
}

// ServeHTTP writes the contract envelope.
func (r *Runner) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	report := r.Run(req.Context())
	w.Header().Set("Content-Type", "application/json")
	if report.Status == StatusFail {
		w.WriteHeader(http.StatusServiceUnavailable)
	} else {
		w.WriteHeader(http.StatusOK)
	}
	_ = json.NewEncoder(w).Encode(report)
}

// Run returns a (possibly cached) report.
func (r *Runner) Run(ctx context.Context) Report {
	start := r.cfg.Now()
	r.mu.Lock()
	now := r.cfg.Now()
	if r.cache != nil && now.Before(r.cache.until) {
		rep := r.cache.report
		r.mu.Unlock()
		rep.Cached = true
		observe(r.cfg.Service, rep.Status, r.cfg.Now().Sub(start))
		return rep
	}
	if r.inflight != nil {
		fl := r.inflight
		r.mu.Unlock()
		select {
		case <-fl.done:
		case <-ctx.Done():
			// The in-flight run still finishes and fills the cache;
			// this caller just cannot wait. Treat as timeout of the
			// whole budget rather than launching a second fan-out.
			rep := Report{
				Status:    StatusFail,
				Service:   r.cfg.Service,
				Checks:    []CheckResult{},
				LatencyMS: r.cfg.Now().Sub(start).Milliseconds(),
			}
			if !hasCritical(r.cfg.Checks) {
				rep.Status = StatusWarn
			}
			observe(r.cfg.Service, rep.Status, r.cfg.Now().Sub(start))
			return rep
		}
		rep := fl.report
		rep.Cached = true
		observe(r.cfg.Service, rep.Status, r.cfg.Now().Sub(start))
		return rep
	}
	fl := &flight{done: make(chan struct{})}
	r.inflight = fl
	r.mu.Unlock()

	rep := r.compute(ctx)
	rep.Cached = false

	r.mu.Lock()
	r.cache = &cacheEntry{report: rep, until: r.cfg.Now().Add(r.cfg.CacheTTL)}
	fl.report = rep
	r.inflight = nil
	close(fl.done)
	r.mu.Unlock()

	observe(r.cfg.Service, rep.Status, r.cfg.Now().Sub(start))
	return rep
}

func hasCritical(checks []Check) bool {
	for _, c := range checks {
		if c.Critical {
			return true
		}
	}
	return false
}

func (r *Runner) compute(ctx context.Context) Report {
	start := r.cfg.Now()
	budget, cancel := context.WithTimeout(context.WithoutCancel(ctx), r.cfg.Budget)
	defer cancel()

	n := len(r.cfg.Checks)
	results := make([]CheckResult, n)
	var wg sync.WaitGroup
	wg.Add(n)
	for i, c := range r.cfg.Checks {
		i, c := i, c
		go func() {
			defer wg.Done()
			results[i] = r.runOne(budget, c)
		}()
	}
	wg.Wait()

	overall := StatusPass
	for _, row := range results {
		if row.Status == StatusFail && row.Critical {
			overall = StatusFail
			break
		}
		if row.Status != StatusPass && overall != StatusFail {
			overall = StatusWarn
		}
	}

	return Report{
		Status:    overall,
		Service:   r.cfg.Service,
		Checks:    results,
		LatencyMS: r.cfg.Now().Sub(start).Milliseconds(),
	}
}

func (r *Runner) runOne(ctx context.Context, c Check) CheckResult {
	start := r.cfg.Now()
	checkCtx, cancel := context.WithTimeout(ctx, r.cfg.PerCheck)
	defer cancel()

	err := c.Probe(checkCtx)
	row := CheckResult{
		Name:      c.Name,
		Critical:  c.Critical,
		LatencyMS: r.cfg.Now().Sub(start).Milliseconds(),
		Status:    StatusPass,
	}
	if err == nil {
		return row
	}
	row.Reason = reasonOf(err, checkCtx)
	if c.Critical {
		row.Status = StatusFail
	} else {
		row.Status = StatusWarn
	}
	return row
}

func reasonOf(err error, ctx context.Context) string {
	switch {
	case errors.Is(err, ErrTimeout), errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled), ctx.Err() != nil:
		return ReasonTimeout
	case errors.Is(err, ErrNotReady):
		return ReasonNotReady
	default:
		return ReasonUnavailable
	}
}

func observe(service, status string, d time.Duration) {
	var v float64
	switch status {
	case StatusPass:
		v = statusPassValue
	case StatusWarn:
		v = statusWarnValue
	default:
		v = statusFailValue
	}
	statusGauge.WithLabelValues(service).Set(v)
	latencyGauge.WithLabelValues(service).Set(d.Seconds())
}
