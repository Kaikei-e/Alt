package sovereign_client

import (
	"alt/domain"
	sovereignv1 "alt/gen/proto/services/sovereign/v1"
	"context"
	"fmt"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// === Backfill operations ===

func (c *Client) CreateBackfillJob(ctx context.Context, job domain.KnowledgeBackfillJob) error {
	if !c.enabled {
		return nil
	}

	_, err := c.client.CreateBackfillJob(ctx, connect.NewRequest(&sovereignv1.CreateBackfillJobRequest{
		Job: domainBackfillJobToProto(job),
	}))
	if err != nil {
		return fmt.Errorf("sovereign CreateBackfillJob: %w", err)
	}
	return nil
}

func (c *Client) GetBackfillJob(ctx context.Context, jobID uuid.UUID) (*domain.KnowledgeBackfillJob, error) {
	if !c.enabled {
		return nil, nil
	}

	resp, err := c.client.GetBackfillJob(ctx, connect.NewRequest(&sovereignv1.GetBackfillJobRequest{
		JobId: jobID.String(),
	}))
	if err != nil {
		return nil, fmt.Errorf("sovereign GetBackfillJob: %w", err)
	}

	if resp.Msg.Job == nil {
		return nil, nil
	}
	job := protoToBackfillJob(resp.Msg.Job)
	return &job, nil
}

func (c *Client) UpdateBackfillJob(ctx context.Context, job domain.KnowledgeBackfillJob) error {
	if !c.enabled {
		return nil
	}

	_, err := c.client.UpdateBackfillJob(ctx, connect.NewRequest(&sovereignv1.UpdateBackfillJobRequest{
		Job: domainBackfillJobToProto(job),
	}))
	if err != nil {
		return fmt.Errorf("sovereign UpdateBackfillJob: %w", err)
	}
	return nil
}

func (c *Client) ListBackfillJobs(ctx context.Context) ([]domain.KnowledgeBackfillJob, error) {
	if !c.enabled {
		return nil, nil
	}

	resp, err := c.client.ListBackfillJobs(ctx, connect.NewRequest(&sovereignv1.ListBackfillJobsRequest{}))
	if err != nil {
		return nil, fmt.Errorf("sovereign ListBackfillJobs: %w", err)
	}

	jobs := make([]domain.KnowledgeBackfillJob, 0, len(resp.Msg.Jobs))
	for _, pb := range resp.Msg.Jobs {
		jobs = append(jobs, protoToBackfillJob(pb))
	}
	return jobs, nil
}

// === Backfill conversion helpers ===

func domainBackfillJobToProto(j domain.KnowledgeBackfillJob) *sovereignv1.BackfillJob {
	pb := &sovereignv1.BackfillJob{
		JobId:             j.JobID.String(),
		Status:            j.Status,
		Kind:              j.Kind,
		ProjectionVersion: int32(j.ProjectionVersion),
		TotalEvents:       int32(j.TotalEvents),
		ProcessedEvents:   int32(j.ProcessedEvents),
		ErrorMessage:      j.ErrorMessage,
		CreatedAt:         timeToProto(j.CreatedAt),
		UpdatedAt:         timeToProto(j.UpdatedAt),
	}
	if j.CursorUserID != nil {
		pb.CursorUserId = j.CursorUserID.String()
	}
	if j.CursorDate != nil {
		pb.CursorDate = j.CursorDate.Format("2006-01-02")
	}
	if j.CursorArticleID != nil {
		pb.CursorArticleId = j.CursorArticleID.String()
	}
	if j.StartedAt != nil {
		pb.StartedAt = timestamppb.New(*j.StartedAt)
	}
	if j.CompletedAt != nil {
		pb.CompletedAt = timestamppb.New(*j.CompletedAt)
	}
	return pb
}

func protoToBackfillJob(pb *sovereignv1.BackfillJob) domain.KnowledgeBackfillJob {
	if pb == nil {
		return domain.KnowledgeBackfillJob{}
	}
	j := domain.KnowledgeBackfillJob{
		JobID:             parseUUID(pb.JobId),
		Status:            pb.Status,
		Kind:              pb.Kind,
		ProjectionVersion: int(pb.ProjectionVersion),
		TotalEvents:       int(pb.TotalEvents),
		ProcessedEvents:   int(pb.ProcessedEvents),
		ErrorMessage:      pb.ErrorMessage,
		CursorUserID:      parseUUIDPtr(pb.CursorUserId),
		CursorArticleID:   parseUUIDPtr(pb.CursorArticleId),
	}
	if pb.CursorDate != "" {
		t, _ := time.Parse("2006-01-02", pb.CursorDate)
		j.CursorDate = &t
	}
	if pb.CreatedAt != nil {
		j.CreatedAt = pb.CreatedAt.AsTime()
	}
	if pb.StartedAt != nil {
		t := pb.StartedAt.AsTime()
		j.StartedAt = &t
	}
	if pb.CompletedAt != nil {
		t := pb.CompletedAt.AsTime()
		j.CompletedAt = &t
	}
	if pb.UpdatedAt != nil {
		j.UpdatedAt = pb.UpdatedAt.AsTime()
	}
	return j
}
