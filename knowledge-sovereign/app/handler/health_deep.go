package handler

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

const (
	deepStatusPass = "pass"
	deepStatusWarn = "warn"
	deepStatusFail = "fail"
)

var (
	errDeepNotReady = errors.New("not_ready")

	deepStatusGauge = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "health_deep_status",
		Help: "Deep health overall status: 0=fail, 1=warn, 2=pass.",
	}, []string{"service"})
	deepLatencyGauge = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "health_deep_latency_seconds",
		Help: "Wall time of the last deep-health run, including cache hits.",
	}, []string{"service"})
)

// DeepPinger is the narrow DB/projector surface /health/deep needs.
type DeepPinger interface {
	PingDB(ctx context.Context) error
	PingProjectors(ctx context.Context, names []string) (ready bool, err error)
}

type deepCheck struct {
	name     string
	critical bool
	probe    func(ctx context.Context) error
}

type deepReport struct {
	Status    string         `json:"status"`
	Service   string         `json:"service"`
	Checks    []deepCheckOut `json:"checks"`
	LatencyMS int64          `json:"latency_ms"`
	Cached    bool           `json:"cached"`
}

type deepCheckOut struct {
	Name      string `json:"name"`
	Status    string `json:"status"`
	Critical  bool   `json:"critical"`
	LatencyMS int64  `json:"latency_ms"`
	Reason    string `json:"reason,omitempty"`
}

// DeepHealthHandler serves GET /health/deep on the ops listener.
type DeepHealthHandler struct {
	service string
	checks  []deepCheck
	ttl     time.Duration
	per     time.Duration
	budget  time.Duration

	mu       sync.Mutex
	cached   *deepReport
	cachedAt time.Time
	inflight chan struct{}
	flight   deepReport
}

// NewDeepHealthHandler wires DB (critical) and projector checkpoint (warn) probes.
func NewDeepHealthHandler(pinger DeepPinger) *DeepHealthHandler {
	names := []string{"knowledge-home-projector", "knowledge-trail-projector"}
	return &DeepHealthHandler{
		service: "knowledge-sovereign",
		ttl:     3 * time.Second,
		per:     250 * time.Millisecond,
		budget:  800 * time.Millisecond,
		checks: []deepCheck{
			{
				name:     "database",
				critical: true,
				probe:    pinger.PingDB,
			},
			{
				name:     "projector",
				critical: false,
				probe: func(ctx context.Context) error {
					ready, err := pinger.PingProjectors(ctx, names)
					if err != nil {
						return err
					}
					if !ready {
						return errDeepNotReady
					}
					return nil
				},
			},
		},
	}
}

func (h *DeepHealthHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	rep := h.run(r.Context())
	w.Header().Set("Content-Type", "application/json")
	if rep.Status == deepStatusFail {
		w.WriteHeader(http.StatusServiceUnavailable)
	} else {
		w.WriteHeader(http.StatusOK)
	}
	_ = json.NewEncoder(w).Encode(rep)
}

func (h *DeepHealthHandler) run(ctx context.Context) deepReport {
	now := time.Now()
	h.mu.Lock()
	if h.cached != nil && now.Sub(h.cachedAt) < h.ttl {
		rep := *h.cached
		h.mu.Unlock()
		rep.Cached = true
		observeDeep(rep)
		return rep
	}
	if h.inflight != nil {
		ch := h.inflight
		h.mu.Unlock()
		<-ch
		h.mu.Lock()
		rep := h.flight
		h.mu.Unlock()
		rep.Cached = true
		observeDeep(rep)
		return rep
	}
	ch := make(chan struct{})
	h.inflight = ch
	h.mu.Unlock()

	rep := h.compute(ctx)
	h.mu.Lock()
	h.cached = &rep
	h.cachedAt = time.Now()
	h.flight = rep
	h.inflight = nil
	close(ch)
	h.mu.Unlock()
	observeDeep(rep)
	return rep
}

func (h *DeepHealthHandler) compute(ctx context.Context) deepReport {
	start := time.Now()
	budget, cancel := context.WithTimeout(context.WithoutCancel(ctx), h.budget)
	defer cancel()
	out := make([]deepCheckOut, len(h.checks))
	var wg sync.WaitGroup
	wg.Add(len(h.checks))
	for i, c := range h.checks {
		i, c := i, c
		go func() {
			defer wg.Done()
			out[i] = h.runOne(budget, c)
		}()
	}
	wg.Wait()
	overall := deepStatusPass
	for _, row := range out {
		if row.Status == deepStatusFail && row.Critical {
			overall = deepStatusFail
			break
		}
		if row.Status != deepStatusPass && overall != deepStatusFail {
			overall = deepStatusWarn
		}
	}
	return deepReport{
		Status:    overall,
		Service:   h.service,
		Checks:    out,
		LatencyMS: time.Since(start).Milliseconds(),
	}
}

func (h *DeepHealthHandler) runOne(ctx context.Context, c deepCheck) deepCheckOut {
	start := time.Now()
	checkCtx, cancel := context.WithTimeout(ctx, h.per)
	defer cancel()
	row := deepCheckOut{Name: c.name, Critical: c.critical, Status: deepStatusPass, LatencyMS: 0}
	err := c.probe(checkCtx)
	row.LatencyMS = time.Since(start).Milliseconds()
	if err == nil {
		return row
	}
	switch {
	case errors.Is(err, context.DeadlineExceeded), checkCtx.Err() != nil:
		row.Reason = "timeout"
	case errors.Is(err, errDeepNotReady):
		row.Reason = "not_ready"
	default:
		row.Reason = "unavailable"
	}
	if c.critical {
		row.Status = deepStatusFail
	} else {
		row.Status = deepStatusWarn
	}
	return row
}

func observeDeep(rep deepReport) {
	v := 0.0
	switch rep.Status {
	case deepStatusPass:
		v = 2
	case deepStatusWarn:
		v = 1
	}
	deepStatusGauge.WithLabelValues(rep.Service).Set(v)
	deepLatencyGauge.WithLabelValues(rep.Service).Set(float64(rep.LatencyMS) / 1000)
}
