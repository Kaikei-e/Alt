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
		// A disabled write must reject, not fake success (CLAUDE.md rule 8).
		return ErrSovereignDisabled
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

	_, err := c.client.CreateLens(ctx, connect.NewRequest(&sovereignv1.CreateLensRequest{
		Lens: pb,
	}))
	if err != nil {
		return fmt.Errorf("sovereign CreateLens: %w", err)
	}
	return nil
}

func (c *Client) CreateLensVersion(ctx context.Context, version domain.KnowledgeLensVersion) error {
	if !c.enabled {
		// A disabled write must reject, not fake success (CLAUDE.md rule 8).
		return ErrSovereignDisabled
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

	_, err := c.client.CreateLensVersion(ctx, connect.NewRequest(&sovereignv1.CreateLensVersionRequest{
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

	lenses := make([]domain.KnowledgeLens, 0, len(resp.Msg.GetLenses()))
	for _, pb := range resp.Msg.GetLenses() {
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
		switch connect.CodeOf(err) {
		case connect.CodePermissionDenied, connect.CodeNotFound:
			return nil, fmt.Errorf("sovereign GetLens: %w: %w", domain.ErrLensNotFound, err)
		default:
			return nil, fmt.Errorf("sovereign GetLens: %w", err)
		}
	}

	if resp.Msg.GetLens() == nil {
		return nil, nil
	}
	lens := protoToLens(resp.Msg.GetLens())
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

	if !resp.Msg.GetFound() || resp.Msg.GetSelection() == nil {
		return nil, nil
	}

	sel := resp.Msg.GetSelection()
	current := domain.KnowledgeCurrentLens{
		UserID:        parseUUID(sel.GetUserId()),
		LensID:        parseUUID(sel.GetLensId()),
		LensVersionID: parseUUID(sel.GetLensVersionId()),
	}
	if sel.GetSelectedAt() != nil {
		current.SelectedAt = sel.GetSelectedAt().AsTime()
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

	if resp.Msg.GetLens() == nil || resp.Msg.GetLens().GetCurrentVersion() == nil {
		return nil, nil
	}

	v := protoToLensVersion(resp.Msg.GetLens().GetCurrentVersion())
	return &v, nil
}

func (c *Client) SelectCurrentLens(ctx context.Context, current domain.KnowledgeCurrentLens) error {
	if !c.enabled {
		// A disabled write must reject, not fake success (CLAUDE.md rule 8).
		return ErrSovereignDisabled
	}

	sel := &sovereignv1.CurrentLensSelection{
		UserId:        current.UserID.String(),
		LensId:        current.LensID.String(),
		LensVersionId: current.LensVersionID.String(),
		SelectedAt:    timeToProto(current.SelectedAt),
	}

	_, err := c.client.SelectCurrentLens(ctx, connect.NewRequest(&sovereignv1.SelectCurrentLensRequest{
		Selection: sel,
	}))
	if err != nil {
		return fmt.Errorf("sovereign SelectCurrentLens: %w", err)
	}
	return nil
}

func (c *Client) ClearCurrentLens(ctx context.Context, userID uuid.UUID) error {
	if !c.enabled {
		// A disabled write must reject, not fake success (CLAUDE.md rule 8).
		return ErrSovereignDisabled
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
		// A disabled write must reject, not fake success (CLAUDE.md rule 8).
		return ErrSovereignDisabled
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

	if !resp.Msg.GetFound() || resp.Msg.GetFilter() == nil {
		return nil, nil
	}

	f := resp.Msg.GetFilter()
	sourceIDs := make([]uuid.UUID, 0, len(f.GetSourceIds()))
	for _, s := range f.GetSourceIds() {
		sourceIDs = append(sourceIDs, parseUUID(s))
	}

	filter := &domain.KnowledgeHomeLensFilter{
		QueryText:  f.GetQueryText(),
		TagNames:   f.GetTagIds(),
		SourceIDs:  sourceIDs,
		TimeWindow: f.GetTimeWindow(),
	}
	if lensID != nil {
		filter.LensID = *lensID
	}
	return filter, nil
}

// === Lens conversion helpers ===

func protoToLens(pb *sovereignv1.Lens) domain.KnowledgeLens {
	lens := domain.KnowledgeLens{
		LensID:      parseUUID(pb.GetLensId()),
		UserID:      parseUUID(pb.GetUserId()),
		TenantID:    parseUUID(pb.GetTenantId()),
		Name:        pb.GetName(),
		Description: pb.GetDescription(),
	}
	if pb.GetCreatedAt() != nil {
		lens.CreatedAt = pb.GetCreatedAt().AsTime()
	}
	if pb.GetUpdatedAt() != nil {
		lens.UpdatedAt = pb.GetUpdatedAt().AsTime()
	}
	if pb.GetArchivedAt() != nil {
		t := pb.GetArchivedAt().AsTime()
		lens.ArchivedAt = &t
	}
	if pb.GetCurrentVersion() != nil {
		v := protoToLensVersion(pb.GetCurrentVersion())
		lens.CurrentVersion = &v
	}
	return lens
}

func protoToLensVersion(pb *sovereignv1.LensVersion) domain.KnowledgeLensVersion {
	v := domain.KnowledgeLensVersion{
		LensVersionID: parseUUID(pb.GetLensVersionId()),
		LensID:        parseUUID(pb.GetLensId()),
		QueryText:     pb.GetQueryText(),
		TagIDs:        pb.GetTagIds(),
		SourceIDs:     pb.GetSourceIds(),
		TimeWindow:    pb.GetTimeWindow(),
		IncludeRecap:  pb.GetIncludeRecap(),
		IncludePulse:  pb.GetIncludePulse(),
		SortMode:      pb.GetSortMode(),
		SupersededBy:  parseUUIDPtr(pb.GetSupersededBy()),
	}
	if pb.GetCreatedAt() != nil {
		v.CreatedAt = pb.GetCreatedAt().AsTime()
	}
	return v
}
