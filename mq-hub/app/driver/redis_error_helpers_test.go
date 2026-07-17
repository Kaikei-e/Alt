package driver

import (
	"errors"
	"testing"
)

func TestIsBusyGroupErr(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil", err: nil, want: false},
		{name: "exact busygroup", err: errors.New("BUSYGROUP Consumer Group name already exists"), want: true},
		{name: "busygroup prefix only", err: errors.New("BUSYGROUP"), want: true},
		{name: "other redis error", err: errors.New("NOGROUP No such key"), want: false},
		{name: "substring not prefix", err: errors.New("ERR BUSYGROUP elsewhere"), want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := isBusyGroupErr(tt.err); got != tt.want {
				t.Fatalf("isBusyGroupErr(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestIsNoSuchKeyErr(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil", err: nil, want: false},
		{name: "contains no such key", err: errors.New("ERR no such key"), want: true},
		{name: "plain no such key", err: errors.New("no such key"), want: true},
		{name: "other error", err: errors.New("connection refused"), want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := isNoSuchKeyErr(tt.err); got != tt.want {
				t.Fatalf("isNoSuchKeyErr(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}
