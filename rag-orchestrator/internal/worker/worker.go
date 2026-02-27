package worker

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"rag-orchestrator/internal/domain"
	"rag-orchestrator/internal/usecase"
)

const (
	defaultPollInterval = 100 * time.Millisecond
	jobTimeout          = 60 * time.Second
	initialBackoff      = 1 * time.Second
	maxBackoff          = 5 * time.Minute
)

type JobWorker struct {
	jobRepo      domain.RagJobRepository
	indexUsecase usecase.IndexArticleUsecase
	logger       *slog.Logger
	stopChan     chan struct{}
	backoff      time.Duration
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
	ticker := time.NewTicker(defaultPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-w.stopChan:
			return
		case <-ticker.C:
			w.processNextJob()
			if w.backoff > 0 {
				ticker.Reset(w.backoff)
			} else {
				ticker.Reset(defaultPollInterval)
			}
		}
	}
}

func (w *JobWorker) processNextJob() {
	ctx, cancel := context.WithTimeout(context.Background(), jobTimeout)
	defer cancel()

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
		w.backoff = w.nextBackoff(w.backoff)
		w.logger.Warn("Worker backing off", "job_id", job.ID, "backoff", w.backoff, "error", processErr)
	} else {
		w.backoff = 0
		w.logger.Info("Job completed", "job_id", job.ID)
	}

	if err := w.jobRepo.UpdateStatus(ctx, job.ID, status, errMsg); err != nil {
		w.logger.Error("Failed to update job status", "job_id", job.ID, "error", err)
	}
}

func (w *JobWorker) nextBackoff(current time.Duration) time.Duration {
	if current == 0 {
		return initialBackoff
	}
	next := current * 2
	if next > maxBackoff {
		return maxBackoff
	}
	return next
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
