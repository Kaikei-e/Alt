package bootstrap

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"pre-processor/test/mocks"
	"pre-processor/utils/otel"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// TestResolveOtelShutdown_NilFallsBackToNoOp reproduces the shutdown panic:
// otel.InitProvider returns (nil, err) when initialization fails, and the
// deferred shutdown in Run previously called that nil func unconditionally.
func TestResolveOtelShutdown_NilFallsBackToNoOp(t *testing.T) {
	got := resolveOtelShutdown(nil)
	if got == nil {
		t.Fatal("resolveOtelShutdown(nil) returned nil, want a callable no-op")
	}

	if err := got(context.Background()); err != nil {
		t.Fatalf("no-op shutdown returned error: %v", err)
	}
}

func TestResolveOtelShutdown_PassesThroughNonNil(t *testing.T) {
	wantErr := errors.New("boom")
	called := false
	fn := otel.ShutdownFunc(func(context.Context) error {
		called = true
		return wantErr
	})

	got := resolveOtelShutdown(fn)
	if err := got(context.Background()); !errors.Is(err, wantErr) {
		t.Fatalf("got error %v, want %v", err, wantErr)
	}
	if !called {
		t.Fatal("resolveOtelShutdown replaced a non-nil ShutdownFunc instead of passing it through")
	}
}

func TestStartJobs_SkipsSyncAndBackfillWhenArticleSyncDisabled(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	jobHandler := mocks.NewMockJobHandler(ctrl)
	healthHandler := mocks.NewMockHealthHandler(ctrl)

	// StartArticleSyncJob and StartBackfillJob must NOT be called when ArticleSyncEnabled is false.
	jobHandler.EXPECT().StartSummarizationJob(gomock.Any()).Return(nil)
	jobHandler.EXPECT().StartQualityCheckJob(gomock.Any()).Return(nil)
	jobHandler.EXPECT().StartSummarizeQueueWorker(gomock.Any()).Return(nil)
	jobHandler.EXPECT().StartNotificationRelayJob(gomock.Any()).Return(nil)
	healthHandler.EXPECT().CheckDependencies(gomock.Any()).Return(nil)

	deps := &Dependencies{
		JobHandler:         jobHandler,
		HealthHandler:      healthHandler,
		ArticleSyncEnabled: false,
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	require.NoError(t, startJobs(context.Background(), deps, log))
}

func TestStartJobs_StartsSyncAndBackfillWhenArticleSyncEnabled(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	jobHandler := mocks.NewMockJobHandler(ctrl)
	healthHandler := mocks.NewMockHealthHandler(ctrl)

	jobHandler.EXPECT().StartArticleSyncJob(gomock.Any()).Return(nil)
	jobHandler.EXPECT().StartBackfillJob(gomock.Any()).Return(nil)
	jobHandler.EXPECT().StartSummarizationJob(gomock.Any()).Return(nil)
	jobHandler.EXPECT().StartQualityCheckJob(gomock.Any()).Return(nil)
	jobHandler.EXPECT().StartSummarizeQueueWorker(gomock.Any()).Return(nil)
	jobHandler.EXPECT().StartNotificationRelayJob(gomock.Any()).Return(nil)
	healthHandler.EXPECT().CheckDependencies(gomock.Any()).Return(nil)

	deps := &Dependencies{
		JobHandler:         jobHandler,
		HealthHandler:      healthHandler,
		ArticleSyncEnabled: true,
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	require.NoError(t, startJobs(context.Background(), deps, log))
}
