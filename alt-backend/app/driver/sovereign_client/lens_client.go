package sovereign_client

import (
	"alt/domain"
	sovereignv1 "alt/gen/proto/services/sovereign/v1"
	"context"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// === Lens operations ===

func (c *Client) CreateLens(ctx context.Context, lens domain.KnowledgeLens) error {
	if !c.enabled {
		return nil
	}

	pb := &sovereignv1.Lens{
		LensId:      lens.LensID.String(),
		UserId:      lens.UserID.String(),
		TenantId:    lens.TenantID.String(),
		Name:        lens.Name,
		Description: lens.Description,
		CreatedAt:   timeToProto(lens.CreatedAt),
		UpdatedAt:   timeToProto(lens.UpdatedAt),
	}
	if lens.ArchivedAt != nil {
		pb.ArchivedAt = timestamppb.New(*lens.ArchivedAt)
	}

	_, err := c.client.CreateLens(ctx, connect.NewRequest(&sovereignv1.CreateLensRpcRequest{
		Lens: pb,
	}))
	if err != nil {
		return fmt.Errorf("sovereign CreateLens: %w", err)
	}
	return nil
}

func (c *Client) CreateLensVersion(ctx context.Context, version domain.KnowledgeLensVersion) error {
	if !c.enabled {
		return nil
	}

	pv := &sovereignv1.LensVersion{
		LensVersionId: version.LensVersionID.String(),
		LensId:        version.LensID.String(),
		CreatedAt:     timeToProto(version.CreatedAt),
		QueryText:     version.QueryText,
		TagIds:        version.TagIDs,
		SourceIds:     version.SourceIDs,
		TimeWindow:    version.TimeWindow,
		IncludeRecap:  version.IncludeRecap,
		IncludePulse:  version.IncludePulse,
		SortMode:      version.SortMode,
	}
	if version.SupersededBy != nil {
		pv.SupersededBy = version.SupersededBy.String()
	}

	_, err := c.client.CreateLensVersion(ctx, connect.NewRequest(&sovereignv1.CreateLensVersionRpcRequest{
		Version: pv,
	}))
	if err != nil {
		return fmt.Errorf("sovereign CreateLensVersion: %w", err)
	}
	return nil
}

func (c *Client) ListLenses(ctx context.Context, userID uuid.UUID) ([]domain.KnowledgeLens, error) {
	if !c.enabled {
		return nil, nil
	}

	resp, err := c.client.ListLenses(ctx, connect.NewRequest(&sovereignv1.ListLensesRequest{
		UserId: userID.String(),
	}))
	if err != nil {
		return nil, fmt.Errorf("sovereign ListLenses: %w", err)
	}

	lenses := make([]domain.KnowledgeLens, 0, len(resp.Msg.Lenses))
	for _, pb := range resp.Msg.Lenses {
		lenses = append(lenses, protoToLens(pb))
	}
	return lenses, nil
}

func (c *Client) GetLens(ctx context.Context, lensID uuid.UUID) (*domain.KnowledgeLens, error) {
	if !c.enabled {
		return nil, nil
	}

	resp, err := c.client.GetLens(ctx, connect.NewRequest(&sovereignv1.GetLensRequest{
		LensId: lensID.String(),
	}))
	if err != nil {
		return nil, fmt.Errorf("sovereign GetLens: %w", err)
	}

	if resp.Msg.Lens == nil {
		return nil, nil
	}
	lens := protoToLens(resp.Msg.Lens)
	return &lens, nil
}

func (c *Client) GetCurrentLensSelection(ctx context.Context, userID uuid.UUID) (*domain.KnowledgeCurrentLens, error) {
	if !c.enabled {
		return nil, nil
	}

	resp, err := c.client.GetCurrentLensSelection(ctx, connect.NewRequest(&sovereignv1.GetCurrentLensSelectionRequest{
		UserId: userID.String(),
	}))
	if err != nil {
		return nil, fmt.Errorf("sovereign GetCurrentLensSelection: %w", err)
	}

	if !resp.Msg.Found || resp.Msg.Selection == nil {
		return nil, nil
	}

	sel := resp.Msg.Selection
	current := domain.KnowledgeCurrentLens{
		UserID:        parseUUID(sel.UserId),
		LensID:        parseUUID(sel.LensId),
		LensVersionID: parseUUID(sel.LensVersionId),
	}
	if sel.SelectedAt != nil {
		current.SelectedAt = sel.SelectedAt.AsTime()
	}
	return &current, nil
}

func (c *Client) GetCurrentLensVersion(ctx context.Context, lensID uuid.UUID) (*domain.KnowledgeLensVersion, error) {
	if !c.enabled {
		return nil, nil
	}

	resp, err := c.client.GetLens(ctx, connect.NewRequest(&sovereignv1.GetLensRequest{
		LensId: lensID.String(),
	}))
	if err != nil {
		return nil, fmt.Errorf("sovereign GetCurrentLensVersion: %w", err)
	}

	if resp.Msg.Lens == nil || resp.Msg.Lens.CurrentVersion == nil {
		return nil, nil
	}

	v := protoToLensVersion(resp.Msg.Lens.CurrentVersion)
	return &v, nil
}

func (c *Client) SelectCurrentLens(ctx context.Context, current domain.KnowledgeCurrentLens) error {
	if !c.enabled {
		return nil
	}

	sel := &sovereignv1.CurrentLensSelection{
		UserId:        current.UserID.String(),
		LensId:        current.LensID.String(),
		LensVersionId: current.LensVersionID.String(),
		SelectedAt:    timeToProto(current.SelectedAt),
	}

	_, err := c.client.SelectCurrentLens(ctx, connect.NewRequest(&sovereignv1.SelectCurrentLensRpcRequest{
		Selection: sel,
	}))
	if err != nil {
		return fmt.Errorf("sovereign SelectCurrentLens: %w", err)
	}
	return nil
}

func (c *Client) ClearCurrentLens(ctx context.Context, userID uuid.UUID) error {
	if !c.enabled {
		return nil
	}

	_, err := c.client.ClearCurrentLens(ctx, connect.NewRequest(&sovereignv1.ClearCurrentLensRequest{
		UserId: userID.String(),
	}))
	if err != nil {
		return fmt.Errorf("sovereign ClearCurrentLens: %w", err)
	}
	return nil
}

func (c *Client) ArchiveLens(ctx context.Context, lensID uuid.UUID) error {
	if !c.enabled {
		return nil
	}

	_, err := c.client.ArchiveLens(ctx, connect.NewRequest(&sovereignv1.ArchiveLensRequest{
		LensId: lensID.String(),
	}))
	if err != nil {
		return fmt.Errorf("sovereign ArchiveLens: %w", err)
	}
	return nil
}

func (c *Client) ResolveKnowledgeHomeLens(ctx context.Context, userID uuid.UUID, lensID *uuid.UUID) (*domain.KnowledgeHomeLensFilter, error) {
	if !c.enabled {
		return nil, nil
	}

	req := &sovereignv1.ResolveLensFilterRequest{
		UserId: userID.String(),
	}
	if lensID != nil {
		req.LensId = lensID.String()
	}

	resp, err := c.client.ResolveLensFilter(ctx, connect.NewRequest(req))
	if err != nil {
		return nil, fmt.Errorf("sovereign ResolveLensFilter: %w", err)
	}

	if !resp.Msg.Found || resp.Msg.Filter == nil {
		return nil, nil
	}

	f := resp.Msg.Filter
	sourceIDs := make([]uuid.UUID, 0, len(f.SourceIds))
	for _, s := range f.SourceIds {
		sourceIDs = append(sourceIDs, parseUUID(s))
	}

	filter := &domain.KnowledgeHomeLensFilter{
		QueryText:  f.QueryText,
		TagNames:   f.TagIds,
		SourceIDs:  sourceIDs,
		TimeWindow: f.TimeWindow,
	}
	if lensID != nil {
		filter.LensID = *lensID
	}
	return filter, nil
}

// === Lens conversion helpers ===

func protoToLens(pb *sovereignv1.Lens) domain.KnowledgeLens {
	lens := domain.KnowledgeLens{
		LensID:      parseUUID(pb.LensId),
		UserID:      parseUUID(pb.UserId),
		TenantID:    parseUUID(pb.TenantId),
		Name:        pb.Name,
		Description: pb.Description,
	}
	if pb.CreatedAt != nil {
		lens.CreatedAt = pb.CreatedAt.AsTime()
	}
	if pb.UpdatedAt != nil {
		lens.UpdatedAt = pb.UpdatedAt.AsTime()
	}
	if pb.ArchivedAt != nil {
		t := pb.ArchivedAt.AsTime()
		lens.ArchivedAt = &t
	}
	if pb.CurrentVersion != nil {
		v := protoToLensVersion(pb.CurrentVersion)
		lens.CurrentVersion = &v
	}
	return lens
}

func protoToLensVersion(pb *sovereignv1.LensVersion) domain.KnowledgeLensVersion {
	v := domain.KnowledgeLensVersion{
		LensVersionID: parseUUID(pb.LensVersionId),
		LensID:        parseUUID(pb.LensId),
		QueryText:     pb.QueryText,
		TagIDs:        pb.TagIds,
		SourceIDs:     pb.SourceIds,
		TimeWindow:    pb.TimeWindow,
		IncludeRecap:  pb.IncludeRecap,
		IncludePulse:  pb.IncludePulse,
		SortMode:      pb.SortMode,
		SupersededBy:  parseUUIDPtr(pb.SupersededBy),
	}
	if pb.CreatedAt != nil {
		v.CreatedAt = pb.CreatedAt.AsTime()
	}
	return v
}
