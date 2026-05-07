// Package sovereign_client provides a Connect-RPC client for the
// Knowledge Sovereign service. It implements the port interfaces
// (ProjectionMutator, RecallMutator, CurationMutator) so that
// alt-backend can route all knowledge write operations to the
// independent sovereign service.
package sovereign_client

import (
	"alt/port/knowledge_sovereign_port"
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"connectrpc.com/connect"

	sovereignv1 "alt/gen/proto/services/sovereign/v1"
	"alt/gen/proto/services/sovereign/v1/sovereignv1connect"
)

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
// enabled, NewClient performs a startup health probe to detect upstream
// misrouting (e.g. a staging slice whose baseURL points at a JSON-returning
// proxy instead of the sovereign service). Detected misroutes degrade the
// client to disabled so the caller does not enter a content-type retry loop.
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

	if !c.healthProbe(context.Background()) {
		c.enabled = false
	}
	return c
}

// Enabled reports whether the client will issue real RPCs. A disabled client
// no-ops every mutation call.
func (c *Client) Enabled() bool {
	return c.enabled
}

// healthProbe issues one cheap unary RPC and reports whether the response
// looks like a Connect-RPC compatible upstream. Network errors and
// application-level errors (e.g. CodeUnimplemented) are treated as healthy:
// they prove the upstream speaks Connect even if this specific method is
// unavailable. Only content-type mismatches are treated as misroutes.
func (c *Client) healthProbe(ctx context.Context) bool {
	probeCtx, cancel := context.WithTimeout(ctx, healthProbeTimeout)
	defer cancel()

	_, err := c.client.GetActiveProjectionVersion(probeCtx,
		connect.NewRequest(&sovereignv1.GetActiveProjectionVersionRequest{}))
	if err == nil {
		return true
	}
	if isContentTypeMismatch(err) {
		slog.Warn("knowledge sovereign health probe detected upstream content-type mismatch; degrading client",
			"base_url", c.baseURL,
			"error", err,
			"hint", "verify the configured KNOWLEDGE_SOVEREIGN_BASE_URL routes to the sovereign service, not a JSON proxy")
		return false
	}
	// Any other error (Unimplemented, network blip, transient code) is
	// treated as healthy — runtime retries with the bounded-backoff loop
	// will recover. Fail-fast only on the misroute signature.
	slog.Info("knowledge sovereign health probe returned a non-fatal error; client stays enabled",
		"base_url", c.baseURL,
		"error", err)
	return true
}

func isContentTypeMismatch(err error) bool {
	return err != nil && strings.Contains(err.Error(), "invalid content-type")
}

// ApplyProjectionMutation implements knowledge_sovereign_port.ProjectionMutator.
func (c *Client) ApplyProjectionMutation(ctx context.Context, mutation knowledge_sovereign_port.ProjectionMutation) error {
	if !c.enabled {
		return nil
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
		return nil
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
		return nil
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
