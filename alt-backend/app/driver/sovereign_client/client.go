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
	"net/http"
	"time"

	"connectrpc.com/connect"

	sovereignv1 "alt/gen/proto/services/sovereign/v1"
	"alt/gen/proto/services/sovereign/v1/sovereignv1connect"
)

// Client provides Connect-RPC client for Knowledge Sovereign.
type Client struct {
	client  sovereignv1connect.KnowledgeSovereignServiceClient
	baseURL string
	enabled bool
}

// NewClient creates a new Knowledge Sovereign Connect-RPC client.
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
	return &Client{
		client:  client,
		baseURL: baseURL,
		enabled: true,
	}
}

// ApplyProjectionMutation implements knowledge_sovereign_port.ProjectionMutator.
func (c *Client) ApplyProjectionMutation(ctx context.Context, mutation knowledge_sovereign_port.ProjectionMutation) error {
	if !c.enabled {
		return nil
	}

	resp, err := c.client.ApplyProjectionMutation(ctx, connect.NewRequest(&sovereignv1.ApplyMutationRequest{
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

	resp, err := c.client.ApplyRecallMutation(ctx, connect.NewRequest(&sovereignv1.ApplyMutationRequest{
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

	resp, err := c.client.ApplyCurationMutation(ctx, connect.NewRequest(&sovereignv1.ApplyMutationRequest{
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
