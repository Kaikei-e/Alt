package service_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"

	"pre-processor/domain"
	"pre-processor/service"
	"pre-processor/test/mocks"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func noRefill(context.Context) (bool, error) {
	return false, nil
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

type signalLogHandler struct {
	slog.Handler
	targetMsg string
	onMatch   func()
}

func (h *signalLogHandler) Handle(ctx context.Context, r slog.Record) error {
	err := h.Handler.Handle(ctx, r)
	if r.Message == h.targetMsg && h.onMatch != nil {
		h.onMatch()
	}
	return err
}

func newSignalLogger(targetMsg string, onMatch func()) *slog.Logger {
	base := discardLogger()
	return slog.New(&signalLogHandler{
		Handler:   base.Handler(),
		targetMsg: targetMsg,
		onMatch:   onMatch,
	})
}

func TestSummarizeQueueWorker_ProcessQueue_ContinuousDispatchWithoutBatchBarrier(t *testing.T) {
	ctx := context.Background()
	ctrl := gomock.NewController(t)

	jobRepo := mocks.NewMockSummarizeJobRepository(ctrl)
	articleRepo := mocks.NewMockArticleRepository(ctrl)
	apiRepo := mocks.NewMockExternalAPIRepository(ctrl)
	summaryRepo := mocks.NewMockSummaryRepository(ctrl)

	jobs := []*domain.SummarizeJob{
		{JobID: uuid.New(), ArticleID: "straggler-1", MaxRetries: 3},
		{JobID: uuid.New(), ArticleID: "fast-2", MaxRetries: 3},
		{JobID: uuid.New(), ArticleID: "fast-3", MaxRetries: 3},
		{JobID: uuid.New(), ArticleID: "fast-4", MaxRetries: 3},
		{JobID: uuid.New(), ArticleID: "fast-5", MaxRetries: 3},
	}

	var queueMu sync.Mutex
	pending := append([]*domain.SummarizeJob(nil), jobs...)
	var inFlight atomic.Int32
	var maxInFlight atomic.Int32
	var runningInDB atomic.Int32
	var maxRunningInDB atomic.Int32

	recordMax := func(target *atomic.Int32, val int32) {
		for {
			cur := target.Load()
			if val <= cur || target.CompareAndSwap(cur, val) {
				return
			}
		}
	}

	stragglerStarted := make(chan struct{})
	releaseStraggler := make(chan struct{})
	fast4And5Done := make(chan struct{})
	var fastCompleted atomic.Int32

	jobRepo.EXPECT().RecoverStuckJobs(gomock.Any()).Return(int64(0), nil)
	jobRepo.EXPECT().DequeueJobs(gomock.Any(), 1).DoAndReturn(func(_ context.Context, limit int) ([]*domain.SummarizeJob, error) {
		assert.Equal(t, 1, limit, "each free worker must dequeue 1 job on demand")
		queueMu.Lock()
		defer queueMu.Unlock()
		if len(pending) == 0 {
			return nil, nil
		}
		j := pending[0]
		pending = pending[1:]
		curRun := runningInDB.Add(1)
		recordMax(&maxRunningInDB, curRun)
		return []*domain.SummarizeJob{j}, nil
	}).AnyTimes()

	articleRepo.EXPECT().FindByID(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, id string) (*domain.Article, error) {
		return &domain.Article{ID: id, UserID: "u1", Title: "Title", Content: "Body text"}, nil
	}).Times(5)

	apiRepo.EXPECT().SummarizeArticle(gomock.Any(), gomock.Any(), "low").DoAndReturn(func(_ context.Context, a *domain.Article, _ string) (*domain.SummarizedContent, error) {
		cur := inFlight.Add(1)
		recordMax(&maxInFlight, cur)
		defer inFlight.Add(-1)

		if a.ID == "straggler-1" {
			close(stragglerStarted)
			<-releaseStraggler
		} else {
			<-stragglerStarted
		}
		return &domain.SummarizedContent{SummaryJapanese: "要約:" + a.ID}, nil
	}).Times(5)

	summaryRepo.EXPECT().Create(gomock.Any(), gomock.Any()).Return(nil).Times(5)

	jobRepo.EXPECT().CompleteJobWithNotification(gomock.Any(), gomock.Any(), gomock.Any(), "u1", gomock.Any()).
		DoAndReturn(func(_ context.Context, _, _, _, articleID string) error {
			runningInDB.Add(-1)
			if articleID != "straggler-1" {
				if fastCompleted.Add(1) == 4 {
					close(fast4And5Done)
				}
			}
			return nil
		}).Times(5)

	worker := service.NewSummarizeQueueWorker(jobRepo, articleRepo, apiRepo, summaryRepo, discardLogger(), 10)
	worker.SetConcurrency(3)

	doneCh := make(chan error, 1)
	go func() {
		doneCh <- worker.ProcessQueue(ctx, noRefill)
	}()

	// fast-2, fast-3, fast-4, and fast-5 must all be pulled and completed while straggler-1 is still blocked.
	<-fast4And5Done
	assert.Equal(t, int32(4), fastCompleted.Load(), "other workers must pull and complete remaining jobs while straggler is still in flight")
	select {
	case err := <-doneCh:
		t.Fatalf("ProcessQueue returned before straggler finished: %v", err)
	default:
	}

	close(releaseStraggler)
	require.NoError(t, <-doneCh)

	assert.LessOrEqual(t, maxInFlight.Load(), int32(3), "never more than concurrency jobs processing at once")
	assert.LessOrEqual(t, maxRunningInDB.Load(), int32(3), "jobs must not be held in running state waiting for a worker")
}

func TestSummarizeQueueWorker_ProcessQueue_RefillOnDrainedWhileStragglerRuns(t *testing.T) {
	ctx := context.Background()
	ctrl := gomock.NewController(t)

	jobRepo := mocks.NewMockSummarizeJobRepository(ctrl)
	articleRepo := mocks.NewMockArticleRepository(ctrl)
	apiRepo := mocks.NewMockExternalAPIRepository(ctrl)
	summaryRepo := mocks.NewMockSummaryRepository(ctrl)

	var queueMu sync.Mutex
	pending := []*domain.SummarizeJob{
		{JobID: uuid.New(), ArticleID: "straggler-1", MaxRetries: 3},
		{JobID: uuid.New(), ArticleID: "fast-2", MaxRetries: 3},
	}

	stragglerStarted := make(chan struct{})
	releaseStraggler := make(chan struct{})
	refillJobCompleted := make(chan struct{})

	jobRepo.EXPECT().RecoverStuckJobs(gomock.Any()).Return(int64(0), nil)
	jobRepo.EXPECT().DequeueJobs(gomock.Any(), 1).DoAndReturn(func(_ context.Context, _ int) ([]*domain.SummarizeJob, error) {
		queueMu.Lock()
		defer queueMu.Unlock()
		if len(pending) == 0 {
			return nil, nil
		}
		j := pending[0]
		pending = pending[1:]
		return []*domain.SummarizeJob{j}, nil
	}).AnyTimes()

	articleRepo.EXPECT().FindByID(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, id string) (*domain.Article, error) {
		return &domain.Article{ID: id, UserID: "u1", Title: "Title", Content: "Body text"}, nil
	}).Times(3)

	apiRepo.EXPECT().SummarizeArticle(gomock.Any(), gomock.Any(), "low").DoAndReturn(func(_ context.Context, a *domain.Article, _ string) (*domain.SummarizedContent, error) {
		if a.ID == "straggler-1" {
			close(stragglerStarted)
			<-releaseStraggler
		} else if a.ID == "fast-2" {
			<-stragglerStarted
		}
		return &domain.SummarizedContent{SummaryJapanese: "要約:" + a.ID}, nil
	}).Times(3)

	summaryRepo.EXPECT().Create(gomock.Any(), gomock.Any()).Return(nil).Times(3)
	jobRepo.EXPECT().CompleteJobWithNotification(gomock.Any(), gomock.Any(), gomock.Any(), "u1", gomock.Any()).
		DoAndReturn(func(_ context.Context, _, _, _, articleID string) error {
			if articleID == "refilled-3" {
				close(refillJobCompleted)
			}
			return nil
		}).Times(3)

	var refillOnce sync.Once
	onDrained := func(_ context.Context) (bool, error) {
		refilled := false
		refillOnce.Do(func() {
			queueMu.Lock()
			pending = append(pending, &domain.SummarizeJob{JobID: uuid.New(), ArticleID: "refilled-3", MaxRetries: 3})
			queueMu.Unlock()
			refilled = true
		})
		return refilled, nil
	}

	worker := service.NewSummarizeQueueWorker(jobRepo, articleRepo, apiRepo, summaryRepo, discardLogger(), 10)
	worker.SetConcurrency(2)

	doneCh := make(chan error, 1)
	go func() {
		doneCh <- worker.ProcessQueue(ctx, onDrained)
	}()

	<-refillJobCompleted
	select {
	case err := <-doneCh:
		t.Fatalf("ProcessQueue returned before straggler finished: %v", err)
	default:
	}

	close(releaseStraggler)
	require.NoError(t, <-doneCh)
}

func TestSummarizeQueueWorker_ProcessQueue_OverloadStopsAndReleasesUnstartedJob(t *testing.T) {
	ctx := context.Background()
	ctrl := gomock.NewController(t)

	jobRepo := mocks.NewMockSummarizeJobRepository(ctrl)
	articleRepo := mocks.NewMockArticleRepository(ctrl)
	apiRepo := mocks.NewMockExternalAPIRepository(ctrl)
	summaryRepo := mocks.NewMockSummaryRepository(ctrl)

	job1 := &domain.SummarizeJob{JobID: uuid.New(), ArticleID: "job-1", MaxRetries: 3}
	job2 := &domain.SummarizeJob{JobID: uuid.New(), ArticleID: "job-2", MaxRetries: 3}

	overloadedSet := make(chan struct{})
	var overloadedSetOnce sync.Once
	worker2EnteringDequeue := make(chan struct{})
	var dequeueCall atomic.Int32

	jobRepo.EXPECT().RecoverStuckJobs(gomock.Any()).Return(int64(0), nil)
	jobRepo.EXPECT().DequeueJobs(gomock.Any(), 1).DoAndReturn(func(_ context.Context, _ int) ([]*domain.SummarizeJob, error) {
		switch dequeueCall.Add(1) {
		case 1:
			return []*domain.SummarizeJob{job1}, nil
		case 2:
			close(worker2EnteringDequeue)
			<-overloadedSet
			return []*domain.SummarizeJob{job2}, nil
		default:
			t.Errorf("unexpected DequeueJobs call after overload")
			return nil, nil
		}
	}).Times(2)

	articleRepo.EXPECT().FindByID(gomock.Any(), "job-1").Return(&domain.Article{
		ID: "job-1", UserID: "u1", Title: "T1", Content: "Body",
	}, nil).Times(1)

	apiRepo.EXPECT().SummarizeArticle(gomock.Any(), gomock.Any(), "low").DoAndReturn(func(_ context.Context, _ *domain.Article, _ string) (*domain.SummarizedContent, error) {
		<-worker2EnteringDequeue
		return nil, domain.ErrUpstreamBusy
	}).Times(1)

	jobRepo.EXPECT().UpdateJobStatus(gomock.Any(), job1.JobID.String(), domain.SummarizeJobStatusFailed, "", gomock.Any()).
		Return(nil).Times(1)
	jobRepo.EXPECT().UpdateJobStatus(gomock.Any(), job2.JobID.String(), domain.SummarizeJobStatusPending, "", "").
		Return(nil).Times(1)

	log := newSignalLogger("downstream service busy, backing off queue worker", func() {
		overloadedSetOnce.Do(func() { close(overloadedSet) })
	})
	worker := service.NewSummarizeQueueWorker(jobRepo, articleRepo, apiRepo, summaryRepo, log, 10)
	worker.SetConcurrency(2)

	err := worker.ProcessQueue(ctx, noRefill)
	require.ErrorIs(t, err, domain.ErrServiceOverloaded)
}

func TestSummarizeQueueWorker_ProcessQueue_ContextCancelStopsAndReleasesUnstartedJob(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctrl := gomock.NewController(t)

	jobRepo := mocks.NewMockSummarizeJobRepository(ctrl)
	articleRepo := mocks.NewMockArticleRepository(ctrl)
	apiRepo := mocks.NewMockExternalAPIRepository(ctrl)
	summaryRepo := mocks.NewMockSummaryRepository(ctrl)

	job1 := &domain.SummarizeJob{JobID: uuid.New(), ArticleID: "job-1", MaxRetries: 3}
	job2 := &domain.SummarizeJob{JobID: uuid.New(), ArticleID: "job-2", MaxRetries: 3}

	worker2EnteringDequeue := make(chan struct{})
	ctxCanceled := make(chan struct{})
	var dequeueCall atomic.Int32

	jobRepo.EXPECT().RecoverStuckJobs(gomock.Any()).Return(int64(0), nil)
	jobRepo.EXPECT().DequeueJobs(gomock.Any(), 1).DoAndReturn(func(_ context.Context, _ int) ([]*domain.SummarizeJob, error) {
		switch dequeueCall.Add(1) {
		case 1:
			return []*domain.SummarizeJob{job1}, nil
		case 2:
			close(worker2EnteringDequeue)
			<-ctxCanceled
			return []*domain.SummarizeJob{job2}, nil
		default:
			t.Errorf("unexpected DequeueJobs call after context cancellation")
			return nil, nil
		}
	}).Times(2)

	articleRepo.EXPECT().FindByID(gomock.Any(), "job-1").Return(&domain.Article{
		ID: "job-1", UserID: "u1", Title: "T1", Content: "Body",
	}, nil).Times(1)

	apiRepo.EXPECT().SummarizeArticle(gomock.Any(), gomock.Any(), "low").DoAndReturn(func(_ context.Context, _ *domain.Article, _ string) (*domain.SummarizedContent, error) {
		<-worker2EnteringDequeue
		cancel()
		close(ctxCanceled)
		return &domain.SummarizedContent{SummaryJapanese: "要約"}, nil
	}).Times(1)

	summaryRepo.EXPECT().Create(gomock.Any(), gomock.Any()).Return(nil).Times(1)
	jobRepo.EXPECT().CompleteJobWithNotification(gomock.Any(), job1.JobID.String(), "要約", "u1", "job-1").Return(nil).Times(1)
	jobRepo.EXPECT().UpdateJobStatus(gomock.Any(), job2.JobID.String(), domain.SummarizeJobStatusPending, "", "").
		DoAndReturn(func(relCtx context.Context, _ string, _ domain.SummarizeJobStatus, _, _ string) error {
			assert.NoError(t, relCtx.Err(), "releaseJob must use a non-canceled context")
			return nil
		}).Times(1)

	worker := service.NewSummarizeQueueWorker(jobRepo, articleRepo, apiRepo, summaryRepo, discardLogger(), 10)
	worker.SetConcurrency(2)

	err := worker.ProcessQueue(ctx, noRefill)
	require.NoError(t, err)
}

func TestSummarizeQueueWorker_ProcessQueue_RefillErrorStopsWorkersAndPropagates(t *testing.T) {
	ctx := context.Background()
	ctrl := gomock.NewController(t)

	jobRepo := mocks.NewMockSummarizeJobRepository(ctrl)
	articleRepo := mocks.NewMockArticleRepository(ctrl)
	apiRepo := mocks.NewMockExternalAPIRepository(ctrl)
	summaryRepo := mocks.NewMockSummaryRepository(ctrl)

	jobRepo.EXPECT().RecoverStuckJobs(gomock.Any()).Return(int64(0), nil)
	jobRepo.EXPECT().DequeueJobs(gomock.Any(), 1).Return(nil, nil).Times(1)

	refillErr := errors.New("refill sweep failed")
	worker := service.NewSummarizeQueueWorker(jobRepo, articleRepo, apiRepo, summaryRepo, discardLogger(), 10)
	worker.SetConcurrency(1)

	err := worker.ProcessQueue(ctx, func(_ context.Context) (bool, error) {
		return false, refillErr
	})
	require.ErrorIs(t, err, refillErr)
}
