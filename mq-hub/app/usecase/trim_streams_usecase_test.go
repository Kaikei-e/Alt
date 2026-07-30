package usecase

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"mq-hub/domain"
)

// stubTrimmer records the trim requests it receives and replays a scripted
// result per stream.
type stubTrimmer struct {
	calls   []domain.StreamKey
	maxLens []int64
	deleted map[domain.StreamKey]int64
	failOn  domain.StreamKey
}

func (s *stubTrimmer) TrimMaxLenApprox(_ context.Context, stream domain.StreamKey, maxLen int64) (int64, error) {
	s.calls = append(s.calls, stream)
	s.maxLens = append(s.maxLens, maxLen)
	if stream == s.failOn {
		return 0, errors.New("redis unavailable")
	}
	return s.deleted[stream], nil
}

func TestTrimStreamsUsecase(t *testing.T) {
	const hardMaxLen = int64(50000)

	t.Run("covers every known stream with the configured cap", func(t *testing.T) {
		trimmer := &stubTrimmer{deleted: map[domain.StreamKey]int64{}}

		report, err := NewTrimStreamsUsecase(trimmer, hardMaxLen).Execute(context.Background())

		require.NoError(t, err)
		assert.Equal(t, domain.AllStreamKeys(), trimmer.calls)
		for _, maxLen := range trimmer.maxLens {
			assert.Equal(t, hardMaxLen, maxLen)
		}
		assert.Zero(t, report.Deleted, "nothing was over the cap")
	})

	// The backstop firing at all means the XADD-time trim failed to keep up,
	// which is the signal an operator needs. It must be reported, not swallowed.
	t.Run("reports what it deleted", func(t *testing.T) {
		trimmer := &stubTrimmer{deleted: map[domain.StreamKey]int64{
			domain.StreamKeyArticles: 12,
			domain.StreamKeyTags:     3,
		}}

		report, err := NewTrimStreamsUsecase(trimmer, hardMaxLen).Execute(context.Background())

		require.NoError(t, err)
		assert.Equal(t, int64(15), report.Deleted)
		assert.Equal(t, int64(12), report.PerStream[domain.StreamKeyArticles])
	})

	t.Run("one unreachable stream does not leave the others unbounded", func(t *testing.T) {
		trimmer := &stubTrimmer{
			deleted: map[domain.StreamKey]int64{domain.StreamKeyTags: 7},
			failOn:  domain.StreamKeyArticles,
		}

		report, err := NewTrimStreamsUsecase(trimmer, hardMaxLen).Execute(context.Background())

		require.Error(t, err, "the failure must be surfaced")
		assert.Len(t, trimmer.calls, len(domain.AllStreamKeys()))
		assert.Equal(t, int64(7), report.Deleted, "streams that succeeded still count")
	})

	t.Run("refuses a cap that would empty every stream", func(t *testing.T) {
		_, err := NewTrimStreamsUsecase(&stubTrimmer{}, 0).Execute(context.Background())

		require.Error(t, err)
	})
}
