package knowledge_home_admin

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	knowledgehomev1 "alt/gen/proto/alt/knowledge_home/v1"
	"alt/usecase/knowledge_url_backfill_usecase"
)

// stubURLBackfillUsecase implements URLBackfillUsecase for handler-level tests.
type stubURLBackfillUsecase struct {
	gotMax    int
	gotDryRun bool
	calls     int
	res       *knowledge_url_backfill_usecase.EmitResult
	err       error
}

func (s *stubURLBackfillUsecase) Emit(_ context.Context, max int, dryRun bool) (*knowledge_url_backfill_usecase.EmitResult, error) {
	s.calls++
	s.gotMax = max
	s.gotDryRun = dryRun
	if s.err != nil {
		return nil, s.err
	}
	return s.res, nil
}

func newHandlerForURLBackfill(stub URLBackfillUsecase) *Handler {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := &Handler{
		urlBackfillUsecase: stub,
		logger:             logger,
	}
	return h
}

func TestEmitArticleUrlBackfill_PassesRequestThroughAndMapsResponse(t *testing.T) {
	t.Parallel()
	stub := &stubURLBackfillUsecase{
		res: &knowledge_url_backfill_usecase.EmitResult{
			ArticlesScanned:      100,
			EventsAppended:       80,
			SkippedBlockedScheme: 5,
			SkippedDuplicate:     15,
			MoreRemaining:        true,
		},
	}
	h := newHandlerForURLBackfill(stub)

	resp, err := h.EmitArticleUrlBackfill(context.Background(), connect.NewRequest(&knowledgehomev1.EmitArticleUrlBackfillRequest{
		MaxArticles: 100,
		DryRun:      false,
	}))
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, 1, stub.calls)
	assert.Equal(t, 100, stub.gotMax)
	assert.False(t, stub.gotDryRun)

	assert.Equal(t, int32(100), resp.Msg.ArticlesScanned)
	assert.Equal(t, int32(80), resp.Msg.EventsAppended)
	assert.Equal(t, int32(5), resp.Msg.SkippedBlockedScheme)
	assert.Equal(t, int32(15), resp.Msg.SkippedDuplicate)
	assert.True(t, resp.Msg.MoreRemaining)
}

func TestEmitArticleUrlBackfill_DryRunPropagates(t *testing.T) {
	t.Parallel()
	stub := &stubURLBackfillUsecase{
		res: &knowledge_url_backfill_usecase.EmitResult{},
	}
	h := newHandlerForURLBackfill(stub)

	_, err := h.EmitArticleUrlBackfill(context.Background(), connect.NewRequest(&knowledgehomev1.EmitArticleUrlBackfillRequest{
		MaxArticles: 0,
		DryRun:      true,
	}))
	require.NoError(t, err)
	assert.True(t, stub.gotDryRun, "dry_run flag must reach the usecase verbatim")
	assert.Equal(t, 0, stub.gotMax, "max_articles=0 must reach the usecase as 0 (process all)")
}

func TestEmitArticleUrlBackfill_NegativeMaxArticlesRejected(t *testing.T) {
	t.Parallel()
	stub := &stubURLBackfillUsecase{}
	h := newHandlerForURLBackfill(stub)

	_, err := h.EmitArticleUrlBackfill(context.Background(), connect.NewRequest(&knowledgehomev1.EmitArticleUrlBackfillRequest{
		MaxArticles: -1,
		DryRun:      false,
	}))
	require.Error(t, err)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeInvalidArgument, connectErr.Code())
	assert.Equal(t, 0, stub.calls, "usecase must not be invoked on invalid input")
}

func TestEmitArticleUrlBackfill_UsecaseErrorMappedToInternal(t *testing.T) {
	t.Parallel()
	stub := &stubURLBackfillUsecase{
		err: errors.New("sovereign down"),
	}
	h := newHandlerForURLBackfill(stub)

	_, err := h.EmitArticleUrlBackfill(context.Background(), connect.NewRequest(&knowledgehomev1.EmitArticleUrlBackfillRequest{}))
	require.Error(t, err)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeInternal, connectErr.Code())
}
