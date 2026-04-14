package consumer

import (
	"testing"
)

func TestDefaultConfig_HasDLQDefaults(t *testing.T) {
	t.Parallel()
	cfg := DefaultConfig()

	if cfg.DLQStreamKey == "" {
		t.Fatal("DLQStreamKey must default to a non-empty stream name")
	}
	if cfg.DLQStreamKey == cfg.StreamKey {
		t.Fatalf("DLQ must be separate from main stream: %q", cfg.DLQStreamKey)
	}
	if cfg.MaxDeliveries <= 0 {
		t.Fatalf("MaxDeliveries must be positive, got %d", cfg.MaxDeliveries)
	}
}

func TestShouldSendToDLQ(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		delivery int64
		max      int64
		want     bool
	}{
		{"first attempt", 1, 3, false},
		{"third attempt equals limit", 3, 3, false},
		{"over limit", 4, 3, true},
		{"way over limit", 99, 3, true},
		{"zero max disables", 1, 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := shouldSendToDLQ(tc.delivery, tc.max); got != tc.want {
				t.Fatalf("shouldSendToDLQ(%d,%d) = %v, want %v", tc.delivery, tc.max, got, tc.want)
			}
		})
	}
}
