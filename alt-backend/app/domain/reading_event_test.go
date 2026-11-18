package domain

import (
	"errors"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseReadingSurface(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    ReadingSurface
		wantErr bool
	}{
		{name: "gallery", raw: "gallery", want: ReadingSurfaceGallery},
		{name: "swipe", raw: "swipe", want: ReadingSurfaceSwipe},
		{name: "article_page", raw: "article_page", want: ReadingSurfaceArticlePage},
		{name: "other", raw: "other", want: ReadingSurfaceOther},
		{name: "empty", raw: "", wantErr: true},
		{name: "unknown", raw: "home", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseReadingSurface(tt.raw)
			if tt.wantErr {
				require.Error(t, err)
				assert.False(t, got.IsValid())
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
			assert.True(t, got.IsValid())
			assert.Equal(t, tt.raw, got.String())
		})
	}
}

func TestReadingEventInput_Validate(t *testing.T) {
	validEventID := "01960000-0000-7000-8000-000000000001"
	validArticleID := "11111111-1111-4111-8111-111111111111"
	validTime := time.Date(2026, 11, 18, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name    string
		input   ReadingEventInput
		wantErr bool
	}{
		{
			name: "valid surface_viewed",
			input: ReadingEventInput{
				ClientEventID: validEventID,
				OccurredAt:    validTime,
				Payload:       SurfaceViewedInput{Surface: ReadingSurfaceGallery},
			},
		},
		{
			name: "valid surface_viewed other",
			input: ReadingEventInput{
				ClientEventID: validEventID,
				OccurredAt:    validTime,
				Payload:       SurfaceViewedInput{Surface: ReadingSurfaceOther},
			},
		},
		{
			name: "valid reached_end",
			input: ReadingEventInput{
				ClientEventID: validEventID,
				OccurredAt:    validTime,
				Payload: ReachedEndInput{
					ArticleID: validArticleID,
					Surface:   ReadingSurfaceSwipe,
				},
			},
		},
		{
			name: "valid session_ended at boundary zeros and one",
			input: ReadingEventInput{
				ClientEventID: validEventID,
				OccurredAt:    validTime,
				Payload: SessionEndedInput{
					ArticleID:       validArticleID,
					Surface:         ReadingSurfaceArticlePage,
					ActiveDwellMs:   0,
					MaxScrollRatio:  1,
					BodyLengthChars: 0,
					ReachedEnd:      true,
				},
			},
		},
		{
			name: "invalid client_event_id",
			input: ReadingEventInput{
				ClientEventID: "not-a-uuid",
				OccurredAt:    validTime,
				Payload:       SurfaceViewedInput{Surface: ReadingSurfaceGallery},
			},
			wantErr: true,
		},
		{
			name: "missing occurred_at",
			input: ReadingEventInput{
				ClientEventID: validEventID,
				Payload:       SurfaceViewedInput{Surface: ReadingSurfaceGallery},
			},
			wantErr: true,
		},
		{
			name: "nil payload",
			input: ReadingEventInput{
				ClientEventID: validEventID,
				OccurredAt:    validTime,
			},
			wantErr: true,
		},
		{
			name: "invalid surface on surface_viewed",
			input: ReadingEventInput{
				ClientEventID: validEventID,
				OccurredAt:    validTime,
				Payload:       SurfaceViewedInput{Surface: ""},
			},
			wantErr: true,
		},
		{
			name: "invalid article_id on reached_end",
			input: ReadingEventInput{
				ClientEventID: validEventID,
				OccurredAt:    validTime,
				Payload: ReachedEndInput{
					ArticleID: "bad",
					Surface:   ReadingSurfaceGallery,
				},
			},
			wantErr: true,
		},
		{
			name: "invalid surface on reached_end",
			input: ReadingEventInput{
				ClientEventID: validEventID,
				OccurredAt:    validTime,
				Payload: ReachedEndInput{
					ArticleID: validArticleID,
					Surface:   ReadingSurface("unknown"),
				},
			},
			wantErr: true,
		},
		{
			name: "negative active_dwell_ms",
			input: ReadingEventInput{
				ClientEventID: validEventID,
				OccurredAt:    validTime,
				Payload: SessionEndedInput{
					ArticleID:       validArticleID,
					Surface:         ReadingSurfaceGallery,
					ActiveDwellMs:   -1,
					MaxScrollRatio:  0.5,
					BodyLengthChars: 100,
				},
			},
			wantErr: true,
		},
		{
			name: "max_scroll_ratio below zero",
			input: ReadingEventInput{
				ClientEventID: validEventID,
				OccurredAt:    validTime,
				Payload: SessionEndedInput{
					ArticleID:       validArticleID,
					Surface:         ReadingSurfaceGallery,
					ActiveDwellMs:   100,
					MaxScrollRatio:  -0.01,
					BodyLengthChars: 100,
				},
			},
			wantErr: true,
		},
		{
			name: "max_scroll_ratio above one",
			input: ReadingEventInput{
				ClientEventID: validEventID,
				OccurredAt:    validTime,
				Payload: SessionEndedInput{
					ArticleID:       validArticleID,
					Surface:         ReadingSurfaceGallery,
					ActiveDwellMs:   100,
					MaxScrollRatio:  1.01,
					BodyLengthChars: 100,
				},
			},
			wantErr: true,
		},
		{
			name: "max_scroll_ratio NaN",
			input: ReadingEventInput{
				ClientEventID: validEventID,
				OccurredAt:    validTime,
				Payload: SessionEndedInput{
					ArticleID:       validArticleID,
					Surface:         ReadingSurfaceGallery,
					ActiveDwellMs:   100,
					MaxScrollRatio:  math.NaN(),
					BodyLengthChars: 100,
				},
			},
			wantErr: true,
		},
		{
			name: "max_scroll_ratio +Inf",
			input: ReadingEventInput{
				ClientEventID: validEventID,
				OccurredAt:    validTime,
				Payload: SessionEndedInput{
					ArticleID:       validArticleID,
					Surface:         ReadingSurfaceGallery,
					ActiveDwellMs:   100,
					MaxScrollRatio:  math.Inf(1),
					BodyLengthChars: 100,
				},
			},
			wantErr: true,
		},
		{
			name: "negative body_length_chars",
			input: ReadingEventInput{
				ClientEventID: validEventID,
				OccurredAt:    validTime,
				Payload: SessionEndedInput{
					ArticleID:       validArticleID,
					Surface:         ReadingSurfaceGallery,
					ActiveDwellMs:   100,
					MaxScrollRatio:  0.5,
					BodyLengthChars: -1,
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.input.Validate()
			if tt.wantErr {
				require.Error(t, err)
				assert.True(t, errors.Is(err, ErrInvalidReadingEvent))
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestValidateReadingEventBatch(t *testing.T) {
	validEvent := ReadingEventInput{
		ClientEventID: "01960000-0000-7000-8000-000000000001",
		OccurredAt:    time.Date(2026, 11, 18, 0, 0, 0, 0, time.UTC),
		Payload:       SurfaceViewedInput{Surface: ReadingSurfaceGallery},
	}

	t.Run("empty batch is valid", func(t *testing.T) {
		require.NoError(t, ValidateReadingEventBatch(nil))
		require.NoError(t, ValidateReadingEventBatch([]ReadingEventInput{}))
	})

	t.Run("batch of 50 is valid", func(t *testing.T) {
		batch := make([]ReadingEventInput, MaxReadingEventsBatchSize)
		for i := range batch {
			batch[i] = validEvent
		}
		require.NoError(t, ValidateReadingEventBatch(batch))
	})

	t.Run("batch of 51 is rejected", func(t *testing.T) {
		batch := make([]ReadingEventInput, MaxReadingEventsBatchSize+1)
		for i := range batch {
			batch[i] = validEvent
		}
		err := ValidateReadingEventBatch(batch)
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrInvalidReadingEvent))
	})
}
