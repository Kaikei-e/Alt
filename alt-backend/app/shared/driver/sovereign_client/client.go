// Package sovereign_client provides a Connect-RPC client for the
// Knowledge Sovereign service. It implements the port interfaces
// (ProjectionMutator, RecallMutator, CurationMutator) so that
// alt-backend can route all knowledge write operations to the
// independent sovereign service.
package sovereign_client

import (
	"alt/orchestrator/port/knowledge_sovereign_port"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"connectrpc.com/connect"

	sovereignv1 "alt/gen/proto/services/sovereign/v1"
	"alt/gen/proto/services/sovereign/v1/sovereignv1connect"
)

// ErrSovereignDisabled is returned by every mutator (ApplyProjectionMutation,
// ApplyRecallMutation, ApplyCurationMutation, and the write_ports.go
// pass-throughs) when the client is disabled (SOVEREIGN_URL unset). Silently
// returning nil here made a deliberately-disabled client indistinguishable
// from a DI wiring bug that forgot to set SOVEREIGN_URL — every knowledge
// mutation looked like it succeeded while doing nothing (CLAUDE.md rule 8 /
// .claude/rules/di-wiring.md).
var ErrSovereignDisabled = errors.New("sovereign_client: disabled (SOVEREIGN_URL unset); mutation rejected instead of silently no-op'ing")

// healthProbeTimeout caps the startup probe so a misrouted upstream cannot
// block process startup. Connect-RPC content-type errors return well under a
// second on localhost; 5 s is safe headroom for slow networks.
const healthProbeTimeout = 5 * time.Second

// maxHealthResponseBytes limits the health response read to prevent memory exhaustion.
const maxHealthResponseBytes = 4096

type healthProbeResponse struct {
	Status  string `json:"status"`
	Service string `json:"service"`
}

// Client provides Connect-RPC client for Knowledge Sovereign.
type Client struct {
	client     sovereignv1connect.KnowledgeSovereignServiceClient
	httpClient *http.Client
	baseURL    string
	token      string
	enabled    bool
}

// Option configures Client.
type Option func(*clientOptions)

type clientOptions struct {
	token      string
	httpClient *http.Client
}

// WithEventToken supplies the Bearer token for authenticating to
// knowledge-sovereign's event listener.
func WithEventToken(token string) Option {
	return func(o *clientOptions) {
		o.token = token
	}
}

// WithHTTPClient supplies a custom HTTP client (e.g. for testing or strict TLS).
func WithHTTPClient(client *http.Client) Option {
	return func(o *clientOptions) {
		o.httpClient = client
	}
}

type clientAuthInterceptor struct {
	token string
}

func (i *clientAuthInterceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		if i.token != "" {
			req.Header().Set("Authorization", "Bearer "+i.token)
		}
		if jwtToken, ok := JWTFromContext(ctx); ok && jwtToken != "" {
			req.Header().Set("X-Alt-Backend-Token", jwtToken)
		}
		return next(ctx, req)
	}
}

func (i *clientAuthInterceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return func(ctx context.Context, spec connect.Spec) connect.StreamingClientConn {
		conn := next(ctx, spec)
		if i.token != "" {
			conn.RequestHeader().Set("Authorization", "Bearer "+i.token)
		}
		if jwtToken, ok := JWTFromContext(ctx); ok && jwtToken != "" {
			conn.RequestHeader().Set("X-Alt-Backend-Token", jwtToken)
		}
		return conn
	}
}

func (i *clientAuthInterceptor) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return next
}

// NewClientAuthInterceptor creates a Connect client interceptor that sets
// the Authorization: Bearer <token> header on outgoing RPCs.
func NewClientAuthInterceptor(token string) connect.Interceptor {
	return &clientAuthInterceptor{token: token}
}

// NewClient creates a new Knowledge Sovereign Connect-RPC client. When
// enabled, NewClient runs a one-shot startup health probe purely to surface
// likely misconfiguration (e.g. a staging slice whose baseURL points at a
// JSON-returning proxy instead of the sovereign service). The probe is
// observational only — it does NOT disable the client on failure, because
// "endpoint not implemented" and "wrong upstream" are not distinguishable
// from the wire (both look like content-type mismatches to connect-go).
// The bounded backoff and circuit breaker on the projector retry loop is
// what actually contains the runtime failure mode (PM-2026-042 P-1).
func NewClient(baseURL string, enabled bool, opts ...Option) *Client {
	if !enabled {
		return &Client{enabled: false}
	}

	var co clientOptions
	for _, opt := range opts {
		opt(&co)
	}

	httpClient := co.httpClient
	if httpClient == nil {
		httpClient = &http.Client{
			Transport: &http.Transport{
				MaxIdleConns:        50,
				MaxIdleConnsPerHost: 25,
				IdleConnTimeout:     90 * time.Second,
			},
			Timeout: 30 * time.Second,
		}
	}

	var clientOpts []connect.ClientOption
	if co.token != "" {
		clientOpts = append(clientOpts, connect.WithInterceptors(NewClientAuthInterceptor(co.token)))
	}

	client := sovereignv1connect.NewKnowledgeSovereignServiceClient(
		httpClient,
		baseURL,
		clientOpts...,
	)
	c := &Client{
		client:     client,
		httpClient: httpClient,
		baseURL:    baseURL,
		token:      co.token,
		enabled:    true,
	}

	c.runHealthProbe(context.Background())
	return c
}

// Enabled reports whether the client will issue real RPCs. A disabled client
// no-ops every mutation call.
func (c *Client) Enabled() bool {
	return c.enabled
}

// runHealthProbe issues a bounded public GET /health request and logs the outcome.
// Both the RPC listener (:9500) and the metrics/ops listener (:9501) expose /health.
// The probe is observational only — it leaves the client enabled so test stubs
// or delayed startup do not crash callers, while surfacing misconfigurations.
func (c *Client) runHealthProbe(ctx context.Context) {
	probeCtx, cancel := context.WithTimeout(ctx, healthProbeTimeout)
	defer cancel()

	healthURL := strings.TrimRight(c.baseURL, "/") + "/health"
	if !strings.HasPrefix(healthURL, "http://") && !strings.HasPrefix(healthURL, "https://") {
		healthURL = "http://" + healthURL
	}

	req, err := http.NewRequestWithContext(probeCtx, http.MethodGet, healthURL, nil)
	if err != nil {
		slog.Warn("knowledge sovereign health probe request creation failed", "base_url", c.baseURL, "error", err)
		return
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		slog.Warn("knowledge sovereign health probe failed", "base_url", c.baseURL, "error", err)
		return
	}
	defer func() {
		// Deliberate best-effort cleanup of response body before caller retries or subsequent RPCs;
		// close error is explicitly ignored to preserve HTTP probe semantics without masking primary errors or logging secrets.
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		slog.Warn("knowledge sovereign health probe returned non-200 status", "base_url", c.baseURL, "status_code", resp.StatusCode)
		return
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxHealthResponseBytes))
	if err != nil {
		slog.Warn("knowledge sovereign health probe failed reading body", "base_url", c.baseURL, "error", err)
		return
	}

	var hr healthProbeResponse
	if err := json.Unmarshal(body, &hr); err != nil {
		slog.Warn("knowledge sovereign health probe returned invalid JSON", "base_url", c.baseURL, "error", err)
		return
	}

	if hr.Status != "ok" {
		slog.Warn("knowledge sovereign health probe reported non-ok status", "base_url", c.baseURL, "status", hr.Status)
		return
	}

	slog.Info("knowledge sovereign health probe ok", "base_url", c.baseURL)
}

// ApplyProjectionMutation implements knowledge_sovereign_port.ProjectionMutator.
func (c *Client) ApplyProjectionMutation(ctx context.Context, mutation knowledge_sovereign_port.ProjectionMutation) error {
	if !c.enabled {
		return ErrSovereignDisabled
	}

	resp, err := c.client.ApplyProjectionMutation(ctx, connect.NewRequest(&sovereignv1.ApplyProjectionMutationRequest{
		MutationType:   mutation.MutationType,
		EntityId:       mutation.EntityID,
		Payload:        mutation.Payload,
		IdempotencyKey: mutation.IdempotencyKey,
	}))
	if err != nil {
		return fmt.Errorf("sovereign ApplyProjectionMutation(%s): %w", mutation.MutationType, err)
	}
	if !resp.Msg.Success {
		return fmt.Errorf("sovereign ApplyProjectionMutation(%s): %s", mutation.MutationType, resp.Msg.ErrorMessage)
	}
	return nil
}

// ApplyRecallMutation implements knowledge_sovereign_port.RecallMutator.
func (c *Client) ApplyRecallMutation(ctx context.Context, mutation knowledge_sovereign_port.RecallMutation) error {
	if !c.enabled {
		return ErrSovereignDisabled
	}

	resp, err := c.client.ApplyRecallMutation(ctx, connect.NewRequest(&sovereignv1.ApplyRecallMutationRequest{
		MutationType:   mutation.MutationType,
		EntityId:       mutation.EntityID,
		Payload:        mutation.Payload,
		IdempotencyKey: mutation.IdempotencyKey,
	}))
	if err != nil {
		return fmt.Errorf("sovereign ApplyRecallMutation(%s): %w", mutation.MutationType, err)
	}
	if !resp.Msg.Success {
		return fmt.Errorf("sovereign ApplyRecallMutation(%s): %s", mutation.MutationType, resp.Msg.ErrorMessage)
	}
	return nil
}

// ApplyCurationMutation implements knowledge_sovereign_port.CurationMutator.
func (c *Client) ApplyCurationMutation(ctx context.Context, mutation knowledge_sovereign_port.CurationMutation) error {
	if !c.enabled {
		return ErrSovereignDisabled
	}

	resp, err := c.client.ApplyCurationMutation(ctx, connect.NewRequest(&sovereignv1.ApplyCurationMutationRequest{
		MutationType:   mutation.MutationType,
		EntityId:       mutation.EntityID,
		Payload:        mutation.Payload,
		IdempotencyKey: mutation.IdempotencyKey,
	}))
	if err != nil {
		return fmt.Errorf("sovereign ApplyCurationMutation(%s): %w", mutation.MutationType, err)
	}
	if !resp.Msg.Success {
		return fmt.Errorf("sovereign ApplyCurationMutation(%s): %s", mutation.MutationType, resp.Msg.ErrorMessage)
	}
	return nil
}
