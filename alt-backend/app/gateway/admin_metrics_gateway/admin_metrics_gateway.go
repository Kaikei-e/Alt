package admin_metrics_gateway

import (
	"alt/domain"
	"alt/driver/prometheus_client"
	"alt/port/admin_metrics_port"
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
	"golang.org/x/time/rate"
)

var _ admin_metrics_port.AdminMetricsPort = (*Gateway)(nil)

// Config controls the gateway's resource protection knobs. Zero values fall
// back to Prometheus-safe defaults (cache 10s, 5rps, burst 10).
type Config struct {
	Client         *prometheus_client.Client
	CacheTTL       time.Duration
	RateLimit      float64 // requests per second against Prometheus
	RateLimitBurst int
	Now            func() time.Time
}

type Gateway struct {
	client *prometheus_client.Client
	cache  *cache
	limit  *rate.Limiter
	sf     singleflight.Group
	now    func() time.Time
}

func New(c Config) *Gateway {
	if c.CacheTTL < 0 {
		c.CacheTTL = 0
	}
	if c.RateLimit <= 0 {
		c.RateLimit = 5
	}
	if c.RateLimitBurst <= 0 {
		c.RateLimitBurst = 10
	}
	now := c.Now
	if now == nil {
		now = time.Now
	}
	return &Gateway{
		client: c.Client,
		cache:  newCache(c.CacheTTL, now),
		limit:  rate.NewLimiter(rate.Limit(c.RateLimit), c.RateLimitBurst),
		now:    now,
	}
}

func (g *Gateway) Catalog() []domain.MetricCatalogEntry {
	return catalogEntries()
}

func (g *Gateway) Healthy(ctx context.Context) error {
	return g.client.Health(ctx)
}

// Snapshot resolves each requested MetricKey via the server-side allowlist,
// applies per-upstream-query cache + singleflight, and returns a degraded
// result on timeout/unavailable rather than failing the whole snapshot.
func (g *Gateway) Snapshot(ctx context.Context, keys []domain.MetricKey, window domain.RangeWindow, step domain.Step) (*domain.MetricsSnapshot, error) {
	if len(keys) == 0 {
		return nil, errors.New("admin_metrics_gateway: keys required")
	}
	// Validate allowlist & window/step up front to fail closed before any upstream call.
	entries := make([]allowEntry, 0, len(keys))
	for _, k := range keys {
		e, ok := lookup(k)
		if !ok {
			return nil, fmt.Errorf("admin_metrics_gateway: unknown metric key %q", k)
		}
		entries = append(entries, e)
	}
	if window != "" && window.Duration() == 0 {
		return nil, fmt.Errorf("admin_metrics_gateway: invalid window %q", window)
	}
	if step != "" && step.Duration() == 0 {
		return nil, fmt.Errorf("admin_metrics_gateway: invalid step %q", step)
	}
	// Enforce window/step floor to prevent point-count explosions.
	if window != "" && step != "" {
		if int64(window.Duration()/step.Duration()) > 720 {
			return nil, fmt.Errorf("admin_metrics_gateway: window/step ratio exceeds 720")
		}
	}

	snap := &domain.MetricsSnapshot{Time: g.now(), Metrics: make(map[domain.MetricKey]*domain.MetricResult, len(entries))}
	for _, e := range entries {
		snap.Metrics[e.key] = g.resolve(ctx, e, window, step)
	}
	return snap, nil
}

func (g *Gateway) resolve(ctx context.Context, e allowEntry, window domain.RangeWindow, step domain.Step) *domain.MetricResult {
	kind := e.kind
	if window != "" && step != "" {
		kind = domain.SeriesKindRange
	}
	if kind == domain.SeriesKindRange && (window == "" || step == "") {
		// Fall back to a safe default window/step for range metrics.
		window = domain.RangeWindow15m
		step = domain.Step30s
	}

	cacheKey := fmt.Sprintf("%s|%s|%s|%s", e.key, kind, window, step)
	if v, ok := g.cache.get(cacheKey); ok {
		return v
	}

	v, _, _ := g.sf.Do(cacheKey, func() (any, error) {
		if err := g.limit.Wait(ctx); err != nil {
			return g.degraded(e, kind, "rate_limited: "+err.Error()), nil
		}
		res := g.fetch(ctx, e, kind, window, step)
		g.cache.put(cacheKey, res)
		return res, nil
	})
	mr, _ := v.(*domain.MetricResult)
	if mr == nil {
		mr = g.degraded(e, kind, "singleflight_nil")
	}
	return mr
}

func (g *Gateway) fetch(ctx context.Context, e allowEntry, kind domain.SeriesKind, window domain.RangeWindow, step domain.Step) *domain.MetricResult {
	switch kind {
	case domain.SeriesKindInstant:
		res, err := g.client.QueryInstant(ctx, e.promql, g.now())
		if err != nil {
			return g.degraded(e, kind, err.Error())
		}
		return convertInstant(e, res)
	case domain.SeriesKindRange:
		end := g.now()
		start := end.Add(-window.Duration())
		res, err := g.client.QueryRange(ctx, e.promql, start, end, step.Duration())
		if err != nil {
			return g.degraded(e, kind, err.Error())
		}
		return convertRange(e, res)
	}
	return g.degraded(e, kind, "unsupported_kind")
}

func (g *Gateway) degraded(e allowEntry, kind domain.SeriesKind, reason string) *domain.MetricResult {
	return &domain.MetricResult{
		Key:        e.key,
		Kind:       kind,
		Unit:       e.unit,
		GrafanaURL: e.grafanaURL,
		Degraded:   true,
		Reason:     reason,
	}
}

func convertInstant(e allowEntry, r *prometheus_client.Result) *domain.MetricResult {
	out := &domain.MetricResult{
		Key:        e.key,
		Kind:       domain.SeriesKindInstant,
		Unit:       e.unit,
		GrafanaURL: e.grafanaURL,
		Warnings:   r.Warnings,
		Series:     make([]domain.MetricSeries, 0, len(r.Vector)),
	}
	for _, s := range r.Vector {
		out.Series = append(out.Series, domain.MetricSeries{
			Labels: s.Labels,
			Points: []domain.MetricPoint{{Time: s.Time, Value: s.Value}},
		})
	}
	return out
}

func convertRange(e allowEntry, r *prometheus_client.Result) *domain.MetricResult {
	out := &domain.MetricResult{
		Key:        e.key,
		Kind:       domain.SeriesKindRange,
		Unit:       e.unit,
		GrafanaURL: e.grafanaURL,
		Warnings:   r.Warnings,
		Series:     make([]domain.MetricSeries, 0, len(r.Matrix)),
	}
	for _, s := range r.Matrix {
		pts := make([]domain.MetricPoint, 0, len(s.Points))
		for _, p := range s.Points {
			pts = append(pts, domain.MetricPoint{Time: p.Time, Value: p.Value})
		}
		out.Series = append(out.Series, domain.MetricSeries{Labels: s.Labels, Points: pts})
	}
	return out
}

// small TTL cache keyed by upstream-query identity.
type cache struct {
	mu    sync.RWMutex
	items map[string]cacheItem
	ttl   time.Duration
	now   func() time.Time
}

type cacheItem struct {
	v      *domain.MetricResult
	expiry time.Time
}

func newCache(ttl time.Duration, now func() time.Time) *cache {
	return &cache{items: map[string]cacheItem{}, ttl: ttl, now: now}
}

func (c *cache) get(k string) (*domain.MetricResult, bool) {
	if c.ttl <= 0 {
		return nil, false
	}
	c.mu.RLock()
	it, ok := c.items[k]
	c.mu.RUnlock()
	if !ok || c.now().After(it.expiry) {
		return nil, false
	}
	return it.v, true
}

func (c *cache) put(k string, v *domain.MetricResult) {
	if c.ttl <= 0 {
		return
	}
	c.mu.Lock()
	c.items[k] = cacheItem{v: v, expiry: c.now().Add(c.ttl)}
	c.mu.Unlock()
}
