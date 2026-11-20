package sovereign_client

import (
	"alt/domain"
	sovereignv1 "alt/gen/proto/services/sovereign/v1"
	"alt/utils/safeconv"
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

	if resp.Msg.GetJob() == nil {
		return nil, nil
	}
	job := protoToBackfillJob(resp.Msg.GetJob())
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

	jobs := make([]domain.KnowledgeBackfillJob, 0, len(resp.Msg.GetJobs()))
	for _, pb := range resp.Msg.GetJobs() {
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
		ProjectionVersion: safeconv.Int32(j.ProjectionVersion),
		TotalEvents:       safeconv.Int32(j.TotalEvents),
		ProcessedEvents:   safeconv.Int32(j.ProcessedEvents),
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
		JobID:             parseUUID(pb.GetJobId()),
		Status:            pb.GetStatus(),
		Kind:              pb.GetKind(),
		ProjectionVersion: int(pb.GetProjectionVersion()),
		TotalEvents:       int(pb.GetTotalEvents()),
		ProcessedEvents:   int(pb.GetProcessedEvents()),
		ErrorMessage:      pb.GetErrorMessage(),
		CursorUserID:      parseUUIDPtr(pb.GetCursorUserId()),
		CursorArticleID:   parseUUIDPtr(pb.GetCursorArticleId()),
	}
	if pb.GetCursorDate() != "" {
		t, _ := time.Parse("2006-01-02", pb.GetCursorDate())
		j.CursorDate = &t
	}
	if pb.GetCreatedAt() != nil {
		j.CreatedAt = pb.GetCreatedAt().AsTime()
	}
	if pb.GetStartedAt() != nil {
		t := pb.GetStartedAt().AsTime()
		j.StartedAt = &t
	}
	if pb.GetCompletedAt() != nil {
		t := pb.GetCompletedAt().AsTime()
		j.CompletedAt = &t
	}
	if pb.GetUpdatedAt() != nil {
		j.UpdatedAt = pb.GetUpdatedAt().AsTime()
	}
	return j
}
