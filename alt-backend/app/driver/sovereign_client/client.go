// Package sovereign_client provides a Connect-RPC client for the
// Knowledge Sovereign service. It implements the port interfaces
// (ProjectionMutator, RecallMutator, CurationMutator) so that
// alt-backend can route all knowledge write operations to the
// independent sovereign service.
package sovereign_client

import (
	"alt/port/knowledge_sovereign_port"
	"context"
	"errors"
	"fmt"
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

// Client provides Connect-RPC client for Knowledge Sovereign.
type Client struct {
	client  sovereignv1connect.KnowledgeSovereignServiceClient
	baseURL string
	enabled bool
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
func NewClient(baseURL string, enabled bool) *Client {
	if !enabled {
		return &Client{enabled: false}
	}

	httpClient := &http.Client{
		Transport: &http.Transport{
			MaxIdleConns:        50,
			MaxIdleConnsPerHost: 25,
			IdleConnTimeout:     90 * time.Second,
		},
		Timeout: 30 * time.Second,
	}
	client := sovereignv1connect.NewKnowledgeSovereignServiceClient(
		httpClient,
		baseURL,
	)
	c := &Client{
		client:  client,
		baseURL: baseURL,
		enabled: true,
	}

	c.runHealthProbe(context.Background())
	return c
}

// Enabled reports whether the client will issue real RPCs. A disabled client
// no-ops every mutation call.
func (c *Client) Enabled() bool {
	return c.enabled
}

// runHealthProbe issues one cheap unary RPC and logs the outcome. The probe
// always lets the caller stay enabled; its job is to make startup-time
// misconfiguration loud in operator-facing logs, not to silently degrade.
func (c *Client) runHealthProbe(ctx context.Context) {
	probeCtx, cancel := context.WithTimeout(ctx, healthProbeTimeout)
	defer cancel()

	_, err := c.client.GetActiveProjectionVersion(probeCtx,
		connect.NewRequest(&sovereignv1.GetActiveProjectionVersionRequest{}))
	if err == nil {
		slog.Info("knowledge sovereign health probe ok", "base_url", c.baseURL)
		return
	}
	if isContentTypeMismatch(err) {
		// This signal is shared by two very different conditions: a real
		// upstream misroute (the PM-2026-042 staging slice scenario) AND a
		// reachable Connect server that simply does not implement the probe
		// method (common in test stubs). Surface a loud warning so operators
		// can investigate, but keep the client enabled so legitimate stub
		// environments are not broken.
		slog.Warn("knowledge sovereign health probe saw non-Connect response; verify upstream routing",
			"base_url", c.baseURL,
			"error", err,
			"hint", "if running against the real sovereign service, check KNOWLEDGE_SOVEREIGN_BASE_URL")
		return
	}
	slog.Info("knowledge sovereign health probe returned a non-fatal error; client stays enabled",
		"base_url", c.baseURL,
		"error", err)
}

func isContentTypeMismatch(err error) bool {
	return err != nil && strings.Contains(err.Error(), "invalid content-type")
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
