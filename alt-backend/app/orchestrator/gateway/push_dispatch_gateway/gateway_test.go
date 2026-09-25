package push_dispatch_gateway

import (
	"alt/shared/driver/webpush"
	"testing"
	"time"
)

func TestUrgency(t *testing.T) {
	tests := []struct {
		input    string
		expected webpush.Urgency
	}{
		{"very-low", webpush.UrgencyVeryLow},
		{"low", webpush.UrgencyLow},
		{"high", webpush.UrgencyHigh},
		{"normal", webpush.UrgencyNormal},
		{"unknown", webpush.UrgencyNormal},
		{"", webpush.UrgencyNormal},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := urgency(tt.input); got != tt.expected {
				t.Errorf("urgency(%q) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}

func TestMapSendResultToOutcome(t *testing.T) {
	res := webpush.SendResult{
		StatusCode:  200,
		Gone:        false,
		Retryable:   false,
		RetryAfter:  10 * time.Second,
		BodyExcerpt: "ok",
	}

	outcome := mapSendResultToOutcome(res)
	if outcome.StatusCode != 200 || outcome.Gone || outcome.Retryable || outcome.RetryAfter != 10*time.Second || outcome.BodyExcerpt != "ok" {
		t.Errorf("unexpected outcome: %+v", outcome)
	}
}
