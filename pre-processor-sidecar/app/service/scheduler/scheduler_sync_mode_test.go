package scheduler

import (
	"errors"
	"log/slog"
	"testing"
	"time"
)

func TestParseSyncMode(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    SyncMode
		wantErr bool
	}{
		{name: "unset keeps the historical enabled behaviour", raw: "", want: SyncEnabled},
		{name: "explicit enabled", raw: "enabled", want: SyncEnabled},
		{name: "explicit disabled", raw: "disabled", want: SyncDisabled},
		{name: "a boolean-ish value is rejected rather than guessed", raw: "false", wantErr: true},
		{name: "an unknown word is rejected", raw: "off", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseSyncMode(tt.raw)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ParseSyncMode(%q) = %q, want an error", tt.raw, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseSyncMode(%q) returned %v", tt.raw, err)
			}
			if got != tt.want {
				t.Errorf("ParseSyncMode(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}

func TestDefaultConfigEnablesSync(t *testing.T) {
	if got := DefaultConfig().SyncMode; got != SyncEnabled {
		t.Errorf("DefaultConfig().SyncMode = %q, want %q", got, SyncEnabled)
	}
}

func TestScheduler_Start_DisabledRunsNothing(t *testing.T) {
	s := NewScheduler(nil, nil, nil, slog.Default())
	s.Start(Config{FetchInterval: time.Hour, RefreshInterval: time.Hour, SyncMode: SyncDisabled})
	t.Cleanup(s.Stop)

	if s.isRunning {
		t.Error("Scheduler should not run while Inoreader sync is disabled")
	}
	if err := s.TriggerFetchNow(); !errors.Is(err, ErrSyncDisabled) {
		t.Errorf("TriggerFetchNow() = %v, want ErrSyncDisabled", err)
	}
	if err := s.TriggerRefreshNow(); !errors.Is(err, ErrSyncDisabled) {
		t.Errorf("TriggerRefreshNow() = %v, want ErrSyncDisabled", err)
	}
}

func TestScheduler_Start_PanicsWhenSyncModeUnset(t *testing.T) {
	// A SyncMode nobody wired must not be readable as "intentionally
	// disabled": that is the failure mode where a DI gap looks like an
	// operator decision.
	defer func() {
		if recover() == nil {
			t.Error("Start with an unset SyncMode should panic")
		}
	}()

	s := NewScheduler(nil, nil, nil, slog.Default())
	s.Start(Config{FetchInterval: time.Hour, RefreshInterval: time.Hour})
}
