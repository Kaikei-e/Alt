package sovereign_client

import (
	"alt/domain"
	sovereignv1 "alt/gen/proto/services/sovereign/v1"
	"alt/utils/safeconv"
	"context"
	"fmt"
	"time"

	"connectrpc.com/connect"
)

// === Projection infra ===

func (c *Client) GetActiveProjectionVersion(ctx context.Context) (*domain.KnowledgeProjectionVersion, error) {
	if !c.enabled {
		return nil, nil
	}
	resp, err := c.client.GetActiveProjectionVersion(ctx, connect.NewRequest(&sovereignv1.GetActiveProjectionVersionRequest{}))
	if err != nil {
		return nil, fmt.Errorf("sovereign GetActiveProjectionVersion: %w", err)
	}
	if resp.Msg.Version == nil {
		return nil, nil
	}
	return protoToProjectionVersion(resp.Msg.Version), nil
}

func (c *Client) ListProjectionVersions(ctx context.Context) ([]domain.KnowledgeProjectionVersion, error) {
	if !c.enabled {
		return nil, nil
	}
	resp, err := c.client.ListProjectionVersions(ctx, connect.NewRequest(&sovereignv1.ListProjectionVersionsRequest{}))
	if err != nil {
		return nil, fmt.Errorf("sovereign ListProjectionVersions: %w", err)
	}
	versions := make([]domain.KnowledgeProjectionVersion, len(resp.Msg.Versions))
	for i, v := range resp.Msg.Versions {
		versions[i] = *protoToProjectionVersion(v)
	}
	return versions, nil
}

func (c *Client) CreateProjectionVersion(ctx context.Context, v domain.KnowledgeProjectionVersion) error {
	if !c.enabled {
		return nil
	}
	_, err := c.client.CreateProjectionVersion(ctx, connect.NewRequest(&sovereignv1.CreateProjectionVersionRequest{
		Version: domainToProtoVersion(v),
	}))
	if err != nil {
		return fmt.Errorf("sovereign CreateProjectionVersion: %w", err)
	}
	return nil
}

func (c *Client) ActivateProjectionVersion(ctx context.Context, version int) error {
	if !c.enabled {
		return nil
	}
	_, err := c.client.ActivateProjectionVersion(ctx, connect.NewRequest(&sovereignv1.ActivateProjectionVersionRequest{
		Version: safeconv.Int32(version),
	}))
	if err != nil {
		return fmt.Errorf("sovereign ActivateProjectionVersion: %w", err)
	}
	return nil
}

func (c *Client) GetProjectionCheckpoint(ctx context.Context, projectorName string) (int64, error) {
	if !c.enabled {
		return 0, nil
	}
	resp, err := c.client.GetProjectionCheckpoint(ctx, connect.NewRequest(&sovereignv1.GetProjectionCheckpointRequest{
		ProjectorName: projectorName,
	}))
	if err != nil {
		return 0, fmt.Errorf("sovereign GetProjectionCheckpoint: %w", err)
	}
	return resp.Msg.LastEventSeq, nil
}

func (c *Client) UpdateProjectionCheckpoint(ctx context.Context, projectorName string, lastSeq int64) error {
	if !c.enabled {
		return nil
	}
	_, err := c.client.UpdateProjectionCheckpoint(ctx, connect.NewRequest(&sovereignv1.UpdateProjectionCheckpointRequest{
		ProjectorName: projectorName, LastEventSeq: lastSeq,
	}))
	if err != nil {
		return fmt.Errorf("sovereign UpdateProjectionCheckpoint: %w", err)
	}
	return nil
}

func (c *Client) GetProjectionLag(ctx context.Context) (time.Duration, error) {
	if !c.enabled {
		return 0, nil
	}
	resp, err := c.client.GetProjectionLag(ctx, connect.NewRequest(&sovereignv1.GetProjectionLagRequest{}))
	if err != nil {
		return 0, fmt.Errorf("sovereign GetProjectionLag: %w", err)
	}
	if resp.Msg.LagSeconds < 0 {
		return time.Duration(-1), nil
	}
	return time.Duration(resp.Msg.LagSeconds * float64(time.Second)), nil
}

func (c *Client) GetProjectionAge(ctx context.Context) (time.Duration, error) {
	if !c.enabled {
		return 0, nil
	}
	resp, err := c.client.GetProjectionLag(ctx, connect.NewRequest(&sovereignv1.GetProjectionLagRequest{}))
	if err != nil {
		return 0, fmt.Errorf("sovereign GetProjectionAge: %w", err)
	}
	if resp.Msg.AgeSeconds < 0 {
		return time.Duration(-1), nil
	}
	return time.Duration(resp.Msg.AgeSeconds * float64(time.Second)), nil
}

// === Port interface aliases ===
// These methods alias the longer names to satisfy the shorter port interfaces.

func (c *Client) GetActiveVersion(ctx context.Context) (*domain.KnowledgeProjectionVersion, error) {
	return c.GetActiveProjectionVersion(ctx)
}

func (c *Client) ListVersions(ctx context.Context) ([]domain.KnowledgeProjectionVersion, error) {
	return c.ListProjectionVersions(ctx)
}

func (c *Client) CreateVersion(ctx context.Context, v domain.KnowledgeProjectionVersion) error {
	return c.CreateProjectionVersion(ctx, v)
}

func (c *Client) ActivateVersion(ctx context.Context, version int) error {
	return c.ActivateProjectionVersion(ctx, version)
}
