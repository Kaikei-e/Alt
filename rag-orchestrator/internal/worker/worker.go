package worker

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"rag-orchestrator/internal/domain"
	"rag-orchestrator/internal/usecase"
)

type JobWorker struct {
	jobRepo      domain.RagJobRepository
	indexUsecase usecase.IndexArticleUsecase
	logger       *slog.Logger
	stopChan     chan struct{}
}

func NewJobWorker(
	jobRepo domain.RagJobRepository,
	indexUsecase usecase.IndexArticleUsecase,
	logger *slog.Logger,
) *JobWorker {
	return &JobWorker{
		jobRepo:      jobRepo,
		indexUsecase: indexUsecase,
		logger:       logger,
		stopChan:     make(chan struct{}),
	}
}

func (w *JobWorker) Start() {
	w.logger.Info("Starting JobWorker")
	go w.run()
}

func (w *JobWorker) Stop() {
	w.logger.Info("Stopping JobWorker")
	close(w.stopChan)
}

func (w *JobWorker) run() {
	ticker := time.NewTicker(1 * time.Second) // Poll interval
	defer ticker.Stop()

	for {
		select {
		case <-w.stopChan:
			return
		case <-ticker.C:
			w.processNextJob()
		}
	}
}

func (w *JobWorker) processNextJob() {
	ctx := context.Background() // Create a new context for each job processing

	job, err := w.jobRepo.AcquireNextJob(ctx)
	if err != nil {
		w.logger.Error("Failed to acquire next job", "error", err)
		return
	}
	if job == nil {
		return // No jobs
	}

	w.logger.Info("Processing job", "job_id", job.ID, "type", job.JobType)

	var processErr error

	switch job.JobType {
	case "backfill_article":
		processErr = w.processBackfillArticle(ctx, job)
	default:
		processErr = fmt.Errorf("unknown job type: %s", job.JobType)
	}

	status := "completed"
	var errMsg *string
	if processErr != nil {
		status = "failed"
		msg := processErr.Error()
		errMsg = &msg
		w.logger.Error("Job failed", "job_id", job.ID, "error", processErr)
	} else {
		w.logger.Info("Job completed", "job_id", job.ID)
	}

	if err := w.jobRepo.UpdateStatus(ctx, job.ID, status, errMsg); err != nil {
		w.logger.Error("Failed to update job status", "job_id", job.ID, "error", err)
	}
}

func (w *JobWorker) processBackfillArticle(ctx context.Context, job *domain.RagJob) error {
	payload := job.Payload
	articleID, ok := payload["article_id"].(string)
	if !ok {
		return fmt.Errorf("missing or invalid article_id")
	}
	title, ok := payload["title"].(string)
	if !ok {
		return fmt.Errorf("missing or invalid title")
	}
	body, ok := payload["body"].(string)
	if !ok {
		return fmt.Errorf("missing or invalid body")
	}
	url, ok := payload["url"].(string) // Optional for existing jobs, but encouraged
	if !ok {
		url = "" // Default if missing
	}

	// Throttling could be implemented here (e.g., token bucket or simple sleep)
	// For now, let's keep it simple as relying on the poll interval acts as a basic rate limiter (1 job/sec/worker)

	return w.indexUsecase.Upsert(ctx, articleID, title, url, body)
}
