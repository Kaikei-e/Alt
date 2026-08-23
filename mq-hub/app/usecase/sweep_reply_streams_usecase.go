package usecase

import (
	"context"
	"errors"
	"fmt"

	"mq-hub/port"
)

// SweepReport summarises one reply-stream safety-net pass.
type SweepReport struct {
	// Bounded is the number of untracked reply streams that were given a TTL.
	Bounded int
}

// SweepReplyStreamsUsecase re-applies a bounded TTL to temporary request-reply
// streams that lost theirs.
//
// GenerateTagsForArticle deletes its reply stream (ReplyStreamPrefix +
// correlationID) on completion or timeout, but a worker that replies LATE can
// XADD-recreate the key afterwards with no expiry. The length-cap trim pass
// (TrimStreamsUsecase) only enforces a ceiling on the fixed set in
// domain.AllStreamKeys(), so it never touches these per-correlation keys; without
// this sweep such a key lives forever. The sweep is non-destructive: it only
// applies replyStreamTTL to keys that currently have no expiry, and that TTL
// (5m) sits well above maxTagGenerationTimeoutMs, so it can never truncate a
// reply an in-flight request is still waiting on.
type SweepReplyStreamsUsecase struct {
	sweeper port.ReplyStreamSweeper
}

// NewSweepReplyStreamsUsecase creates the reply-stream safety-net usecase.
func NewSweepReplyStreamsUsecase(sweeper port.ReplyStreamSweeper) *SweepReplyStreamsUsecase {
	return &SweepReplyStreamsUsecase{sweeper: sweeper}
}

// Execute scans for reply streams without a TTL and applies replyStreamTTL to
// each, continuing past individual failures so one unreachable key cannot leave
// the rest unbounded. The returned report covers whatever did succeed even when
// err is non-nil.
func (u *SweepReplyStreamsUsecase) Execute(ctx context.Context) (SweepReport, error) {
	report := SweepReport{}

	leaked, err := u.sweeper.ScanReplyStreamsWithoutTTL(ctx, ReplyStreamPrefix)
	if err != nil {
		return report, fmt.Errorf("scan reply streams: %w", err)
	}

	var failures []error
	for _, key := range leaked {
		if err := u.sweeper.Expire(ctx, key, replyStreamTTL); err != nil {
			failures = append(failures, fmt.Errorf("expire %s: %w", key, err))
			continue
		}
		report.Bounded++
	}

	return report, errors.Join(failures...)
}
