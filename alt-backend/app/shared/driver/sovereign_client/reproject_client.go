package sovereign_client

import (
	"alt/domain"
	sovereignv1 "alt/gen/proto/services/sovereign/v1"
	"alt/utils/safeconv"
	"context"
	"encoding/json"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// === Reproject operations ===

func (c *Client) CreateReprojectRun(ctx context.Context, run *domain.ReprojectRun) error {
	if !c.enabled {
		return nil
	}

	_, err := c.client.CreateReprojectRun(ctx, connect.NewRequest(&sovereignv1.CreateReprojectRunRequest{
		Run: domainReprojectRunToProto(run),
	}))
	if err != nil {
		return fmt.Errorf("sovereign CreateReprojectRun: %w", err)
	}
	return nil
}

func (c *Client) GetReprojectRun(ctx context.Context, runID uuid.UUID) (*domain.ReprojectRun, error) {
	if !c.enabled {
		return nil, nil
	}

	resp, err := c.client.GetReprojectRun(ctx, connect.NewRequest(&sovereignv1.GetReprojectRunRequest{
		RunId: runID.String(),
	}))
	if err != nil {
		return nil, fmt.Errorf("sovereign GetReprojectRun: %w", err)
	}

	if resp.Msg.GetRun() == nil {
		return nil, nil
	}
	run := protoToReprojectRun(resp.Msg.GetRun())
	return &run, nil
}

func (c *Client) UpdateReprojectRun(ctx context.Context, run *domain.ReprojectRun) error {
	if !c.enabled {
		return nil
	}

	_, err := c.client.UpdateReprojectRun(ctx, connect.NewRequest(&sovereignv1.UpdateReprojectRunRequest{
		Run: domainReprojectRunToProto(run),
	}))
	if err != nil {
		return fmt.Errorf("sovereign UpdateReprojectRun: %w", err)
	}
	return nil
}

func (c *Client) ListReprojectRuns(ctx context.Context, statusFilter string, limit int) ([]domain.ReprojectRun, error) {
	if !c.enabled {
		return nil, nil
	}

	resp, err := c.client.ListReprojectRuns(ctx, connect.NewRequest(&sovereignv1.ListReprojectRunsRequest{
		StatusFilter: statusFilter,
		Limit:        safeconv.Int32(limit),
	}))
	if err != nil {
		return nil, fmt.Errorf("sovereign ListReprojectRuns: %w", err)
	}

	runs := make([]domain.ReprojectRun, 0, len(resp.Msg.GetRuns()))
	for _, pb := range resp.Msg.GetRuns() {
		runs = append(runs, protoToReprojectRun(pb))
	}
	return runs, nil
}

func (c *Client) CompareProjections(ctx context.Context, fromVersion, toVersion string) (*domain.ReprojectDiffSummary, error) {
	if !c.enabled {
		return nil, nil
	}

	resp, err := c.client.CompareProjections(ctx, connect.NewRequest(&sovereignv1.CompareProjectionsRequest{
		FromVersion: fromVersion,
		ToVersion:   toVersion,
	}))
	if err != nil {
		return nil, fmt.Errorf("sovereign CompareProjections: %w", err)
	}

	s := resp.Msg.GetSummary()
	if s == nil {
		return nil, nil
	}
	return &domain.ReprojectDiffSummary{
		FromItemCount:  int64(s.GetFromCount()),
		ToItemCount:    int64(s.GetToCount()),
		FromAvgScore:   s.GetFromAvgScore(),
		ToAvgScore:     s.GetToAvgScore(),
		FromEmptyCount: int64(s.GetFromEmptySummary()),
		ToEmptyCount:   int64(s.GetToEmptySummary()),
	}, nil
}

func (c *Client) CreateProjectionAudit(ctx context.Context, audit *domain.ProjectionAudit) error {
	if !c.enabled {
		return nil
	}

	_, err := c.client.CreateProjectionAudit(ctx, connect.NewRequest(&sovereignv1.CreateProjectionAuditRequest{
		Audit: &sovereignv1.ProjectionAudit{
			AuditId:           audit.AuditID.String(),
			ProjectionName:    audit.ProjectionName,
			ProjectionVersion: audit.ProjectionVersion,
			CheckedAt:         timeToProto(audit.CheckedAt),
			SampleSize:        safeconv.Int32(audit.SampleSize),
			MismatchCount:     safeconv.Int32(audit.MismatchCount),
			DetailsJson:       audit.DetailsJSON,
		},
	}))
	if err != nil {
		return fmt.Errorf("sovereign CreateProjectionAudit: %w", err)
	}
	return nil
}

func (c *Client) ListProjectionAudits(ctx context.Context, projectionName string, limit int) ([]domain.ProjectionAudit, error) {
	if !c.enabled {
		return nil, nil
	}

	resp, err := c.client.ListProjectionAudits(ctx, connect.NewRequest(&sovereignv1.ListProjectionAuditsRequest{
		ProjectionName: projectionName,
		Limit:          safeconv.Int32(limit),
	}))
	if err != nil {
		return nil, fmt.Errorf("sovereign ListProjectionAudits: %w", err)
	}

	audits := make([]domain.ProjectionAudit, 0, len(resp.Msg.GetAudits()))
	for _, pb := range resp.Msg.GetAudits() {
		a := domain.ProjectionAudit{
			AuditID:           parseUUID(pb.GetAuditId()),
			ProjectionName:    pb.GetProjectionName(),
			ProjectionVersion: pb.GetProjectionVersion(),
			SampleSize:        int(pb.GetSampleSize()),
			MismatchCount:     int(pb.GetMismatchCount()),
			DetailsJSON:       json.RawMessage(pb.GetDetailsJson()),
		}
		if pb.GetCheckedAt() != nil {
			a.CheckedAt = pb.GetCheckedAt().AsTime()
		}
		audits = append(audits, a)
	}
	return audits, nil
}

// === Reproject conversion helpers ===

func domainReprojectRunToProto(run *domain.ReprojectRun) *sovereignv1.ReprojectRun {
	if run == nil {
		return nil
	}
	pb := &sovereignv1.ReprojectRun{
		ReprojectRunId:    run.ReprojectRunID.String(),
		ProjectionName:    run.ProjectionName,
		FromVersion:       run.FromVersion,
		ToVersion:         run.ToVersion,
		Mode:              run.Mode,
		Status:            run.Status,
		CheckpointPayload: run.CheckpointPayload,
		StatsJson:         run.StatsJSON,
		DiffSummaryJson:   run.DiffSummaryJSON,
		CreatedAt:         timeToProto(run.CreatedAt),
	}
	if run.InitiatedBy != nil {
		pb.InitiatedBy = run.InitiatedBy.String()
	}
	if run.RangeStart != nil {
		pb.RangeStart = timestamppb.New(*run.RangeStart)
	}
	if run.RangeEnd != nil {
		pb.RangeEnd = timestamppb.New(*run.RangeEnd)
	}
	if run.StartedAt != nil {
		pb.StartedAt = timestamppb.New(*run.StartedAt)
	}
	if run.FinishedAt != nil {
		pb.FinishedAt = timestamppb.New(*run.FinishedAt)
	}
	return pb
}

func protoToReprojectRun(pb *sovereignv1.ReprojectRun) domain.ReprojectRun {
	if pb == nil {
		return domain.ReprojectRun{}
	}
	run := domain.ReprojectRun{
		ReprojectRunID:    parseUUID(pb.GetReprojectRunId()),
		ProjectionName:    pb.GetProjectionName(),
		FromVersion:       pb.GetFromVersion(),
		ToVersion:         pb.GetToVersion(),
		InitiatedBy:       parseUUIDPtr(pb.GetInitiatedBy()),
		Mode:              pb.GetMode(),
		Status:            pb.GetStatus(),
		CheckpointPayload: json.RawMessage(pb.GetCheckpointPayload()),
		StatsJSON:         json.RawMessage(pb.GetStatsJson()),
		DiffSummaryJSON:   json.RawMessage(pb.GetDiffSummaryJson()),
	}
	if pb.GetCreatedAt() != nil {
		run.CreatedAt = pb.GetCreatedAt().AsTime()
	}
	if pb.GetRangeStart() != nil {
		t := pb.GetRangeStart().AsTime()
		run.RangeStart = &t
	}
	if pb.GetRangeEnd() != nil {
		t := pb.GetRangeEnd().AsTime()
		run.RangeEnd = &t
	}
	if pb.GetStartedAt() != nil {
		t := pb.GetStartedAt().AsTime()
		run.StartedAt = &t
	}
	if pb.GetFinishedAt() != nil {
		t := pb.GetFinishedAt().AsTime()
		run.FinishedAt = &t
	}
	return run
}
