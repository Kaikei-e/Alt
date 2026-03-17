package knowledge_backfill_usecase

import (
	"alt/domain"
	"alt/port/knowledge_backfill_port"
	"alt/port/knowledge_event_port"
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Usecase orchestrates backfill job lifecycle.
type Usecase struct {
	createPort knowledge_backfill_port.CreateBackfillJobPort
	getPort    knowledge_backfill_port.GetBackfillJobPort
	updatePort knowledge_backfill_port.UpdateBackfillJobPort
	listPort   knowledge_backfill_port.ListBackfillJobsPort
	countPort  knowledge_backfill_port.CountBackfillArticlesPort
	eventPort  knowledge_event_port.AppendKnowledgeEventPort
}

// NewUsecase creates a new backfill usecase.
func NewUsecase(
	createPort knowledge_backfill_port.CreateBackfillJobPort,
	getPort knowledge_backfill_port.GetBackfillJobPort,
	updatePort knowledge_backfill_port.UpdateBackfillJobPort,
	listPort knowledge_backfill_port.ListBackfillJobsPort,
	countPort knowledge_backfill_port.CountBackfillArticlesPort,
	eventPort knowledge_event_port.AppendKnowledgeEventPort,
) *Usecase {
	return &Usecase{
		createPort: createPort,
		getPort:    getPort,
		updatePort: updatePort,
		listPort:   listPort,
		countPort:  countPort,
		eventPort:  eventPort,
	}
}

// StartBackfill creates a new pending backfill job for the given projection version.
func (u *Usecase) StartBackfill(ctx context.Context, projectionVersion int) (*domain.KnowledgeBackfillJob, error) {
	now := time.Now()
	totalEvents := 0
	if u.countPort != nil {
		count, err := u.countPort.CountBackfillArticles(ctx)
		if err != nil {
			return nil, fmt.Errorf("count backfill articles: %w", err)
		}
		totalEvents = count
	}
	job := domain.KnowledgeBackfillJob{
		JobID:             uuid.New(),
		Status:            domain.BackfillStatusPending,
		ProjectionVersion: projectionVersion,
		TotalEvents:       totalEvents,
		CreatedAt:         now,
		UpdatedAt:         now,
	}

	if err := u.createPort.CreateBackfillJob(ctx, job); err != nil {
		return nil, fmt.Errorf("start backfill: %w", err)
	}
	return &job, nil
}

// PauseBackfill pauses a running backfill job.
func (u *Usecase) PauseBackfill(ctx context.Context, jobID uuid.UUID) error {
	job, err := u.getPort.GetBackfillJob(ctx, jobID)
	if err != nil {
		return fmt.Errorf("pause backfill: %w", err)
	}
	if job.Status != domain.BackfillStatusRunning {
		return fmt.Errorf("cannot pause job in status %q", job.Status)
	}

	job.Status = domain.BackfillStatusPaused
	if err := u.updatePort.UpdateBackfillJob(ctx, *job); err != nil {
		return fmt.Errorf("pause backfill update: %w", err)
	}
	return nil
}

// ResumeBackfill resumes a paused backfill job.
func (u *Usecase) ResumeBackfill(ctx context.Context, jobID uuid.UUID) error {
	job, err := u.getPort.GetBackfillJob(ctx, jobID)
	if err != nil {
		return fmt.Errorf("resume backfill: %w", err)
	}
	if job.Status != domain.BackfillStatusPaused {
		return fmt.Errorf("cannot resume job in status %q", job.Status)
	}

	job.Status = domain.BackfillStatusRunning
	if err := u.updatePort.UpdateBackfillJob(ctx, *job); err != nil {
		return fmt.Errorf("resume backfill update: %w", err)
	}
	return nil
}

// GetBackfillStatus returns the current status of a backfill job.
func (u *Usecase) GetBackfillStatus(ctx context.Context, jobID uuid.UUID) (*domain.KnowledgeBackfillJob, error) {
	job, err := u.getPort.GetBackfillJob(ctx, jobID)
	if err != nil {
		return nil, fmt.Errorf("get backfill status: %w", err)
	}
	return job, nil
}

// ListBackfillJobs returns all backfill jobs.
func (u *Usecase) ListBackfillJobs(ctx context.Context) ([]domain.KnowledgeBackfillJob, error) {
	jobs, err := u.listPort.ListBackfillJobs(ctx)
	if err != nil {
		return nil, fmt.Errorf("list backfill jobs: %w", err)
	}
	return jobs, nil
}
