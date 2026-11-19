package domain

import (
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/google/uuid"
)

const (
	MaxReadingEventsBatchSize = 50
	MaxReadingClockSkew       = 24 * time.Hour

	ReadRuleVersionV1              = 1
	ReadRuleV1MinReachedEndDwellMs = int64(5000)
	ReadRuleV1MinDwellMs           = int64(30000)
	ReadRuleV1MsPerChar            = int64(30)
)

// ClampReadingOccurredAt guards monthly event partitioning against wildly
// skewed device clocks by clamping occurred_at to receivedAt when |skew| > 24h.
func ClampReadingOccurredAt(clientOccurredAt, receivedAt time.Time) (time.Time, string) {
	utcClient := clientOccurredAt.UTC()
	utcReceived := receivedAt.UTC()
	skew := utcReceived.Sub(utcClient)
	if skew < 0 {
		skew = -skew
	}
	if skew > MaxReadingClockSkew {
		return utcReceived, utcClient.Format(time.RFC3339)
	}
	return utcClient, ""
}

// EvaluateReadRuleV1 determines whether a reading session counts as read under
// read_rule_version = 1.
func EvaluateReadRuleV1(reachedEnd bool, activeDwellMs int64, bodyLengthChars int32) bool {
	if reachedEnd && activeDwellMs >= ReadRuleV1MinReachedEndDwellMs {
		return true
	}
	if bodyLengthChars > 0 {
		threshold := int64(bodyLengthChars) * ReadRuleV1MsPerChar
		if threshold < ReadRuleV1MinDwellMs {
			threshold = ReadRuleV1MinDwellMs
		}
		if activeDwellMs >= threshold {
			return true
		}
	}
	return false
}

var ErrInvalidReadingEvent = errors.New("invalid reading event")

type ReadingSurface string

const (
	ReadingSurfaceGallery     ReadingSurface = "gallery"
	ReadingSurfaceSwipe       ReadingSurface = "swipe"
	ReadingSurfaceArticlePage ReadingSurface = "article_page"
	ReadingSurfaceOther       ReadingSurface = "other"
)

func ParseReadingSurface(raw string) (ReadingSurface, error) {
	switch ReadingSurface(raw) {
	case ReadingSurfaceGallery, ReadingSurfaceSwipe, ReadingSurfaceArticlePage, ReadingSurfaceOther:
		return ReadingSurface(raw), nil
	default:
		return "", fmt.Errorf("%w: invalid surface %q", ErrInvalidReadingEvent, raw)
	}
}

func (s ReadingSurface) IsValid() bool {
	switch s {
	case ReadingSurfaceGallery, ReadingSurfaceSwipe, ReadingSurfaceArticlePage, ReadingSurfaceOther:
		return true
	default:
		return false
	}
}

func (s ReadingSurface) String() string {
	return string(s)
}

type ReadingEventPayload interface {
	isReadingEventPayload()
}

type SurfaceViewedInput struct {
	Surface ReadingSurface
}

func (SurfaceViewedInput) isReadingEventPayload() {}

type ReachedEndInput struct {
	ArticleID string
	Surface   ReadingSurface
}

func (ReachedEndInput) isReadingEventPayload() {}

type SessionEndedInput struct {
	ArticleID       string
	Surface         ReadingSurface
	ActiveDwellMs   int64
	MaxScrollRatio  float64
	BodyLengthChars int32
	ReachedEnd      bool
}

func (SessionEndedInput) isReadingEventPayload() {}

type OpenedInput struct {
	ArticleID string
	Surface   ReadingSurface
}

func (OpenedInput) isReadingEventPayload() {}

type ReadingEventInput struct {
	ClientEventID string
	OccurredAt    time.Time
	Payload       ReadingEventPayload
}

func (e ReadingEventInput) Validate() error {
	if _, err := uuid.Parse(e.ClientEventID); err != nil {
		return fmt.Errorf("%w: invalid client_event_id", ErrInvalidReadingEvent)
	}
	if e.OccurredAt.IsZero() {
		return fmt.Errorf("%w: occurred_at is required", ErrInvalidReadingEvent)
	}
	if e.Payload == nil {
		return fmt.Errorf("%w: event payload is required", ErrInvalidReadingEvent)
	}

	switch p := e.Payload.(type) {
	case SurfaceViewedInput:
		if !p.Surface.IsValid() {
			return fmt.Errorf("%w: invalid surface", ErrInvalidReadingEvent)
		}
	case ReachedEndInput:
		if !p.Surface.IsValid() {
			return fmt.Errorf("%w: invalid surface", ErrInvalidReadingEvent)
		}
		if _, err := uuid.Parse(p.ArticleID); err != nil {
			return fmt.Errorf("%w: invalid article_id", ErrInvalidReadingEvent)
		}
	case OpenedInput:
		if !p.Surface.IsValid() {
			return fmt.Errorf("%w: invalid surface", ErrInvalidReadingEvent)
		}
		if _, err := uuid.Parse(p.ArticleID); err != nil {
			return fmt.Errorf("%w: invalid article_id", ErrInvalidReadingEvent)
		}
	case SessionEndedInput:
		if !p.Surface.IsValid() {
			return fmt.Errorf("%w: invalid surface", ErrInvalidReadingEvent)
		}
		if _, err := uuid.Parse(p.ArticleID); err != nil {
			return fmt.Errorf("%w: invalid article_id", ErrInvalidReadingEvent)
		}
		if p.ActiveDwellMs < 0 {
			return fmt.Errorf("%w: active_dwell_ms must be non-negative", ErrInvalidReadingEvent)
		}
		if math.IsNaN(p.MaxScrollRatio) || math.IsInf(p.MaxScrollRatio, 0) || p.MaxScrollRatio < 0 || p.MaxScrollRatio > 1 {
			return fmt.Errorf("%w: max_scroll_ratio must be between 0 and 1", ErrInvalidReadingEvent)
		}
		if p.BodyLengthChars < 0 {
			return fmt.Errorf("%w: body_length_chars must be non-negative", ErrInvalidReadingEvent)
		}
	default:
		return fmt.Errorf("%w: unknown event payload", ErrInvalidReadingEvent)
	}
	return nil
}

func ValidateReadingEventBatch(events []ReadingEventInput) error {
	if len(events) > MaxReadingEventsBatchSize {
		return fmt.Errorf("%w: batch exceeds maximum of %d events", ErrInvalidReadingEvent, MaxReadingEventsBatchSize)
	}
	for i, ev := range events {
		if err := ev.Validate(); err != nil {
			return fmt.Errorf("event[%d]: %w", i, err)
		}
	}
	return nil
}
