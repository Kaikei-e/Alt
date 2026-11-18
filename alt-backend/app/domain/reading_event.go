package domain

import (
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/google/uuid"
)

const MaxReadingEventsBatchSize = 50

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
