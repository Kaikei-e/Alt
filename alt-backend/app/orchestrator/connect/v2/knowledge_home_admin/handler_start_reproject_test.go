package knowledge_home_admin

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"alt/domain"
	knowledgehomev1 "alt/gen/proto/alt/knowledge_home/v1"
	"alt/orchestrator/usecase/knowledge_reproject_usecase"
)

type stubReprojectUsecase struct {
	run *domain.ReprojectRun
	err error
}

func (s *stubReprojectUsecase) StartReproject(_ context.Context, _, _, _ string, _, _ *time.Time) (*domain.ReprojectRun, error) {
	return s.run, s.err
}

func (s *stubReprojectUsecase) GetReprojectStatus(_ context.Context, _ uuid.UUID) (*domain.ReprojectRun, error) {
	return s.run, s.err
}

func (s *stubReprojectUsecase) ListReprojectRuns(_ context.Context, _ string, _ int) ([]domain.ReprojectRun, error) {
	return nil, s.err
}

func (s *stubReprojectUsecase) CompareReproject(_ context.Context, _ uuid.UUID) (*domain.ReprojectDiffSummary, error) {
	return nil, s.err
}

func (s *stubReprojectUsecase) SwapReproject(_ context.Context, _ uuid.UUID) error { return s.err }

func (s *stubReprojectUsecase) RollbackReproject(_ context.Context, _ uuid.UUID) error { return s.err }

func newHandlerForReproject(stub ReprojectUsecase) *Handler {
	return &Handler{
		reprojectUsecase: stub,
		logger:           slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
}

// "No executor is wired" is a statement about the system, not about the
// request: the operator sent a well-formed one and there is simply nothing to
// carry it out. Internal would read as a transient server fault and invite a
// retry that can never succeed, and the UI's error surface would say nothing
// useful. FailedPrecondition is the code that means "fix the system state
// first", and the message has to name what is missing.
func TestStartReproject_MissingExecutorIsFailedPrecondition(t *testing.T) {
	stub := &stubReprojectUsecase{err: knowledge_reproject_usecase.ErrNoReprojectExecutor}
	h := newHandlerForReproject(stub)

	_, err := h.StartReproject(context.Background(), connect.NewRequest(&knowledgehomev1.StartReprojectRequest{
		Mode:        "dry_run",
		FromVersion: "v7",
		ToVersion:   "v8",
	}))

	require.Error(t, err)
	assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err))
	assert.Contains(t, err.Error(), "no reproject executor is wired")
}

// Everything else the usecase can fail with is still an internal fault.
func TestStartReproject_OtherErrorsStayInternal(t *testing.T) {
	stub := &stubReprojectUsecase{err: errors.New("boom")}
	h := newHandlerForReproject(stub)

	_, err := h.StartReproject(context.Background(), connect.NewRequest(&knowledgehomev1.StartReprojectRequest{
		Mode:        "full",
		FromVersion: "v7",
		ToVersion:   "v8",
	}))

	require.Error(t, err)
	assert.Equal(t, connect.CodeInternal, connect.CodeOf(err))
}
