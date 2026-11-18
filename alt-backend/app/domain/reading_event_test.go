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

func TestReadingEventInput_OpenedValidation(t *testing.T) {
	validEventID := "01960000-0000-7000-8000-000000000001"
	validArticleID := "11111111-1111-4111-8111-111111111111"
	validTime := time.Date(2026, 11, 18, 0, 0, 0, 0, time.UTC)

	require.NoError(t, ReadingEventInput{
		ClientEventID: validEventID,
		OccurredAt:    validTime,
		Payload:       OpenedInput{ArticleID: validArticleID, Surface: ReadingSurfaceGallery},
	}.Validate())

	err := ReadingEventInput{
		ClientEventID: validEventID,
		OccurredAt:    validTime,
		Payload:       OpenedInput{ArticleID: "bad-uuid", Surface: ReadingSurfaceGallery},
	}.Validate()
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidReadingEvent))

	err = ReadingEventInput{
		ClientEventID: validEventID,
		OccurredAt:    validTime,
		Payload:       OpenedInput{ArticleID: validArticleID, Surface: ReadingSurface("")},
	}.Validate()
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidReadingEvent))
}

func TestEvaluateReadRuleV1(t *testing.T) {
	tests := []struct {
		name            string
		reachedEnd      bool
		activeDwellMs   int64
		bodyLengthChars int32
		want            bool
	}{
		{
			name:            "reached end at 5000ms boundary counts as read",
			reachedEnd:      true,
			activeDwellMs:   5000,
			bodyLengthChars: 0,
			want:            true,
		},
		{
			name:            "reached end under 5000ms does not count as read",
			reachedEnd:      true,
			activeDwellMs:   4999,
			bodyLengthChars: 100,
			want:            false,
		},
		{
			name:            "short body meets 30000ms floor without reaching end",
			reachedEnd:      false,
			activeDwellMs:   30000,
			bodyLengthChars: 500,
			want:            true,
		},
		{
			name:            "short body below 30000ms floor without reaching end is not read",
			reachedEnd:      false,
			activeDwellMs:   29999,
			bodyLengthChars: 500,
			want:            false,
		},
		{
			name:            "long body meets scaled 30ms per char threshold",
			reachedEnd:      false,
			activeDwellMs:   36000,
			bodyLengthChars: 1200,
			want:            true,
		},
		{
			name:            "long body below scaled 30ms per char threshold is not read",
			reachedEnd:      false,
			activeDwellMs:   35999,
			bodyLengthChars: 1200,
			want:            false,
		},
		{
			name:            "zero body length does not satisfy dwell branch even with high dwell",
			reachedEnd:      false,
			activeDwellMs:   120000,
			bodyLengthChars: 0,
			want:            false,
		},
		{
			name:            "negative body length does not satisfy dwell branch",
			reachedEnd:      false,
			activeDwellMs:   120000,
			bodyLengthChars: -10,
			want:            false,
		},
		{
			name:            "reached end with 5000ms counts even on long body",
			reachedEnd:      true,
			activeDwellMs:   5000,
			bodyLengthChars: 10000,
			want:            true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EvaluateReadRuleV1(tt.reachedEnd, tt.activeDwellMs, tt.bodyLengthChars)
			assert.Equal(t, tt.want, got)
		})
	}
}
