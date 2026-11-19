package reading_projector

import "testing"

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
			if got != tt.want {
				t.Errorf("EvaluateReadRuleV1(%v, %d, %d) = %v, want %v",
					tt.reachedEnd, tt.activeDwellMs, tt.bodyLengthChars, got, tt.want)
			}
		})
	}
}
