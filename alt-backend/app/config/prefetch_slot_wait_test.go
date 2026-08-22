package config

import (
	"testing"
	"time"
)

// The prefetch budget has three states and every one of them has to be a value
// somebody wrote down. Unset falls back to the declared default (enabled, with
// a short budget); "off" is the explicit disable; zero is refused, because for
// this class it would mean "queue for this host like a background job" — the
// priority inversion the third fetch class exists to avoid. Rules 8 and 9: no
// state is reachable by silence, and none is inferred from an unset variable.
func TestPrefetchSlotWaitSetting(t *testing.T) {
	cases := []struct {
		name        string
		raw         string
		wantEnabled bool
		wantBudget  time.Duration
		wantErr     bool
	}{
		{name: "explicit off disables", raw: "off", wantEnabled: false},
		{name: "OFF is case-insensitive", raw: "OFF", wantEnabled: false},
		{name: "a positive duration enables", raw: "250ms", wantEnabled: true, wantBudget: 250 * time.Millisecond},
		{name: "seconds work too", raw: "1s", wantEnabled: true, wantBudget: time.Second},
		{name: "zero is refused, not read as unbounded", raw: "0s", wantErr: true},
		{name: "bare zero is refused", raw: "0", wantErr: true},
		{name: "negative is refused", raw: "-1s", wantErr: true},
		{name: "unparseable is refused", raw: "soon", wantErr: true},
		{name: "empty is refused rather than guessed", raw: "", wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := RateLimitConfig{PrefetchSlotWait: tc.raw}
			budget, enabled, err := cfg.PrefetchSlotWaitSetting()

			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected %q to be refused at startup, got budget=%v enabled=%v", tc.raw, budget, enabled)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error for %q: %v", tc.raw, err)
			}
			if enabled != tc.wantEnabled {
				t.Fatalf("%q: enabled = %v, want %v", tc.raw, enabled, tc.wantEnabled)
			}
			if enabled && budget != tc.wantBudget {
				t.Fatalf("%q: budget = %v, want %v", tc.raw, budget, tc.wantBudget)
			}
		})
	}
}

// The default has to be a real, working value: a frontend that calls the RPC
// against a stock deployment must not get FAILED_PRECONDITION because nobody
// remembered to add an env var.
func TestPrefetchSlotWaitDefaultIsEnabledAndBounded(t *testing.T) {
	cfg := &Config{}
	if err := loadFromEnvironment(cfg); err != nil {
		t.Fatalf("loadFromEnvironment: %v", err)
	}

	budget, enabled, err := cfg.RateLimit.PrefetchSlotWaitSetting()
	if err != nil {
		t.Fatalf("the declared default must parse: %v", err)
	}
	if !enabled {
		t.Fatal("the declared default must be enabled; disabling is an operator decision, not a shipping default")
	}
	if budget <= 0 || budget > time.Second {
		t.Fatalf("default budget %v is not a give-up-fast budget", budget)
	}
}

// A refused value must stop the process, not warn and limp (rule 9).
func TestValidateRateLimitConfigRejectsBadPrefetchSlotWait(t *testing.T) {
	cfg := &RateLimitConfig{
		ExternalAPIInterval: 10 * time.Second,
		FeedFetchLimit:      100,
		PrefetchSlotWait:    "0s",
		DOSProtection: DOSProtectionConfig{
			RateLimit:      100,
			BurstLimit:     200,
			WindowSize:     time.Minute,
			BlockDuration:  5 * time.Minute,
			CircuitBreaker: CircuitBreakerConfig{FailureThreshold: 10, TimeoutDuration: 30 * time.Second, RecoveryTimeout: time.Minute},
		},
	}

	if err := validateRateLimitConfig(cfg); err == nil {
		t.Fatal("expected startup validation to reject RATE_LIMIT_PREFETCH_SLOT_WAIT=0s")
	}
}
