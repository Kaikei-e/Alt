package prometheus_client

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

type Config struct {
	URL     string
	Timeout time.Duration
}

type Client struct {
	base       *url.URL
	httpClient *http.Client
	timeout    time.Duration
}

func New(cfg Config) (*Client, error) {
	if cfg.URL == "" {
		return nil, errors.New("prometheus_client: URL is required")
	}
	u, err := url.Parse(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("prometheus_client: parse URL: %w", err)
	}
	to := cfg.Timeout
	if to <= 0 {
		to = 3 * time.Second
	}
	return &Client{
		base:       u,
		httpClient: &http.Client{Timeout: to},
		timeout:    to,
	}, nil
}

type ErrKind int

const (
	ErrKindUnknown ErrKind = iota
	ErrKindTimeout
	ErrKindBadData
	ErrKindExecution
	ErrKindUnavailable
)

type QueryError struct {
	Kind    ErrKind
	Status  int
	Type    string
	Message string
}

func (e *QueryError) Error() string {
	return fmt.Sprintf("prometheus query error (kind=%d status=%d type=%s): %s", e.Kind, e.Status, e.Type, e.Message)
}

type Sample struct {
	Labels map[string]string
	Time   time.Time
	Value  float64
}

type Series struct {
	Labels map[string]string
	Points []SeriesPoint
}

type SeriesPoint struct {
	Time  time.Time
	Value float64
}

type Result struct {
	Vector   []Sample
	Matrix   []Series
	Warnings []string
}

// QueryInstant executes an instant PromQL query at time ts.
func (c *Client) QueryInstant(ctx context.Context, promql string, ts time.Time) (*Result, error) {
	q := url.Values{}
	q.Set("query", promql)
	if !ts.IsZero() {
		q.Set("time", formatPrometheusTime(ts))
	}
	q.Set("timeout", fmt.Sprintf("%dms", c.timeout.Milliseconds()))
	body, err := c.get(ctx, "/api/v1/query", q)
	if err != nil {
		return nil, err
	}
	return decodeInstantResult(body)
}

// QueryRange executes a range PromQL query between start and end with step.
func (c *Client) QueryRange(ctx context.Context, promql string, start, end time.Time, step time.Duration) (*Result, error) {
	q := url.Values{}
	q.Set("query", promql)
	q.Set("start", formatPrometheusTime(start))
	q.Set("end", formatPrometheusTime(end))
	q.Set("step", fmt.Sprintf("%ds", int64(step.Seconds())))
	q.Set("timeout", fmt.Sprintf("%dms", c.timeout.Milliseconds()))
	body, err := c.get(ctx, "/api/v1/query_range", q)
	if err != nil {
		return nil, err
	}
	return decodeRangeResult(body)
}

// Health returns nil if Prometheus /-/ready responds with 200.
func (c *Client) Health(ctx context.Context) error {
	u := *c.base
	u.Path = "/-/ready"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return classifyTransport(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return &QueryError{Kind: ErrKindUnavailable, Status: resp.StatusCode, Message: "prometheus not ready"}
	}
	return nil
}

func (c *Client) get(ctx context.Context, path string, q url.Values) ([]byte, error) {
	u := *c.base
	u.Path = path
	u.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, classifyTransport(err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("prometheus_client: read body: %w", err)
	}
	if resp.StatusCode >= 400 {
		return nil, classifyHTTPError(resp.StatusCode, raw)
	}
	return raw, nil
}

func classifyTransport(err error) error {
	if errors.Is(err, context.DeadlineExceeded) {
		return &QueryError{Kind: ErrKindTimeout, Message: err.Error()}
	}
	// net/http wraps timeout as url.Error with Timeout() bool.
	type timeoutErr interface{ Timeout() bool }
	var te timeoutErr
	if errors.As(err, &te) && te.Timeout() {
		return &QueryError{Kind: ErrKindTimeout, Message: err.Error()}
	}
	return &QueryError{Kind: ErrKindUnavailable, Message: err.Error()}
}
