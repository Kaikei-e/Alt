package usecase

import (
	"context"
	"errors"
	"fmt"

	"mq-hub/domain"
	"mq-hub/port"
)

// TrimReport summarises one maintenance pass.
type TrimReport struct {
	Deleted   int64
	PerStream map[domain.StreamKey]int64
}

// TrimStreamsUsecase enforces an absolute ceiling on every known stream.
//
// Publish-time trimming (XADD's own MAXLEN) is the primary retention control,
// but it only runs as part of a successful XADD — and XADD is exactly what
// Redis rejects once the instance is at maxmemory under noeviction. That makes
// the normal path self-locking: no publish, no trim, so the stream can never
// shrink back under the limit on its own. XTRIM is not denyoom, so this pass
// keeps working precisely when publishing is locked out, which is the only
// thing that can release the latch without an operator.
//
// The cap is deliberately an absolute one that ignores consumer references. It
// should sit well above the publish-time target so that it only ever fires when
// something is already wrong; when it does fire, that is worth alerting on.
type TrimStreamsUsecase struct {
	trimmer    port.StreamTrimmer
	hardMaxLen int64
}

// NewTrimStreamsUsecase creates the maintenance usecase.
func NewTrimStreamsUsecase(trimmer port.StreamTrimmer, hardMaxLen int64) *TrimStreamsUsecase {
	return &TrimStreamsUsecase{trimmer: trimmer, hardMaxLen: hardMaxLen}
}

// Execute trims every known stream, continuing past individual failures so one
// unreachable stream cannot leave the rest unbounded. The returned report covers
// whatever did succeed even when err is non-nil.
func (u *TrimStreamsUsecase) Execute(ctx context.Context) (TrimReport, error) {
	report := TrimReport{PerStream: make(map[domain.StreamKey]int64)}

	if u.hardMaxLen <= 0 {
		return report, fmt.Errorf("hard max length must be positive, got %d", u.hardMaxLen)
	}

	var failures []error
	for _, stream := range domain.AllStreamKeys() {
		deleted, err := u.trimmer.TrimMaxLenApprox(ctx, stream, u.hardMaxLen)
		if err != nil {
			failures = append(failures, fmt.Errorf("trim %s: %w", stream, err))
			continue
		}
		if deleted > 0 {
			report.PerStream[stream] = deleted
			report.Deleted += deleted
		}
	}

	return report, errors.Join(failures...)
}
