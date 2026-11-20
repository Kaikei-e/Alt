package reading_queue_usecase

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"alt/domain"
	"alt/orchestrator/port/reading_queue_port"
	"alt/utils/safeconv"
)

type GetReadTodayQueueResult struct {
	Items          []domain.HydratedReadingQueueItem
	Cap            int
	ReadCountSince int32
}

type GetReadTodayQueueUsecase struct {
	queuePort        reading_queue_port.GetReadingQueuePort
	countPort        reading_queue_port.CountReadingSourcesSincePort
	articleBatchPort reading_queue_port.FetchArticlesByIDsPort
	ogImagePort      reading_queue_port.GetOgImageURLsPort
	cap              int
	now              func() time.Time
}

func NewGetReadTodayQueueUsecase(
	queuePort reading_queue_port.GetReadingQueuePort,
	countPort reading_queue_port.CountReadingSourcesSincePort,
	articleBatchPort reading_queue_port.FetchArticlesByIDsPort,
	ogImagePort reading_queue_port.GetOgImageURLsPort,
	cap int,
	now func() time.Time,
) *GetReadTodayQueueUsecase {
	if queuePort == nil {
		panic("reading_queue_usecase: queuePort is required")
	}
	if countPort == nil {
		panic("reading_queue_usecase: countPort is required")
	}
	if articleBatchPort == nil {
		panic("reading_queue_usecase: articleBatchPort is required")
	}
	if ogImagePort == nil {
		panic("reading_queue_usecase: ogImagePort is required")
	}
	if cap <= 0 {
		panic("reading_queue_usecase: cap must be positive")
	}
	if now == nil {
		panic("reading_queue_usecase: now clock is required")
	}
	return &GetReadTodayQueueUsecase{
		queuePort:        queuePort,
		countPort:        countPort,
		articleBatchPort: articleBatchPort,
		ogImagePort:      ogImagePort,
		cap:              cap,
		now:              now,
	}
}

func (u *GetReadTodayQueueUsecase) Execute(
	ctx context.Context,
	userID uuid.UUID,
	readSince time.Time,
) (GetReadTodayQueueResult, error) {
	if readSince.IsZero() {
		return GetReadTodayQueueResult{}, fmt.Errorf("%w: read_since is required", ErrInvalidInput)
	}

	asOf := u.now().UTC()
	entries, err := u.queuePort.GetReadingQueue(ctx, userID, asOf)
	if err != nil {
		return GetReadTodayQueueResult{}, fmt.Errorf("get read today queue: %w", err)
	}

	readCount, err := u.countPort.CountReadingSourcesSince(ctx, userID, readSince.UTC())
	if err != nil {
		return GetReadTodayQueueResult{}, fmt.Errorf("get read today queue: %w", err)
	}

	items, err := hydrateQueueEntries(ctx, u.articleBatchPort, u.ogImagePort, entries)
	if err != nil {
		return GetReadTodayQueueResult{}, fmt.Errorf("get read today queue: %w", err)
	}

	return GetReadTodayQueueResult{
		Items:          items,
		Cap:            u.cap,
		ReadCountSince: safeconv.Int32(readCount),
	}, nil
}
