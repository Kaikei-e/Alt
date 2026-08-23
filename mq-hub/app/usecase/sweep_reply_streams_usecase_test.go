package usecase

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"mq-hub/domain"
)

// MockReplyStreamSweeper is a mock implementation of port.ReplyStreamSweeper.
type MockReplyStreamSweeper struct {
	mock.Mock
}

func (m *MockReplyStreamSweeper) ScanReplyStreamsWithoutTTL(ctx context.Context, prefix string) ([]domain.StreamKey, error) {
	args := m.Called(ctx, prefix)
	if v := args.Get(0); v != nil {
		return v.([]domain.StreamKey), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *MockReplyStreamSweeper) Expire(ctx context.Context, stream domain.StreamKey, ttl time.Duration) error {
	args := m.Called(ctx, stream, ttl)
	return args.Error(0)
}

func TestSweepReplyStreamsUsecase_Execute(t *testing.T) {
	t.Run("applies the safety-net TTL to every untracked reply stream", func(t *testing.T) {
		sweeper := new(MockReplyStreamSweeper)
		uc := NewSweepReplyStreamsUsecase(sweeper)

		ctx := context.Background()
		leaked := []domain.StreamKey{
			domain.StreamKey(ReplyStreamPrefix + "corr-1"),
			domain.StreamKey(ReplyStreamPrefix + "corr-2"),
		}

		sweeper.On("ScanReplyStreamsWithoutTTL", ctx, ReplyStreamPrefix).Return(leaked, nil)
		sweeper.On("Expire", ctx, leaked[0], replyStreamTTL).Return(nil)
		sweeper.On("Expire", ctx, leaked[1], replyStreamTTL).Return(nil)

		report, err := uc.Execute(ctx)

		require.NoError(t, err)
		assert.Equal(t, 2, report.Bounded)
		sweeper.AssertExpectations(t)
	})

	t.Run("does nothing when there are no untracked reply streams", func(t *testing.T) {
		sweeper := new(MockReplyStreamSweeper)
		uc := NewSweepReplyStreamsUsecase(sweeper)

		ctx := context.Background()
		sweeper.On("ScanReplyStreamsWithoutTTL", ctx, ReplyStreamPrefix).Return([]domain.StreamKey{}, nil)

		report, err := uc.Execute(ctx)

		require.NoError(t, err)
		assert.Zero(t, report.Bounded)
		sweeper.AssertNotCalled(t, "Expire", mock.Anything, mock.Anything, mock.Anything)
	})

	t.Run("returns the scan error", func(t *testing.T) {
		sweeper := new(MockReplyStreamSweeper)
		uc := NewSweepReplyStreamsUsecase(sweeper)

		ctx := context.Background()
		sweeper.On("ScanReplyStreamsWithoutTTL", ctx, ReplyStreamPrefix).
			Return(nil, errors.New("redis down"))

		report, err := uc.Execute(ctx)

		require.Error(t, err)
		assert.Zero(t, report.Bounded)
	})

	t.Run("continues past a single Expire failure and reports it", func(t *testing.T) {
		sweeper := new(MockReplyStreamSweeper)
		uc := NewSweepReplyStreamsUsecase(sweeper)

		ctx := context.Background()
		leaked := []domain.StreamKey{
			domain.StreamKey(ReplyStreamPrefix + "corr-1"),
			domain.StreamKey(ReplyStreamPrefix + "corr-2"),
		}
		sweeper.On("ScanReplyStreamsWithoutTTL", ctx, ReplyStreamPrefix).Return(leaked, nil)
		sweeper.On("Expire", ctx, leaked[0], replyStreamTTL).Return(errors.New("expire failed"))
		sweeper.On("Expire", ctx, leaked[1], replyStreamTTL).Return(nil)

		report, err := uc.Execute(ctx)

		require.Error(t, err)
		// The second key must still have been bounded despite the first failing.
		assert.Equal(t, 1, report.Bounded)
		sweeper.AssertExpectations(t)
	})
}
