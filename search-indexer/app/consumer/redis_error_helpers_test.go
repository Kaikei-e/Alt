package consumer

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
		{name: "busygroup prefix", err: errors.New("BUSYGROUP"), want: true},
		{name: "other", err: errors.New("NOGROUP No such key"), want: false},
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
