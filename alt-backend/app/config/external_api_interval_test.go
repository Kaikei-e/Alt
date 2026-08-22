package config

import (
	"strings"
	"testing"
	"time"
)

// baseRateLimitConfig is a RateLimitConfig that validateRateLimitConfig accepts
// on every axis except the one a test is exercising, so a failure names the
// interval rather than some unrelated field the fixture forgot.
func baseRateLimitConfig(interval time.Duration) *RateLimitConfig {
	return &RateLimitConfig{
		ExternalAPIInterval: interval,
		ExternalAPIBurst:    3,
		FeedFetchLimit:      100,
		PrefetchSlotWait:    "250ms",
		DOSProtection: DOSProtectionConfig{
			RateLimit:     100,
			BurstLimit:    200,
			WindowSize:    time.Minute,
			BlockDuration: 5 * time.Minute,
			CircuitBreaker: CircuitBreakerConfig{
				FailureThreshold: 10,
				TimeoutDuration:  30 * time.Second,
				RecoveryTimeout:  time.Minute,
			},
		},
	}
}

// loadWithInterval runs the real startup path — env parse, then validate — with
// RATE_LIMIT_EXTERNAL_API_INTERVAL set to raw. An empty raw means "unset", which
// is how loadFromEnvironment reaches the struct-tag default.
func loadWithInterval(t *testing.T, raw string) (*Config, error) {
	t.Helper()

	clearTestEnv()
	t.Cleanup(clearTestEnv)

	// NewConfig refuses to start with the image proxy enabled (the default)
	// and an empty secret; unrelated to the interval, but required to get far
	// enough to see it.
	t.Setenv("IMAGE_PROXY_SECRET", "test-image-proxy-secret")
	t.Setenv("RATE_LIMIT_EXTERNAL_API_INTERVAL", raw)

	return NewConfig()
}

// The shipping default. CLAUDE.md rule 2 sets the floor at 5s; the operator
// chose 7.5s as the value between "too short" (5s) and "too long" (10s).
//
// The assertion is deliberately on the exact nanosecond count rather than on a
// rounded second, because 7.5s is the first default in this struct that is not
// a whole number of seconds — a stack that truncated it would still look
// plausible at 7s or 8s.
func TestExternalAPIInterval_DefaultIsSevenPointFiveSeconds(t *testing.T) {
	cfg, err := loadWithInterval(t, "")
	if err != nil {
		t.Fatalf("NewConfig() with no RATE_LIMIT_EXTERNAL_API_INTERVAL failed: %v", err)
	}

	const want = 7500 * time.Millisecond
	got := cfg.RateLimit.ExternalAPIInterval

	if got != want {
		t.Fatalf("default RateLimit.ExternalAPIInterval = %v (%d ns), want %v (%d ns)",
			got, int64(got), want, int64(want))
	}
	if got == 7*time.Second || got == 8*time.Second {
		t.Fatalf("default interval %v looks like a truncated/rounded 7.5s", got)
	}
	if got == 10*time.Second {
		t.Fatalf("default interval is still the old 10s value")
	}
}

// The burst is not part of this change and must stay where it is: a wider
// burst would raise the effective rate the interval is there to hold down.
func TestExternalAPIBurst_DefaultUnchanged(t *testing.T) {
	cfg, err := loadWithInterval(t, "")
	if err != nil {
		t.Fatalf("NewConfig() failed: %v", err)
	}
	if cfg.RateLimit.ExternalAPIBurst != 3 {
		t.Fatalf("RateLimit.ExternalAPIBurst = %d, want 3", cfg.RateLimit.ExternalAPIBurst)
	}
}

// CLAUDE.md rule 2 promises a publisher's server 5 seconds. Until the floor is
// code-enforced that promise is a convention: any operator can set 1s and the
// process starts happily. Rule 9 says the answer is a non-zero exit at startup,
// naming the variable and the floor — never a clamp and never a warning, both
// of which would leave the operator believing a value the process is not using.
func TestExternalAPIInterval_BelowFloorIsRejectedAtStartup(t *testing.T) {
	rejected := []string{
		"1s",     // the old validated floor
		"999ms",  // sub-second
		"2s",     // plausible but too fast
		"4s",     // just under
		"4999ms", // one millisecond under
		"4.999s", // the fractional form of the same
		"0s",     // degenerate
	}

	for _, raw := range rejected {
		t.Run(raw, func(t *testing.T) {
			cfg, err := loadWithInterval(t, raw)
			if err == nil {
				t.Fatalf("NewConfig() accepted RATE_LIMIT_EXTERNAL_API_INTERVAL=%s (got %v); "+
					"a value under the 5s floor must stop the process", raw, cfg.RateLimit.ExternalAPIInterval)
			}
			if cfg != nil {
				t.Fatalf("NewConfig() returned a config alongside the error for %s; "+
					"a rejected interval must not be clamped into a usable config", raw)
			}

			msg := err.Error()
			if !strings.Contains(msg, "RATE_LIMIT_EXTERNAL_API_INTERVAL") {
				t.Errorf("error must name the variable the operator has to fix, got: %s", msg)
			}
			if !strings.Contains(msg, "5s") {
				t.Errorf("error must name the 5s floor, got: %s", msg)
			}
		})
	}
}

// Exactly at the floor is a legal configuration: rule 2 says "at least 5
// seconds", so 5s is compliant and must not be rejected by an off-by-one.
func TestExternalAPIInterval_ExactlyAtFloorIsAccepted(t *testing.T) {
	for _, raw := range []string{"5s", "5000ms"} {
		t.Run(raw, func(t *testing.T) {
			cfg, err := loadWithInterval(t, raw)
			if err != nil {
				t.Fatalf("NewConfig() rejected the compliant floor value %s: %v", raw, err)
			}
			if cfg.RateLimit.ExternalAPIInterval != 5*time.Second {
				t.Fatalf("RateLimit.ExternalAPIInterval = %v, want 5s", cfg.RateLimit.ExternalAPIInterval)
			}
		})
	}
}

// Fractional intervals have to survive the env → struct hop intact. The loader
// goes through time.ParseDuration, which keeps the fraction as nanoseconds;
// this pins that so a future "simplify to seconds" refactor fails loudly.
func TestExternalAPIInterval_FractionalValuesRoundTrip(t *testing.T) {
	cases := map[string]time.Duration{
		"7.5s":   7500 * time.Millisecond,
		"7500ms": 7500 * time.Millisecond,
		"5.5s":   5500 * time.Millisecond,
		"12.25s": 12250 * time.Millisecond,
	}

	for raw, want := range cases {
		t.Run(raw, func(t *testing.T) {
			cfg, err := loadWithInterval(t, raw)
			if err != nil {
				t.Fatalf("NewConfig() failed for %s: %v", raw, err)
			}
			if got := cfg.RateLimit.ExternalAPIInterval; got != want {
				t.Fatalf("RateLimit.ExternalAPIInterval = %v (%d ns), want %v (%d ns)",
					got, int64(got), want, int64(want))
			}
		})
	}
}

// The primitive the whole fractional default rests on. Cheap, and it turns a
// hypothetical "does Go even parse that" into a fact the rest of the suite can
// lean on.
func TestParseDurationKeepsTheHalfSecond(t *testing.T) {
	d, err := time.ParseDuration("7.5s")
	if err != nil {
		t.Fatalf("time.ParseDuration(\"7.5s\"): %v", err)
	}
	if d != 7500*time.Millisecond {
		t.Fatalf("time.ParseDuration(\"7.5s\") = %v (%d ns), want 7.5s (7500000000 ns)", d, int64(d))
	}
	if d.String() != "7.5s" {
		t.Fatalf("round-trip String() = %q, want \"7.5s\"", d.String())
	}
}

// validateRateLimitConfig is the unit under the startup path above. Testing it
// directly keeps the floor assertion independent of everything else NewConfig
// validates.
func TestValidateRateLimitConfig_ExternalAPIIntervalFloor(t *testing.T) {
	tests := []struct {
		name     string
		interval time.Duration
		wantErr  bool
	}{
		{"one second is below the rule 2 floor", time.Second, true},
		{"4999ms is below the floor", 4999 * time.Millisecond, true},
		{"exactly 5s is compliant", 5 * time.Second, false},
		{"the 7.5s default is compliant", 7500 * time.Millisecond, false},
		{"10s is compliant", 10 * time.Second, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateRateLimitConfig(baseRateLimitConfig(tt.interval))
			if tt.wantErr && err == nil {
				t.Fatalf("validateRateLimitConfig(%v) = nil, want a rejection", tt.interval)
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("validateRateLimitConfig(%v) = %v, want nil", tt.interval, err)
			}
		})
	}
}
